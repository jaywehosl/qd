#include <string.h>

#include "qd_linux.h"
#include "_cgo_export.h"

GtkApplication *qd_app;
static GtkWidget *qd_window;
static int qd_quiet;

static const char *qd_bridge =
	"window.qdWindowCommand=function(w){window.webkit.messageHandlers.qd.postMessage(String(w))};"
	"window.qdWindowGrab=function(e){window.webkit.messageHandlers.qd.postMessage('grab:'+e)};";

static const char *qd_edges[] = {"topleft", "top", "topright", "left", "right", "bottomleft", "bottom", "bottomright"};

static GtkWidget *qd_elsewhere(WebKitWebView *view, WebKitNavigationAction *action, gpointer data) {
	WebKitURIRequest *req = webkit_navigation_action_get_request(action);
	const char *uri = webkit_uri_request_get_uri(req);
	if (uri && *uri) {
		g_app_info_launch_default_for_uri(uri, NULL, NULL);
	}
	return NULL;
}

static void qd_grab(GtkWindow *win, const char *edge) {
	GdkDevice *pointer = gdk_seat_get_pointer(gdk_display_get_default_seat(gdk_display_get_default()));
	gint x, y;
	gdk_device_get_position(pointer, NULL, &x, &y);
	if (g_str_equal(edge, "caption")) {
		gtk_window_begin_move_drag(win, 1, x, y, GDK_CURRENT_TIME);
		return;
	}
	for (int i = 0; i < 8; i++) {
		if (g_str_equal(edge, qd_edges[i])) {
			gtk_window_begin_resize_drag(win, (GdkWindowEdge)i, 1, x, y, GDK_CURRENT_TIME);
			return;
		}
	}
}

static void qd_message(WebKitUserContentManager *m, WebKitJavascriptResult *r, gpointer data) {
	char *msg = jsc_value_to_string(webkit_javascript_result_get_js_value(r));
	GtkWindow *win = GTK_WINDOW(qd_window);
	if (g_str_equal(msg, "minimise")) {
		gtk_window_iconify(win);
	} else if (g_str_equal(msg, "maximise")) {
		if (gtk_window_is_maximized(win)) gtk_window_unmaximize(win); else gtk_window_maximize(win);
	} else if (g_str_equal(msg, "close")) {
		gtk_window_close(win);
	} else if (g_str_has_prefix(msg, "grab:")) {
		qd_grab(win, msg + 5);
	}
	g_free(msg);
}

static WebKitWebView *qd_view;

static gboolean qd_permit(WebKitWebView *view, WebKitPermissionRequest *req, gpointer data) {
	if (!WEBKIT_IS_CLIPBOARD_PERMISSION_REQUEST(req)) return FALSE;
	webkit_permission_request_allow(req);
	return TRUE;
}

static GHashTable *qd_saved;

static void qd_reveal(const char *file) {
	GDBusConnection *bus = g_application_get_dbus_connection(G_APPLICATION(qd_app));
	char *uri = g_filename_to_uri(file, NULL, NULL);
	const char *uris[] = {uri, NULL};
	GVariant *r = bus ? g_dbus_connection_call_sync(bus, "org.freedesktop.FileManager1", "/org/freedesktop/FileManager1",
		"org.freedesktop.FileManager1", "ShowItems", g_variant_new("(^ass)", uris, ""),
		NULL, G_DBUS_CALL_FLAGS_NONE, 3000, NULL, NULL) : NULL;
	if (r) {
		g_variant_unref(r);
	} else {
		char *dir = g_path_get_dirname(file);
		char *dir_uri = g_filename_to_uri(dir, NULL, NULL);
		g_app_info_launch_default_for_uri(dir_uri, NULL, NULL);
		g_free(dir_uri);
		g_free(dir);
	}
	g_free(uri);
}

static void qd_action(GDBusConnection *c, const gchar *sender, const gchar *path, const gchar *iface,
	const gchar *signal, GVariant *params, gpointer data) {
	guint32 id;
	const gchar *action;
	g_variant_get(params, "(u&s)", &id, &action);
	const char *file = g_hash_table_lookup(qd_saved, GUINT_TO_POINTER(id));
	if (file) qd_reveal(file);
}

static void qd_notified(GObject *src, GAsyncResult *res, gpointer file) {
	GVariant *r = g_dbus_connection_call_finish(G_DBUS_CONNECTION(src), res, NULL);
	if (r && file) {
		guint32 id;
		g_variant_get(r, "(u)", &id);
		g_hash_table_replace(qd_saved, GUINT_TO_POINTER(id), file);
		file = NULL;
	}
	if (r) g_variant_unref(r);
	g_free(file);
}

