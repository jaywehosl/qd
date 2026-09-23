#include <unistd.h>

#include "qd_linux.h"
#include "_cgo_export.h"

#define SNI_PATH "/StatusNotifierItem"
#define SNI_IFACE "org.kde.StatusNotifierItem"
#define MENU_PATH "/MenuBar"
#define MENU_IFACE "com.canonical.dbusmenu"
#define WATCHER "org.kde.StatusNotifierWatcher"

enum { ITEM_ROOT, ITEM_CONNECT, ITEM_DISCONNECT, ITEM_SEPARATOR, ITEM_QUIT, ITEM_OPEN, ITEM_GAP };

static const int menu_order[] = {ITEM_OPEN, ITEM_GAP, ITEM_CONNECT, ITEM_DISCONNECT, ITEM_SEPARATOR, ITEM_QUIT};

static GDBusConnection *bus;
static char *bus_name;
static GVariant *pixmaps[3];
static int status_now;
static int connected_now;
static char *tip_now;
static guint revision = 1;
static gint live;
static int held;

static const char sni_xml[] =
	"<node><interface name='" SNI_IFACE "'>"
	"<property name='Category' type='s' access='read'/>"
	"<property name='Id' type='s' access='read'/>"
	"<property name='Title' type='s' access='read'/>"
	"<property name='Status' type='s' access='read'/>"
	"<property name='WindowId' type='i' access='read'/>"
	"<property name='IconName' type='s' access='read'/>"
	"<property name='IconPixmap' type='a(iiay)' access='read'/>"
	"<property name='OverlayIconName' type='s' access='read'/>"
	"<property name='OverlayIconPixmap' type='a(iiay)' access='read'/>"
	"<property name='AttentionIconName' type='s' access='read'/>"
	"<property name='AttentionIconPixmap' type='a(iiay)' access='read'/>"
	"<property name='AttentionMovieName' type='s' access='read'/>"
	"<property name='ToolTip' type='(sa(iiay)ss)' access='read'/>"
	"<property name='ItemIsMenu' type='b' access='read'/>"
	"<property name='Menu' type='o' access='read'/>"
	"<method name='ContextMenu'><arg name='x' type='i' direction='in'/><arg name='y' type='i' direction='in'/></method>"
	"<method name='Activate'><arg name='x' type='i' direction='in'/><arg name='y' type='i' direction='in'/></method>"
	"<method name='SecondaryActivate'><arg name='x' type='i' direction='in'/><arg name='y' type='i' direction='in'/></method>"
	"<method name='Scroll'><arg name='delta' type='i' direction='in'/><arg name='orientation' type='s' direction='in'/></method>"
	"<signal name='NewTitle'/><signal name='NewIcon'/><signal name='NewAttentionIcon'/>"
	"<signal name='NewOverlayIcon'/><signal name='NewToolTip'/>"
	"<signal name='NewStatus'><arg name='status' type='s'/></signal>"
	"</interface></node>";

static const char menu_xml[] =
	"<node><interface name='" MENU_IFACE "'>"
	"<property name='Version' type='u' access='read'/>"
	"<property name='TextDirection' type='s' access='read'/>"
	"<property name='Status' type='s' access='read'/>"
	"<property name='IconThemePath' type='as' access='read'/>"
	"<method name='GetLayout'><arg type='i' name='parentId' direction='in'/><arg type='i' name='recursionDepth' direction='in'/>"
	"<arg type='as' name='propertyNames' direction='in'/><arg type='u' name='revision' direction='out'/>"
	"<arg type='(ia{sv}av)' name='layout' direction='out'/></method>"
	"<method name='GetGroupProperties'><arg type='ai' name='ids' direction='in'/><arg type='as' name='propertyNames' direction='in'/>"
	"<arg type='a(ia{sv})' name='properties' direction='out'/></method>"
	"<method name='GetProperty'><arg type='i' name='id' direction='in'/><arg type='s' name='name' direction='in'/>"
	"<arg type='v' name='value' direction='out'/></method>"
	"<method name='Event'><arg type='i' name='id' direction='in'/><arg type='s' name='eventId' direction='in'/>"
	"<arg type='v' name='data' direction='in'/><arg type='u' name='timestamp' direction='in'/></method>"
	"<method name='EventGroup'><arg type='a(isvu)' name='events' direction='in'/><arg type='ai' name='idErrors' direction='out'/></method>"
	"<method name='AboutToShow'><arg type='i' name='id' direction='in'/><arg type='b' name='needUpdate' direction='out'/></method>"
	"<method name='AboutToShowGroup'><arg type='ai' name='ids' direction='in'/><arg type='ai' name='updatesNeeded' direction='out'/>"
	"<arg type='ai' name='idErrors' direction='out'/></method>"
	"<signal name='ItemsPropertiesUpdated'><arg type='a(ia{sv})' name='updatedProps'/><arg type='a(ias)' name='removedProps'/></signal>"
	"<signal name='LayoutUpdated'><arg type='u' name='revision'/><arg type='i' name='parent'/></signal>"
	"<signal name='ItemActivationRequested'><arg type='i' name='id'/><arg type='u' name='timestamp'/></signal>"
	"</interface></node>";

