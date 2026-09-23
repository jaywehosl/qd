#pragma once

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

extern GtkApplication *qd_app;

void qd_init(void);
int qd_start(int tray);
void qd_show(const char *uri);
void qd_complain(const char *title, const char *text);
void qd_post_show(void);
void qd_post_release(void);
void qd_post_quit(void);
void qd_post_load(const char *uri);

void qd_tray_start(GDBusConnection *bus);
void qd_post_state(int status, const char *tip, int connected);
int qd_tray_live(void);
