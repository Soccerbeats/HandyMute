//go:build linux

#include "gtk_linux.h"
#include <stdlib.h>
#include <string.h>
#include <X11/Xlib.h>
#include <X11/Xutil.h>

GtkWidget *win;
WebKitWebView *webview;
static GtkWidget *trayMenu;
static GtkStatusIcon *trayIcon;

static void on_script_message(WebKitUserContentManager *m, WebKitJavascriptResult *r, gpointer u) {
	JSCValue *val = webkit_javascript_result_get_js_value(r);
	char *s = jsc_value_to_string(val);
	goWebMessage(s);
	g_free(s);
}

static gboolean run_js_idle(gpointer data) {
	char *js = (char*)data;
	webkit_web_view_evaluate_javascript(webview, js, -1, NULL, NULL, NULL, NULL, NULL);
	free(js);
	return G_SOURCE_REMOVE;
}

void ui_eval(const char *js) {
	g_idle_add(run_js_idle, strdup(js));
}

static void position_panel(void) {
	GdkScreen *screen = gtk_window_get_screen(GTK_WINDOW(win));
	int sw = gdk_screen_get_width(screen);
	gtk_window_move(GTK_WINDOW(win), sw - 350, 32);
}

static gboolean show_win_cb(gpointer d) {
	position_panel();
	gtk_widget_show_all(win);
	gtk_window_present(GTK_WINDOW(win));
	return G_SOURCE_REMOVE;
}

void ui_show(void) { g_idle_add(show_win_cb, NULL); }

static gboolean hide_win_cb(gpointer d) {
	gtk_widget_hide(win);
	return G_SOURCE_REMOVE;
}

void ui_hide(void) { g_idle_add(hide_win_cb, NULL); }

static void menu_open_cb(GtkMenuItem *i, gpointer d) { goMenuOpen(); }

static void menu_quit_cb(GtkMenuItem *i, gpointer d) { goMenuQuit(); }

static gboolean popup_menu_cb(gpointer d) {
	gtk_menu_popup_at_pointer(GTK_MENU(trayMenu), NULL);
	return G_SOURCE_REMOVE;
}

void ui_popup_menu(void) { g_idle_add(popup_menu_cb, NULL); }

// GtkStatusIcon (legacy XEmbed tray): handles its own clicks in-process, so
// single left-click activates and right-click pops the menu — unlike the SNI
// path, where the GNOME extension forces double-click to activate.
static void tray_activate_cb(GtkStatusIcon *icon, gpointer d) {
	if (gtk_widget_is_visible(win)) {
		gtk_widget_hide(win);
	} else {
		position_panel();
		gtk_widget_show_all(win);
		gtk_window_present(GTK_WINDOW(win));
	}
}

static void tray_popup_cb(GtkStatusIcon *icon, guint button, guint t, gpointer d) {
	gtk_menu_popup_at_pointer(GTK_MENU(trayMenu), NULL);
}

void ui_tray_init(const char *iconPath) {
	G_GNUC_BEGIN_IGNORE_DEPRECATIONS
	trayIcon = gtk_status_icon_new_from_file(iconPath);
	gtk_status_icon_set_tooltip_text(trayIcon, "HandyMute");
	gtk_status_icon_set_visible(trayIcon, TRUE);
	g_signal_connect(trayIcon, "activate", G_CALLBACK(tray_activate_cb), NULL);
	g_signal_connect(trayIcon, "popup-menu", G_CALLBACK(tray_popup_cb), NULL);
	G_GNUC_END_IGNORE_DEPRECATIONS
}

static gboolean set_icon_idle(gpointer data) {
	char *path = (char*)data;
	G_GNUC_BEGIN_IGNORE_DEPRECATIONS
	if (trayIcon) gtk_status_icon_set_from_file(trayIcon, path);
	G_GNUC_END_IGNORE_DEPRECATIONS
	free(path);
	return G_SOURCE_REMOVE;
}

void ui_tray_set_icon(const char *iconPath) { g_idle_add(set_icon_idle, strdup(iconPath)); }

static gboolean hide_if_not_active(gpointer data) {
	if (!gtk_window_is_active(GTK_WINDOW(win)) && gtk_widget_is_visible(win))
		gtk_widget_hide(win);
	return G_SOURCE_REMOVE;
}

static gboolean on_focus_out(GtkWidget *w, GdkEventFocus *e, gpointer d) {
	g_timeout_add(150, hide_if_not_active, NULL);
	return FALSE;
}

static gboolean on_delete(GtkWidget *w, GdkEvent *e, gpointer d) {
	gtk_widget_hide(w);
	return TRUE;
}