static GVariant *circle(guint8 r, guint8 g, guint8 b) {
	enum { N = 32 };
	guint8 px[N * N * 4];
	const double c = (N - 1) / 2.0, rad = N / 2 - 2;
	for (int y = 0; y < N; y++) {
		for (int x = 0; x < N; x++) {
			double dx = x - c, dy = y - c, d = dx * dx + dy * dy;
			guint8 *p = px + (y * N + x) * 4;
			p[0] = d <= (rad - 1) * (rad - 1) ? 255 : d <= rad * rad ? 128 : 0;
			p[1] = r;
			p[2] = g;
			p[3] = b;
		}
	}
	GVariantBuilder bld;
	g_variant_builder_init(&bld, G_VARIANT_TYPE("a(iiay)"));
	g_variant_builder_add(&bld, "(ii@ay)", N, N, g_variant_new_fixed_array(G_VARIANT_TYPE_BYTE, px, sizeof px, 1));
	return g_variant_ref_sink(g_variant_builder_end(&bld));
}

static GVariant *no_pixmap(void) { return g_variant_new_array(G_VARIANT_TYPE("(iiay)"), NULL, 0); }

static GVariant *item_props(int id) {
	GVariantBuilder b;
	g_variant_builder_init(&b, G_VARIANT_TYPE_VARDICT);
	switch (id) {
	case ITEM_ROOT:
		g_variant_builder_add(&b, "{sv}", "children-display", g_variant_new_string("submenu"));
		break;
	case ITEM_CONNECT:
		g_variant_builder_add(&b, "{sv}", "label", g_variant_new_string("Подключиться"));
		g_variant_builder_add(&b, "{sv}", "enabled", g_variant_new_boolean(!connected_now));
		break;
	case ITEM_DISCONNECT:
		g_variant_builder_add(&b, "{sv}", "label", g_variant_new_string("Отключиться"));
		g_variant_builder_add(&b, "{sv}", "enabled", g_variant_new_boolean(connected_now));
		break;
	case ITEM_SEPARATOR:
	case ITEM_GAP:
		g_variant_builder_add(&b, "{sv}", "type", g_variant_new_string("separator"));
		break;
	case ITEM_QUIT:
		g_variant_builder_add(&b, "{sv}", "label", g_variant_new_string("Выход"));
		break;
	case ITEM_OPEN:
		g_variant_builder_add(&b, "{sv}", "label", g_variant_new_string("Открыть"));
		break;
	}
	return g_variant_builder_end(&b);
}

static GVariant *item(int id) {
	GVariantBuilder kids;
	g_variant_builder_init(&kids, G_VARIANT_TYPE("av"));
	if (id == ITEM_ROOT) {
		for (gsize i = 0; i < G_N_ELEMENTS(menu_order); i++) {
			g_variant_builder_add(&kids, "v", item(menu_order[i]));
		}
	}
	return g_variant_new("(i@a{sv}av)", id, item_props(id), &kids);
}

static void clicked(int id) {
	if (id == ITEM_OPEN) {
		qdOpen();
	} else if (id == ITEM_CONNECT || id == ITEM_DISCONNECT || id == ITEM_QUIT) {
		qdTrayAction(id);
	}
}

static GVariant *empty_ints(void) { return g_variant_new_array(G_VARIANT_TYPE_INT32, NULL, 0); }

