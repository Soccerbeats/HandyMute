//go:build linux

#include "gtk_linux.h"
#include <stdlib.h>
#include <string.h>

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
