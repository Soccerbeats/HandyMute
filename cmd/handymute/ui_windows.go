//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/walk"
	dec "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const spiGetWorkArea = 0x0030 // SystemParametersInfo: usable desktop area (excludes taskbar)

var (
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	gdi32                     = windows.NewLazySystemDLL("gdi32.dll")
	procCreateRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	procSetWindowRgn          = user32.NewProc("SetWindowRgn") // user32 declared in hook.go
)

// ui hosts the Control Center: a borderless, rounded Walk window whose entire client area is
// an embedded WebView2 rendering controlcenter.html. Native Windows controls can't look like
// iOS Control Center, so the visuals are HTML/CSS and state crosses a small JSON bridge.
// Walk still provides the window, the tray icon (Handy's icon, glowing while held), flyout
// positioning, and dismiss-on-deactivate. All fields are touched only on the UI goroutine.
type ui struct {
	settings *Settings
	cmd      chan<- bool

	mw         *walk.MainWindow
	web        *edge.Chromium
	notifyIcon *walk.NotifyIcon
	overlay    *statusOverlay

	iconNormal *walk.Icon
	iconGlow   *walk.Icon
	shownAt    time.Time
	onScreen   bool
}

// offScreen parks the (still-visible) window far outside any monitor. Keeping the window
// shown — just off-screen — is what keeps the embedded WebView2 initialized and painted;
// SW_HIDE'ing a freshly embedded WebView2 leaves it blank.
const offScreen = -30000

func runUI(settings *Settings, cmd chan<- bool, status <-chan bool) error {
	runtime.LockOSThread()
	u := &ui{settings: settings, cmd: cmd}
	u.iconNormal, u.iconGlow = loadIcons()

	// Empty window — its whole client area becomes the WebView2 surface.
	if err := (dec.MainWindow{
		AssignTo: &u.mw,
		Title:    "HandyMute",
		Size:     dec.Size{Width: 340, Height: 470},
		Layout:   dec.VBox{MarginsZero: true},
		Visible:  false,
	}).Create(); err != nil {
		return err
	}

	u.makeFlyoutStyle()

	u.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		*canceled = true
		u.hideFlyout()
	})
	// Dismiss like a real flyout when focus leaves (ignore the transient deactivate right
	// after we show and hand focus to the webview).
	u.mw.Deactivating().Attach(func() {
		if u.onScreen && time.Since(u.shownAt) > 250*time.Millisecond {
			u.hideFlyout()
		}
	})
	u.mw.SizeChanged().Attach(func() {
		if u.web != nil {
			u.web.Resize()
		}
	})

	if err := u.embedWebView(); err != nil {
		return err
	}
	if err := u.buildTray(); err != nil {
		return err
	}

	// Status bubble overlay — created after the main window so WebView2 shares the process.
	if ov, err := newStatusOverlay(settings); err != nil {
		logf("overlay init failed (non-fatal): %v", err)
	} else {
		u.overlay = ov
	}

	// Glow the tray icon + the panel's status dot while ctrl+space is held.
	// Also show/hide the status bubble overlay.
	go func() {
		for active := range status {
			a := active
			u.mw.Synchronize(func() {
				u.setGlow(a)
				u.eval("setActive(%t)", a)
				if u.overlay != nil {
					if a && settings.Overlay() {
						u.overlay.Show()
					} else {
						u.overlay.Hide()
					}
				}
			})
		}
	}()

	u.mw.Run()
	return nil
}

func (u *ui) embedWebView() error {
	hwnd := u.mw.Handle()
	// Park off-screen and make visible BEFORE embedding — WebView2 needs a shown window to
	// initialize its render surface, but we don't want the panel on screen yet.
	win.SetWindowPos(hwnd, 0, offScreen, offScreen, 0, 0, win.SWP_NOSIZE|win.SWP_NOZORDER)
	u.mw.SetVisible(true)

	u.web = edge.NewChromium()
	u.web.MessageCallback = u.onMessage
	ok := u.web.Embed(uintptr(hwnd)) // blocks until the WebView2 controller is ready
	u.web.Resize()
	u.web.NavigateToString(controlCenterHTML)
	logf("webview: embed ok=%v, html=%d bytes, navigated", ok, len(controlCenterHTML))
	u.onScreen = false
	return nil
}