void ui_init(const char *html) {
	gtk_init(NULL, NULL);

	win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(win), "HandyMute");
	gtk_window_set_default_size(GTK_WINDOW(win), 340, 470);
	gtk_window_set_resizable(GTK_WINDOW(win), FALSE);
	gtk_window_set_decorated(GTK_WINDOW(win), FALSE);
	gtk_window_set_skip_taskbar_hint(GTK_WINDOW(win), TRUE);
	gtk_window_set_keep_above(GTK_WINDOW(win), TRUE);
	g_signal_connect(win, "delete-event", G_CALLBACK(on_delete), NULL);
	g_signal_connect(win, "focus-out-event", G_CALLBACK(on_focus_out), NULL);

	WebKitUserContentManager *ucm = webkit_user_content_manager_new();
	webkit_user_content_manager_register_script_message_handler(ucm, "handymute");
	g_signal_connect(ucm, "script-message-received::handymute", G_CALLBACK(on_script_message), NULL);

	const char *shim =
		"window.external={invoke:function(s){window.webkit.messageHandlers.handymute.postMessage(s);}};";
	WebKitUserScript *us = webkit_user_script_new(
		shim, WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
		WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, NULL, NULL);
	webkit_user_content_manager_add_script(ucm, us);

	webview = WEBKIT_WEB_VIEW(webkit_web_view_new_with_user_content_manager(ucm));
	webkit_web_view_load_html(webview, html, NULL);
	gtk_container_add(GTK_CONTAINER(win), GTK_WIDGET(webview));

	trayMenu = gtk_menu_new();
	GtkWidget *mi_open = gtk_menu_item_new_with_label("Open HandyMute");
	g_signal_connect(mi_open, "activate", G_CALLBACK(menu_open_cb), NULL);
	gtk_menu_shell_append(GTK_MENU_SHELL(trayMenu), mi_open);
	GtkWidget *mi_quit = gtk_menu_item_new_with_label("Quit");
	g_signal_connect(mi_quit, "activate", G_CALLBACK(menu_quit_cb), NULL);
	gtk_menu_shell_append(GTK_MENU_SHELL(trayMenu), mi_quit);
	gtk_widget_show_all(trayMenu);
}

void ui_run(void) { gtk_main(); }

// ---- Status overlay pill ----

static GtkWidget *overlayWin;
static GtkWidget *overlayLabel;

// Handy renders its recording pill at the top of a 200x200 host window. This is the
// vertical offset from that window's top edge to just below the visible pill — a Handy
// UI metric, not a screen coordinate, so our overlay still tracks wherever Handy moves.
#define HANDY_PILL_OFFSET 44
// Handy's visible pill sits a few px left of its 200px window's center; nudge to match.
#define HANDY_PILL_DX -4

// find_handy_overlay walks the X window tree for Handy's recording overlay, matched on
// WM_CLASS "Handy" + WM_NAME "Recording" (distinguishes it from Handy's main window).
static Window find_handy_overlay(Display *d, Window w) {
    Window found = 0;
    XClassHint ch;
    if (XGetClassHint(d, w, &ch)) {
        int match = ch.res_class && strcmp(ch.res_class, "Handy") == 0;
        if (ch.res_name)  XFree(ch.res_name);
        if (ch.res_class) XFree(ch.res_class);
        if (match) {
            char *name = NULL;
            if (XFetchName(d, w, &name) && name) {
                if (strcmp(name, "Recording") == 0) found = w;
                XFree(name);
            }
        }
    }
    if (found) return found;

    Window root, parent, *children;
    unsigned int n;
    if (XQueryTree(d, w, &root, &parent, &children, &n)) {
        for (unsigned int i = 0; i < n && !found; i++)
            found = find_handy_overlay(d, children[i]);
        if (children) XFree(children);
    }
    return found;
}

// handy_overlay_geom returns Handy's recording-window geometry in root coordinates.
static gboolean handy_overlay_geom(int *ox, int *oy, int *ow, int *oh) {
    Display *d = XOpenDisplay(NULL);
    if (!d) return FALSE;
    Window root = DefaultRootWindow(d);
    Window hw = find_handy_overlay(d, root);
    gboolean ok = FALSE;
    if (hw) {
        Window r, child;
        int gx, gy, ax, ay;
        unsigned int gw, gh, bw, depth;
        if (XGetGeometry(d, hw, &r, &gx, &gy, &gw, &gh, &bw, &depth) &&
            XTranslateCoordinates(d, hw, root, 0, 0, &ax, &ay, &child)) {
            *ox = ax; *oy = ay; *ow = (int)gw; *oh = (int)gh;
            ok = TRUE;
        }
    }
    XCloseDisplay(d);
    return ok;
}

static void overlay_position(void) {
    GtkRequisition nat;
    gtk_widget_get_preferred_size(overlayWin, NULL, &nat);

    // Horizontal: always centered on the full screen. Vertical: tucked under Handy's
    // pill (following it) when found, else the bottom of the screen.
    GdkDisplay  *disp = gdk_display_get_default();
    GdkMonitor  *mon  = gdk_display_get_primary_monitor(disp);
    GdkRectangle geo;
    gdk_monitor_get_geometry(mon, &geo);
    int x = geo.x + (geo.width - nat.width) / 2;

    int hx, hy, hw, hh;
    if (handy_overlay_geom(&hx, &hy, &hw, &hh)) {
        int y = hy + HANDY_PILL_OFFSET;
        gtk_window_move(GTK_WINDOW(overlayWin), x, y);
        return;
    }

    int y = geo.y + geo.height - nat.height - 8;
    gtk_window_move(GTK_WINDOW(overlayWin), x, y);
}