static void qd_notify(const char *summary, const char *body, const char *file) {
	GDBusConnection *bus = g_application_get_dbus_connection(G_APPLICATION(qd_app));
	if (!bus) return;
	if (!qd_saved) {
		qd_saved = g_hash_table_new_full(g_direct_hash, g_direct_equal, NULL, g_free);
		g_dbus_connection_signal_subscribe(bus, "org.freedesktop.Notifications", "org.freedesktop.Notifications",
			"ActionInvoked", "/org/freedesktop/Notifications", NULL, G_DBUS_SIGNAL_FLAGS_NONE, qd_action, NULL, NULL);
	}
	const char *actions[] = {"default", "Show in folder", "reveal", "Show in folder", NULL};
	g_dbus_connection_call(bus, "org.freedesktop.Notifications", "/org/freedesktop/Notifications",
		"org.freedesktop.Notifications", "Notify",
		g_variant_new("(susss@as@a{sv}i)", "qd", 0, "qd-client", summary, body,
			g_variant_new_strv(file ? actions : NULL, file ? -1 : 0),
			g_variant_new_array(G_VARIANT_TYPE("{sv}"), NULL, 0), -1),
		G_VARIANT_TYPE("(u)"), G_DBUS_CALL_FLAGS_NONE, -1, NULL, qd_notified, g_strdup(file));
}

static void qd_download_failed(WebKitDownload *d, GError *err, gpointer data) {
	g_object_set_data(G_OBJECT(d), "qd-failed", GINT_TO_POINTER(1));
	if (!g_error_matches(err, WEBKIT_DOWNLOAD_ERROR, WEBKIT_DOWNLOAD_ERROR_CANCELLED_BY_USER)) {
		qd_notify("Download failed", err->message, NULL);
	}
}

static void qd_download_done(WebKitDownload *d, gpointer data) {
	if (g_object_get_data(G_OBJECT(d), "qd-failed")) return;
	const char *dest = webkit_download_get_destination(d);
	if (!dest) return;
	char *path = g_str_has_prefix(dest, "file://") ? g_filename_from_uri(dest, NULL, NULL) : g_strdup(dest);
	char *body = g_strdup_printf("Saved to %s", path ? path : dest);
	qd_notify("Download complete", body, path);
	g_free(body);
	g_free(path);
}

static gboolean qd_destination(WebKitDownload *d, gchar *suggested, gpointer data) {
	const char *dir = g_get_user_special_dir(G_USER_DIRECTORY_DOWNLOAD);
	if (!dir) dir = g_get_home_dir();
	char *name = g_path_get_basename(suggested && *suggested ? suggested : "download");
	const char *dot = strrchr(name, '.');
	if (dot == name) dot = NULL;
	char *stem = dot ? g_strndup(name, dot - name) : g_strdup(name);
	const char *ext = dot ? dot : "";

	char *path = g_build_filename(dir, name, NULL);
	for (int i = 1; g_file_test(path, G_FILE_TEST_EXISTS); i++) {
		g_free(path);
		char *alt = g_strdup_printf("%s (%d)%s", stem, i, ext);
		path = g_build_filename(dir, alt, NULL);
		g_free(alt);
	}
	char *uri = g_filename_to_uri(path, NULL, NULL);
	webkit_download_set_destination(d, uri);

	g_free(uri);
	g_free(path);
	g_free(stem);
	g_free(name);
	return TRUE;
}

static void qd_download(WebKitWebContext *ctx, WebKitDownload *d, gpointer data) {
	g_signal_connect(d, "decide-destination", G_CALLBACK(qd_destination), NULL);
	g_signal_connect(d, "failed", G_CALLBACK(qd_download_failed), NULL);
	g_signal_connect(d, "finished", G_CALLBACK(qd_download_done), NULL);
}

static void qd_gone(GtkWidget *w, gpointer data) {
	qd_window = NULL;
	qd_view = NULL;
}

static gboolean qd_load_idle(gpointer uri) {
	if (qd_view) webkit_web_view_load_uri(qd_view, uri);
	g_free(uri);
	return G_SOURCE_REMOVE;
}

void qd_post_load(const char *uri) { g_idle_add(qd_load_idle, g_strdup(uri)); }

void qd_init(void) {
	g_set_prgname("qd-client");
	g_set_application_name("qd");
	gtk_init(NULL, NULL);
	gtk_window_set_default_icon_name("qd-client");
}