// onMessage handles JSON commands posted from the page. It runs on the UI goroutine (invoked
// during WebView2 message dispatch), so it may touch settings, Walk, and the webview freely.
// All settings actions go through the shared handler; only the Windows flyout window-management
// actions (hide/quit) are handled here.
func (u *ui) onMessage(raw string) {
	var m struct {
		Action string `json:"action"`
	}
	if json.Unmarshal([]byte(raw), &m) == nil {
		switch m.Action {
		case "hide":
			u.mw.Hide()
			return
		case "quit":
			walk.App().Exit(0)
			return
		}
	}
	handleControlMessage(raw, u.settings, u.cmd,
		func(js string) {
			if u.web != nil {
				u.web.Eval(js)
			}
		},
		u.setGlow,
	)
}

// pushState sends the current settings to the page so the controls reflect reality.
func (u *ui) pushState() {
	pushControlState(u.settings, func(js string) {
		if u.web != nil {
			u.web.Eval(js)
		}
	})
}

// eval runs a JS snippet in the page. Safe to call only on the UI goroutine.
func (u *ui) eval(format string, args ...any) {
	if u.web != nil {
		u.web.Eval(fmt.Sprintf(format, args...))
	}
}

func (u *ui) buildTray() error {
	ni, err := walk.NewNotifyIcon(u.mw)
	if err != nil {
		return err
	}
	u.notifyIcon = ni
	if u.iconNormal != nil {
		ni.SetIcon(u.iconNormal)
		u.mw.SetIcon(u.iconNormal)
	}
	ni.SetToolTip("HandyMute")
	ni.SetVisible(true)

	ni.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			u.toggleFlyout()
		}
	})

	open := walk.NewAction()
	open.SetText("Open HandyMute")
	open.Triggered().Attach(u.showFlyout)
	ni.ContextMenu().Actions().Add(open)

	ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())

	quit := walk.NewAction()
	quit.SetText("Quit")
	quit.Triggered().Attach(func() { walk.App().Exit(0) })
	ni.ContextMenu().Actions().Add(quit)
	return nil
}

func (u *ui) toggleFlyout() {
	if u.onScreen {
		u.hideFlyout()
		return
	}
	u.showFlyout()
}

// hideFlyout parks the window off-screen (keeping the WebView2 alive) — the flyout's hidden
// state.
func (u *ui) hideFlyout() {
	win.SetWindowPos(u.mw.Handle(), 0, offScreen, offScreen, 0, 0, win.SWP_NOSIZE|win.SWP_NOZORDER)
	u.onScreen = false
}

// showFlyout positions the panel just above the tray (bottom-right of the work area), rounds
// its corners, and brings it forward. Computed in real device pixels, so it is DPI-correct.
func (u *ui) showFlyout() {
	hwnd := u.mw.Handle()

	var wa win.RECT
	win.SystemParametersInfo(spiGetWorkArea, 0, unsafe.Pointer(&wa), 0)

	var rc win.RECT
	win.GetWindowRect(hwnd, &rc)
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top

	const margin = 10
	x := wa.Right - w - margin
	y := wa.Bottom - h - margin

	u.roundCorners(w, h)
	u.shownAt = time.Now()
	u.onScreen = true
	win.SetWindowPos(hwnd, 0, x, y, 0, 0, win.SWP_NOSIZE|win.SWP_NOZORDER)
	u.mw.Activate()
	u.pushState() // make sure controls match current settings each time it opens
}

// roundCorners clips the (borderless) window to a rounded-rectangle region so it reads as a
// flyout card. DWM corner preference doesn't apply to frameless windows, so we use a region.
func (u *ui) roundCorners(w, h int32) {
	const radius = 22
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), radius, radius)
	if rgn != 0 {
		procSetWindowRgn.Call(uintptr(u.mw.Handle()), rgn, 1) // window owns/frees the region
	}
}

// makeFlyoutStyle strips the title bar and resize frame (borderless) and hides the taskbar
// button, so the window is a bare popup panel.
func (u *ui) makeFlyoutStyle() {
	hwnd := u.mw.Handle()
	style := win.GetWindowLong(hwnd, win.GWL_STYLE)
	style &^= win.WS_CAPTION | win.WS_THICKFRAME | win.WS_MINIMIZEBOX | win.WS_MAXIMIZEBOX
	win.SetWindowLong(hwnd, win.GWL_STYLE, style)

	ex := win.GetWindowLong(hwnd, win.GWL_EXSTYLE)
	ex |= win.WS_EX_TOOLWINDOW
	win.SetWindowLong(hwnd, win.GWL_EXSTYLE, ex)

	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0,
		win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

func (u *ui) setGlow(on bool) {
	ic := u.iconNormal
	if on && u.iconGlow != nil {
		ic = u.iconGlow
	}
	if ic != nil && u.notifyIcon != nil {
		u.notifyIcon.SetIcon(ic)
	}
}