// While shown, re-query Handy's live position so our pill follows it in real time.
static guint overlayTrackId = 0;

static gboolean overlay_track_cb(gpointer d) {
    overlay_position();
    return G_SOURCE_CONTINUE;
}

// Opacity fade: fade in over FADE_IN_MS, fade out over FADE_OUT_MS, then hide the window.
#define OVERLAY_OPACITY 0.92
#define FADE_IN_MS      500.0
#define FADE_OUT_MS     650.0
#define FADE_TICK_MS    16

static guint   overlayFadeId = 0;
static double  overlayCur    = 0.0;  // current window opacity
static double  overlayStep   = 0.0;  // signed opacity delta per tick

static gboolean overlay_fade_cb(gpointer d) {
    overlayCur += overlayStep;
    if (overlayStep > 0 && overlayCur >= OVERLAY_OPACITY) {        // fade-in done
        overlayCur = OVERLAY_OPACITY;
        gtk_widget_set_opacity(overlayWin, overlayCur);
        overlayFadeId = 0;
        return G_SOURCE_REMOVE;
    }
    if (overlayStep < 0 && overlayCur <= 0.0) {                    // fade-out done — hide
        overlayCur = 0.0;
        gtk_widget_set_opacity(overlayWin, 0.0);
        gtk_widget_hide(overlayWin);
        overlayFadeId = 0;
        return G_SOURCE_REMOVE;
    }
    gtk_widget_set_opacity(overlayWin, overlayCur);
    return G_SOURCE_CONTINUE;
}

static void overlay_start_fade(gboolean out) {
    if (overlayFadeId) { g_source_remove(overlayFadeId); overlayFadeId = 0; }
    double dur = out ? FADE_OUT_MS : FADE_IN_MS;
    overlayStep = (out ? -OVERLAY_OPACITY : OVERLAY_OPACITY) * (FADE_TICK_MS / dur);
    overlayFadeId = g_timeout_add(FADE_TICK_MS, overlay_fade_cb, NULL);
}

static gboolean overlay_show_idle(gpointer data) {
    char *markup = (char*)data;
    gtk_label_set_markup(GTK_LABEL(overlayLabel), markup);
    free(markup);
    overlay_position();
    gtk_widget_set_opacity(overlayWin, overlayCur);  // resume from current (0, or mid fade-out)
    gtk_widget_show_all(overlayWin);
    if (overlayTrackId == 0)
        overlayTrackId = g_timeout_add(120, overlay_track_cb, NULL);
    overlay_start_fade(FALSE);                        // fade in
    return G_SOURCE_REMOVE;
}

static gboolean overlay_hide_idle(gpointer data) {
    if (overlayTrackId) {
        g_source_remove(overlayTrackId);
        overlayTrackId = 0;
    }
    overlay_start_fade(TRUE);                          // fade out, then hide in the callback
    return G_SOURCE_REMOVE;
}

void overlay_show(const char *markup) { g_idle_add(overlay_show_idle, strdup(markup)); }
void overlay_hide(void)               { g_idle_add(overlay_hide_idle, NULL); }

void overlay_init(void) {
    overlayWin = gtk_window_new(GTK_WINDOW_POPUP);
    gtk_window_set_skip_taskbar_hint(GTK_WINDOW(overlayWin), TRUE);
    gtk_window_set_keep_above(GTK_WINDOW(overlayWin), TRUE);
    gtk_window_set_accept_focus(GTK_WINDOW(overlayWin), FALSE);
    gtk_widget_set_opacity(overlayWin, 0.0);  // starts transparent; fades in on show

    // Give the window an alpha-capable visual so the corners outside the rounded
    // background are transparent — otherwise the radius has nothing to clip and the
    // window reads as a square. Requires a compositor.
    GdkScreen *screen = gtk_widget_get_screen(overlayWin);
    GdkVisual *rgba = gdk_screen_get_rgba_visual(screen);
    if (rgba) gtk_widget_set_visual(overlayWin, rgba);

    // Capsule pill: large border-radius (GTK clamps to half-height) rounds the ends.
    // The transparent window node lets the rounded background show as a true pill.
    GtkCssProvider *css = gtk_css_provider_new();
    gtk_css_provider_load_from_data(css,
        ".hm-overlay { background-color: #121214; border-radius: 999px; }"
        ".hm-overlay label { padding: 5px 10px; }",
        -1, NULL);
    // Register screen-wide, not on the window's context — a per-widget provider does not
    // cascade to children, so ".hm-overlay label" would never reach the label (padding).
    gtk_style_context_add_provider_for_screen(screen,
        GTK_STYLE_PROVIDER(css), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
    gtk_style_context_add_class(gtk_widget_get_style_context(overlayWin), "hm-overlay");

    overlayLabel = gtk_label_new(NULL);
    gtk_label_set_use_markup(GTK_LABEL(overlayLabel), TRUE);
    gtk_container_add(GTK_CONTAINER(overlayWin), overlayLabel);
}
