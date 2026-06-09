//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/walk"
	dec "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

//go:embed overlay.html
var overlayHTML string

const (
	overlayW = 210
	overlayH = 34

	// Extended style: window never becomes the foreground / active window.
	wsExNoActivate = 0x08000000
	// Extended style: layered window (required for alpha transparency).
	wsExLayered = 0x00080000
	// SetWindowPos flag: do not activate the window.
	swpNoActivate = 0x0010
	// SetLayeredWindowAttributes flag: use bAlpha for whole-window opacity.
	lwaAlpha = 0x2
)

// statusOverlay is a compact pill that floats just below Handy's dictation bubble while
// ctrl+space is held. It shows a pulsing green dot, "Active", and the two volume levels.
// When ctrl+space is released it parks off-screen (keeping WebView2 warm for the next hold).
type statusOverlay struct {
	mw       *walk.MainWindow
	web      *edge.Chromium
	settings *Settings
}

// newStatusOverlay creates and parks the overlay window. Must be called on the UI goroutine.
func newStatusOverlay(settings *Settings) (*statusOverlay, error) {
	o := &statusOverlay{settings: settings}

	if err := (dec.MainWindow{
		AssignTo: &o.mw,
		Title:    "HandyMuteStatus",
		Size:     dec.Size{Width: overlayW, Height: overlayH},
		Layout:   dec.VBox{MarginsZero: true},
		Visible:  false,
	}).Create(); err != nil {
		return nil, fmt.Errorf("overlay window: %w", err)
	}

	hwnd := o.mw.Handle()

	// Strip title bar and resize chrome — borderless popup.
	style := win.GetWindowLong(hwnd, win.GWL_STYLE)
	style &^= win.WS_CAPTION | win.WS_THICKFRAME | win.WS_MINIMIZEBOX | win.WS_MAXIMIZEBOX | win.WS_SYSMENU
	win.SetWindowLong(hwnd, win.GWL_STYLE, style)

	// Always on top, no taskbar entry, never steals focus, semi-transparent.
	ex := win.GetWindowLong(hwnd, win.GWL_EXSTYLE)
	ex |= win.WS_EX_TOOLWINDOW | win.WS_EX_TOPMOST | wsExNoActivate | wsExLayered
	win.SetWindowLong(hwnd, win.GWL_EXSTYLE, ex)

	// ~82% opacity — matches Handy's frosted-pill look.
	procSetLayeredWindowAttributes := user32.NewProc("SetLayeredWindowAttributes")
	procSetLayeredWindowAttributes.Call(uintptr(hwnd), 0, 210, lwaAlpha)

	win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0,
		win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_FRAMECHANGED)

	// Rounded pill shape (radius = half the height for a full pill).
	const r = 17
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(overlayW+1), uintptr(overlayH+1), r, r)
	if rgn != 0 {
		procSetWindowRgn.Call(uintptr(hwnd), rgn, 1)
	}

	// WebView2 needs the window to be shown before Embed(). Park off-screen so it's invisible
	// to the user but satisfies the WebView2 requirement.
	win.SetWindowPos(hwnd, 0, offScreen, offScreen, 0, 0, win.SWP_NOSIZE|win.SWP_NOZORDER)
	o.mw.SetVisible(true)

	o.web = edge.NewChromium()
	o.web.Embed(uintptr(hwnd))
	o.web.Resize()
	o.web.NavigateToString(overlayHTML)

	logf("overlay: ready (%dx%d)", overlayW, overlayH)
	return o, nil
}

// Show positions the overlay just above the taskbar (below Handy's bubble) and updates
// the displayed levels from current settings. Safe to call only on the UI goroutine.
func (o *statusOverlay) Show() {
	snap := o.settings.Snapshot()
	o.web.Eval(fmt.Sprintf("update(%d,%d)", pct(snap.TeamsLevel), pct(snap.SpeakerDuck)))

	var wa win.RECT
	win.SystemParametersInfo(spiGetWorkArea, 0, unsafe.Pointer(&wa), 0)

	x := (wa.Left+wa.Right)/2 - overlayW/2
	y := wa.Bottom - int32(overlayH) + 2

	win.SetWindowPos(o.mw.Handle(), win.HWND_TOPMOST,
		x, y, 0, 0,
		win.SWP_NOSIZE|swpNoActivate)
}

// Hide parks the overlay off-screen without destroying the WebView2 controller.
// Safe to call only on the UI goroutine.
func (o *statusOverlay) Hide() {
	win.SetWindowPos(o.mw.Handle(), 0, offScreen, offScreen, 0, 0,
		win.SWP_NOSIZE|win.SWP_NOZORDER|swpNoActivate)
}