void qd_show(const char *uri) {
	if (qd_window) {
		gtk_window_present(GTK_WINDOW(qd_window));
		return;
	}

	WebKitUserContentManager *content = webkit_user_content_manager_new();
	WebKitUserStyleSheet *sheet = webkit_user_style_sheet_new(".topbar-shell{view-transition-name:none!important}",
		WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, WEBKIT_USER_STYLE_LEVEL_USER, NULL, NULL);
	webkit_user_content_manager_add_style_sheet(content, sheet);
	webkit_user_style_sheet_unref(sheet);
	WebKitUserScript *script = webkit_user_script_new(qd_bridge, WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
		WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, NULL, NULL);
	webkit_user_content_manager_add_script(content, script);
	webkit_user_script_unref(script);
	g_signal_connect(content, "script-message-received::qd", G_CALLBACK(qd_message), NULL);
	webkit_user_content_manager_register_script_message_handler(content, "qd");
	WebKitWebView *view = WEBKIT_WEB_VIEW(webkit_web_view_new_with_user_content_manager(content));
	g_signal_connect(view, "create", G_CALLBACK(qd_elsewhere), NULL);
	g_signal_connect(view, "permission-request", G_CALLBACK(qd_permit), NULL);
	webkit_settings_set_enable_developer_extras(webkit_web_view_get_settings(view), TRUE);
	static gboolean watching;
	if (!watching) {
		watching = TRUE;
		g_signal_connect(webkit_web_view_get_context(view), "download-started", G_CALLBACK(qd_download), NULL);
	}
	qd_view = view;

	qd_window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_application(GTK_WINDOW(qd_window), qd_app);
	gtk_window_set_decorated(GTK_WINDOW(qd_window), FALSE);
	gtk_window_set_title(GTK_WINDOW(qd_window), "qd");
	gtk_window_set_default_size(GTK_WINDOW(qd_window), 1280, 860);
	gtk_window_set_position(GTK_WINDOW(qd_window), GTK_WIN_POS_CENTER);
	gtk_container_add(GTK_CONTAINER(qd_window), GTK_WIDGET(view));
	g_signal_connect(qd_window, "destroy", G_CALLBACK(qd_gone), NULL);

	webkit_web_view_load_uri(view, uri);
	gtk_widget_show_all(qd_window);
}

void qd_complain(const char *title, const char *text) {
	GtkWidget *d = gtk_message_dialog_new(NULL, 0, GTK_MESSAGE_ERROR, GTK_BUTTONS_CLOSE, "%s", title);
	gtk_message_dialog_format_secondary_text(GTK_MESSAGE_DIALOG(d), "%s", text);
	gtk_window_set_title(GTK_WINDOW(d), "qd");
	gtk_dialog_run(GTK_DIALOG(d));
	gtk_widget_destroy(d);
}

static gboolean qd_show_idle(gpointer data) {
	qdOpen();
	return G_SOURCE_REMOVE;
}

static gboolean qd_release_idle(gpointer data) {
	g_application_release(G_APPLICATION(qd_app));
	return G_SOURCE_REMOVE;
}

static gboolean qd_quit_idle(gpointer data) {
	g_application_quit(G_APPLICATION(qd_app));
	return G_SOURCE_REMOVE;
}

void qd_post_show(void) { g_idle_add(qd_show_idle, NULL); }

void qd_post_release(void) { g_idle_add(qd_release_idle, NULL); }

void qd_post_quit(void) { g_idle_add(qd_quit_idle, NULL); }

static void qd_startup(GApplication *app, gpointer data) {
	qd_tray_start(g_application_get_dbus_connection(app));
}

static void qd_activate(GApplication *app, gpointer data) {
	if (qd_quiet) {
		qd_quiet = 0;
		return;
	}
	qdOpen();
}

int qd_start(int tray) {
	qd_app = gtk_application_new("qd.client", (GApplicationFlags)0);
	g_signal_connect(qd_app, "startup", G_CALLBACK(qd_startup), NULL);
	g_signal_connect(qd_app, "activate", G_CALLBACK(qd_activate), NULL);

	if (!g_application_register(G_APPLICATION(qd_app), NULL, NULL)) {
		return 1;
	}
	if (g_application_get_is_remote(G_APPLICATION(qd_app))) {
		return tray ? 0 : g_application_run(G_APPLICATION(qd_app), 0, NULL);
	}
	qd_quiet = 1;
	g_application_hold(G_APPLICATION(qd_app));
	return g_application_run(G_APPLICATION(qd_app), 0, NULL);
}