static void menu_call(GDBusConnection *c, const gchar *sender, const gchar *path, const gchar *iface,
	const gchar *method, GVariant *params, GDBusMethodInvocation *inv, gpointer data) {
	if (g_str_equal(method, "GetLayout")) {
		gint32 parent;
		g_variant_get_child(params, 0, "i", &parent);
		g_dbus_method_invocation_return_value(inv, g_variant_new("(u@(ia{sv}av))", revision, item(parent)));
	} else if (g_str_equal(method, "GetGroupProperties")) {
		GVariant *ids = g_variant_get_child_value(params, 0);
		GVariantBuilder b;
		g_variant_builder_init(&b, G_VARIANT_TYPE("a(ia{sv})"));
		gsize n = g_variant_n_children(ids);
		for (gsize k = 0; k < (n ? n : ITEM_GAP + 1); k++) {
			gint32 id = (gint32)k;
			if (n) g_variant_get_child(ids, k, "i", &id);
			g_variant_builder_add(&b, "(i@a{sv})", id, item_props(id));
		}
		g_variant_unref(ids);
		g_dbus_method_invocation_return_value(inv, g_variant_new("(a(ia{sv}))", &b));
	} else if (g_str_equal(method, "GetProperty")) {
		gint32 id;
		const gchar *name;
		g_variant_get(params, "(i&s)", &id, &name);
		GVariant *props = g_variant_ref_sink(item_props(id));
		GVariant *v = g_variant_lookup_value(props, name, NULL);
		g_variant_unref(props);
		if (!v) {
			g_dbus_method_invocation_return_dbus_error(inv, MENU_IFACE ".Error", "no such property");
			return;
		}
		g_dbus_method_invocation_return_value(inv, g_variant_new("(v)", v));
		g_variant_unref(v);
	} else if (g_str_equal(method, "Event")) {
		gint32 id;
		const gchar *event;
		g_variant_get_child(params, 0, "i", &id);
		g_variant_get_child(params, 1, "&s", &event);
		if (g_str_equal(event, "clicked")) clicked(id);
		g_dbus_method_invocation_return_value(inv, NULL);
	} else if (g_str_equal(method, "EventGroup")) {
		GVariant *events = g_variant_get_child_value(params, 0);
		for (gsize k = 0; k < g_variant_n_children(events); k++) {
			GVariant *e = g_variant_get_child_value(events, k);
			gint32 id;
			const gchar *event;
			g_variant_get_child(e, 0, "i", &id);
			g_variant_get_child(e, 1, "&s", &event);
			if (g_str_equal(event, "clicked")) clicked(id);
			g_variant_unref(e);
		}
		g_variant_unref(events);
		g_dbus_method_invocation_return_value(inv, g_variant_new("(@ai)", empty_ints()));
	} else if (g_str_equal(method, "AboutToShow")) {
		g_dbus_method_invocation_return_value(inv, g_variant_new("(b)", FALSE));
	} else if (g_str_equal(method, "AboutToShowGroup")) {
		g_dbus_method_invocation_return_value(inv, g_variant_new("(@ai@ai)", empty_ints(), empty_ints()));
	} else {
		g_dbus_method_invocation_return_value(inv, NULL);
	}
}

static GVariant *menu_prop(GDBusConnection *c, const gchar *sender, const gchar *path, const gchar *iface,
	const gchar *name, GError **err, gpointer data) {
	if (g_str_equal(name, "Version")) return g_variant_new_uint32(3);
	if (g_str_equal(name, "TextDirection")) return g_variant_new_string("ltr");
	if (g_str_equal(name, "Status")) return g_variant_new_string("normal");
	return g_variant_new_strv(NULL, 0);
}

static void sni_call(GDBusConnection *c, const gchar *sender, const gchar *path, const gchar *iface,
	const gchar *method, GVariant *params, GDBusMethodInvocation *inv, gpointer data) {
	g_dbus_method_invocation_return_value(inv, NULL);
	if (g_str_equal(method, "Activate")) qdOpen();
}

static GVariant *sni_prop(GDBusConnection *c, const gchar *sender, const gchar *path, const gchar *iface,
	const gchar *name, GError **err, gpointer data) {
	if (g_str_equal(name, "Category")) return g_variant_new_string("ApplicationStatus");
	if (g_str_equal(name, "Id")) return g_variant_new_string("qd-client");
	if (g_str_equal(name, "Title")) return g_variant_new_string("qd");
	if (g_str_equal(name, "Status")) return g_variant_new_string("Active");
	if (g_str_equal(name, "WindowId")) return g_variant_new_int32(0);
	if (g_str_equal(name, "IconPixmap")) return g_variant_ref(pixmaps[status_now]);
	if (g_str_has_suffix(name, "Pixmap")) return no_pixmap();
	if (g_str_equal(name, "ToolTip")) {
		return g_variant_new("(s@a(iiay)ss)", "", no_pixmap(), tip_now ? tip_now : "qd", "");
	}
	if (g_str_equal(name, "ItemIsMenu")) return g_variant_new_boolean(FALSE);
	if (g_str_equal(name, "Menu")) return g_variant_new_object_path(MENU_PATH);
	return g_variant_new_string("");
}

