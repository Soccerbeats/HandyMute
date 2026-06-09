//go:build linux

#ifndef GTK_LINUX_H
#define GTK_LINUX_H

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

extern GtkWidget *win;
extern WebKitWebView *webview;

extern void goWebMessage(char *msg);
extern void goMenuOpen();
extern void goMenuQuit();

void ui_eval(const char *js);
void ui_show(void);
void ui_hide(void);
void ui_popup_menu(void);
void ui_tray_init(const char *iconPath);
void ui_tray_set_icon(const char *iconPath);
void ui_init(const char *html);
void ui_run(void);

#endif