static const GDBusInterfaceVTable sni_vtable = {sni_call, sni_prop, NULL};
static const GDBusInterfaceVTable menu_vtable = {menu_call, menu_prop, NULL};

static void registered(GObject *src, GAsyncResult *res, gpointer data) {
	GVariant *r = g_dbus_connection_call_finish(G_DBUS_CONNECTION(src), res, NULL);
	if (!r) return;
	g_variant_unref(r);
	g_atomic_int_set(&live, 1);
	if (!held) {
		held = 1;
		g_application_hold(G_APPLICATION(qd_app));
	}
}

static void watcher_up(GDBusConnection *c, const gchar *name, const gchar *owner, gpointer data) {
	g_dbus_connection_call(c, WATCHER, "/StatusNotifierWatcher", WATCHER, "RegisterStatusNotifierItem",
		g_variant_new("(s)", bus_name), NULL, G_DBUS_CALL_FLAGS_NONE, -1, NULL, registered, NULL);
}

static void watcher_gone(GDBusConnection *c, const gchar *name, gpointer data) {
	g_atomic_int_set(&live, 0);
	if (held) {
		held = 0;
		g_application_release(G_APPLICATION(qd_app));
	}
}

static void named(GDBusConnection *c, const gchar *name, gpointer data) {
	g_bus_watch_name_on_connection(c, WATCHER, G_BUS_NAME_WATCHER_FLAGS_NONE, watcher_up, watcher_gone, NULL, NULL);
}

void qd_tray_start(GDBusConnection *c) {
	if (!c) return;

	pixmaps[0] = circle(0x8a, 0x8a, 0x8a);
	pixmaps[1] = circle(0x34, 0xa8, 0x53);
	pixmaps[2] = circle(0xea, 0x43, 0x35);

	GDBusNodeInfo *sni = g_dbus_node_info_new_for_xml(sni_xml, NULL);
	GDBusNodeInfo *menu = g_dbus_node_info_new_for_xml(menu_xml, NULL);
	g_dbus_connection_register_object(c, SNI_PATH, sni->interfaces[0], &sni_vtable, NULL, NULL, NULL);
	g_dbus_connection_register_object(c, MENU_PATH, menu->interfaces[0], &menu_vtable, NULL, NULL, NULL);
	g_dbus_node_info_unref(sni);
	g_dbus_node_info_unref(menu);

	bus = c;
	bus_name = g_strdup_printf("org.kde.StatusNotifierItem-%d-1", (int)getpid());
	g_bus_own_name_on_connection(c, bus_name, G_BUS_NAME_OWNER_FLAGS_NONE, named, NULL, NULL, NULL);
}

struct qd_state {
	int status, connected;
	char *tip;
};

static gboolean apply_state(gpointer p) {
	struct qd_state *s = p;
	int icon = s->status != status_now;
	int tip = g_strcmp0(s->tip, tip_now) != 0;
	int menu = s->connected != connected_now;

	status_now = s->status;
	connected_now = s->connected;
	g_free(tip_now);
	tip_now = s->tip;
	g_free(s);

	if (bus) {
		if (icon) g_dbus_connection_emit_signal(bus, NULL, SNI_PATH, SNI_IFACE, "NewIcon", NULL, NULL);
		if (tip) g_dbus_connection_emit_signal(bus, NULL, SNI_PATH, SNI_IFACE, "NewToolTip", NULL, NULL);
		if (menu) {
			revision++;
			g_dbus_connection_emit_signal(bus, NULL, MENU_PATH, MENU_IFACE, "LayoutUpdated",
				g_variant_new("(ui)", revision, ITEM_ROOT), NULL);
		}
	}
	return G_SOURCE_REMOVE;
}

void qd_post_state(int status, const char *tip, int connected) {
	struct qd_state *s = g_new(struct qd_state, 1);
	s->status = status < 0 || status > 2 ? 0 : status;
	s->connected = connected;
	s->tip = g_strdup(tip);
	g_idle_add(apply_state, s);
}

int qd_tray_live(void) { return g_atomic_int_get(&live); }
