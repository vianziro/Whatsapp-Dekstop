//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 gio-2.0
#include <gtk/gtk.h>
#include <gio/gio.h>
#include <stdlib.h>
#include <string.h>

static gboolean whatsappDeskInitialDark = FALSE;
static gboolean whatsappDeskThemeCaptured = FALSE;

static void setWhatsAppDeskGTKTheme(int mode) {
	GtkSettings* settings = gtk_settings_get_default();
	if (!settings) return;
	if (!whatsappDeskThemeCaptured) {
		g_object_get(settings, "gtk-application-prefer-dark-theme", &whatsappDeskInitialDark, NULL);
		whatsappDeskThemeCaptured = TRUE;
	}
	gboolean preferDark = mode < 0 ? whatsappDeskInitialDark : (mode == 1 ? TRUE : FALSE);
	g_object_set(settings, "gtk-application-prefer-dark-theme", preferDark, NULL);
}

// Per-monitor window placement for GTK. Monitor connectors (e.g. "DP-1")
// are stable across sessions, so each display remembers its own frame.
// The returned static buffer is valid only until the next call.
static char g_monitorNameBuf[64];

static const char* windowMonitorName(void* winPtr) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	GdkWindow* gw = win ? gtk_widget_get_window(win) : NULL;
	if (!gw) return "";
	GdkDisplay* display = gdk_window_get_display(gw);
	GdkMonitor* mon = gdk_display_get_monitor_at_window(display, gw);
	if (!mon) return "";
	const char* model = gdk_monitor_get_model(mon);
	const char* connector = gdk_monitor_get_connector(mon);
	if (connector && connector[0]) {
		snprintf(g_monitorNameBuf, sizeof(g_monitorNameBuf), "%s|%s", connector, model ? model : "");
	} else {
		snprintf(g_monitorNameBuf, sizeof(g_monitorNameBuf), "%s", model ? model : "");
	}
	return g_monitorNameBuf;
}

// Move/resize the window to a saved frame (x/y are window-manager coordinates).
static void moveWindowTo(void* winPtr, int x, int y, int w, int h) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return;
	gtk_window_move(GTK_WINDOW(win), x, y);
	gtk_window_resize(GTK_WINDOW(win), w, h);
}

// Read the current frame. Returns 1 when the window manager reports a real
// position, 0 otherwise (Wayland never exposes absolute positions, so callers
// must treat x/y as unusable there and restore only the size).
static int getWindowFrameLinux(void* winPtr, int* x, int* y, int* w, int* h) {
	GtkWidget* win = GTK_WIDGET(winPtr);
	if (!win) return 0;
	gtk_window_get_size(GTK_WINDOW(win), w, h);
	// gtk_window_get_position returns void and never reports a position on
	// Wayland, so pre-zero x/y and report success whenever a window exists.
	*x = 0; *y = 0;
	gtk_window_get_position(GTK_WINDOW(win), x, y);
	return 1;
}

// Verify a saved frame still overlaps a connected monitor's geometry.
static int frameOnSomeMonitor(int x, int y, int w, int h) {
	GdkDisplay* display = gdk_display_get_default();
	if (!display) return 0;
	int n = gdk_display_get_n_monitors(display);
	for (int i = 0; i < n; i++) {
		GdkRectangle geo;
		gdk_monitor_get_geometry(gdk_display_get_monitor(display, i), &geo);
		int iw = (x + w < geo.x + geo.width ? x + w : geo.x + geo.width) - (x > geo.x ? x : geo.x);
		int ih = (y + h < geo.y + geo.height ? y + h : geo.y + geo.height) - (y > geo.y ? y : geo.y);
		if (iw > 60 && ih > 40) return 1;
	}
	return 0;
}

// Always-on-top must go through GTK directly: wmctrl needs X11 and a window
// manager that honours _NET_WM_STATE, so it silently does nothing on GNOME
// Wayland (Fedora default), and wmctrl is not a dependency of our packages.
static void setWhatsAppDeskKeepAbove(int enable) {
	GtkWindow* top = NULL;
	GList* toplevels = gtk_window_list_toplevels();
	for (GList* l = toplevels; l != NULL; l = l->next) {
		GtkWindow* w = GTK_WINDOW(l->data);
		if (gtk_window_get_window_type(w) == GTK_WINDOW_TOPLEVEL && !gtk_window_get_transient_for(w)) {
			top = w;
		}
	}
	g_list_free(toplevels);
	if (top) {
		gtk_window_set_keep_above(top, enable ? TRUE : FALSE);
	}
}

// --- System Tray (StatusNotifierItem via DBus) ---

static GDBusConnection* g_dbus_conn = NULL;
static guint g_dbus_reg_id = 0;
static GDBusNodeInfo* g_introspection = NULL;
static char g_tray_icon_path[512] = {0};
static int g_tray_visible = 0;

static const gchar tray_introspection_xml[] =
	"<node>"
	"  <interface name='org.kde.StatusNotifierItem'>"
	"    <method name='Activate'>"
	"      <arg name='x' type='i' direction='in'/>"
	"      <arg name='y' type='i' direction='in'/>"
	"    </method>"
	"    <method name='SecondaryActivate'>"
	"      <arg name='x' type='i' direction='in'/>"
	"      <arg name='y' type='i' direction='in'/>"
	"    </method>"
	"    <method name='Scroll'>"
	"      <arg name='delta' type='i' direction='in'/>"
	"      <arg name='orientation' type='i' direction='in'/>"
	"    </method>"
	"    <method name='ContextMenu'>"
	"      <arg name='x' type='i' direction='in'/>"
	"      <arg name='y' type='i' direction='in'/>"
	"    </method>"
	"    <property name='Id' type='s' access='read'/>"
	"    <property name='Title' type='s' access='read'/>"
	"    <property name='Status' type='s' access='read'/>"
	"    <property name='IconName' type='s' access='read'/>"
	"    <property name='IconPixmap' type='a(iiay)' access='read'/>"
	"    <property name='OverlayIconName' type='s' access='read'/>"
	"    <property name='OverlayIconPixmap' type='a(iiay)' access='read'/>"
	"    <property name='AttentionIconName' type='s' access='read'/>"
	"    <property name='AttentionIconPixmap' type='a(iiay)' access='read'/>"
	"    <property name='AttentionMovieName' type='s' access='read'/>"
	"    <property name='ToolTip' type='s' access='read'/>"
	"    <property name='Category' type='s' access='read'/>"
	"    <property name='Menu' type='o' access='read'/>"
	"    <signal name='NewTitle'/>"
	"    <signal name='NewStatus'/>"
	"    <signal name='NewIcon'/>"
	"    <signal name='NewAttentionIcon'/>"
	"    <signal name='NewOverlayIcon'/>"
	"    <signal name='NewToolTip'/>"
	"  </interface>"
	"  <interface name='org.kde.StatusNotifierItem.Menu'>"
	"    <method name='AboutToShow'>"
	"      <arg name='parent' type='i' direction='in'/>"
	"    </method>"
	"    <method name='AboutToShowGroup'>"
	"      <arg name='parent' type='i' direction='in'/>"
	"      <arg name='group' type='i' direction='in'/>"
	"    </method>"
	"    <property name='Items' type='a(iisssis)' access='read'/>"
	"    <signal name='ItemsChanged'/>"
	"  </interface>"
	"  <interface name='org.freedesktop.DBus.Properties'>"
	"    <method name='Get'>"
	"      <arg name='interface' type='s' direction='in'/>"
	"      <arg name='property' type='s' direction='in'/>"
	"      <arg name='value' type='v' direction='out'/>"
	"    </method>"
	"    <method name='GetAll'>"
	"      <arg name='interface' type='s' direction='in'/>"
	"      <arg name='props' type='a{sv}' direction='out'/>"
	"    </method>"
	"    <method name='Set'>"
	"      <arg name='interface' type='s' direction='in'/>"
	"      <arg name='property' type='s' direction='in'/>"
	"      <arg name='value' type='v' direction='in'/>"
	"    </method>"
	"    <signal name='PropertiesChanged'>"
	"      <arg name='interface' type='s'/>"
	"      <arg name='changed_properties' type='a{sv}'/>"
	"      <arg name='invalidated_properties' type='as'/>"
	"    </signal>"
	"  </interface>"
	"</node>";

static GVariant* tray_build_menu_items(void);

static char g_overlay_icon_name[64] = {0};
static int g_has_overlay = 0;

static GVariant* tray_get_property(const gchar* interface, const gchar* property, GError** error) {
	if (strcmp(interface, "org.kde.StatusNotifierItem") == 0) {
		if (strcmp(property, "Id") == 0) {
			return g_variant_new_string("whatsapp-desk");
		}
		if (strcmp(property, "Title") == 0) {
			return g_variant_new_string("WhatsApp Desk");
		}
		if (strcmp(property, "Status") == 0) {
			return g_variant_new_string("Active");
		}
		if (strcmp(property, "Category") == 0) {
			return g_variant_new_string("ApplicationStatus");
		}
		if (strcmp(property, "IconName") == 0) {
			return g_variant_new_string("whatsapp-desk");
		}
		if (strcmp(property, "IconPixmap") == 0) {
			return g_variant_new_from_data(G_VARIANT_TYPE("a(iiay)"), NULL, 0, TRUE, NULL, NULL);
		}
		if (strcmp(property, "OverlayIconName") == 0) {
			if (g_has_overlay && g_overlay_icon_name[0]) {
				return g_variant_new_string(g_overlay_icon_name);
			}
			return g_variant_new_string("");
		}
		if (strcmp(property, "OverlayIconPixmap") == 0) {
			return g_variant_new_from_data(G_VARIANT_TYPE("a(iiay)"), NULL, 0, TRUE, NULL, NULL);
		}
		if (strcmp(property, "ToolTip") == 0) {
			return g_variant_new_string("WhatsApp Desk");
		}
		if (strcmp(property, "Menu") == 0) {
			// Return object path for menu
			return g_variant_new_object_path("/org/kde/StatusNotifierItem/Menu");
		}
	}
	if (strcmp(interface, "org.kde.StatusNotifierItem.Menu") == 0) {
		if (strcmp(property, "Items") == 0) {
			return tray_build_menu_items();
		}
	}
	g_set_error(error, G_DBUS_ERROR, G_DBUS_ERROR_UNKNOWN_PROPERTY, "Unknown property %s.%s", interface, property);
	return NULL;
}

static gboolean tray_method_call(GDBusConnection* conn, const gchar* sender, const gchar* object_path,
                                  const gchar* interface, const gchar* method,
                                  GVariant* params, GDBusMethodInvocation* invocation, gpointer user_data) {
	if (strcmp(interface, "org.kde.StatusNotifierItem") == 0) {
		if (strcmp(method, "Activate") == 0) {
			g_dbus_method_invocation_return_value(invocation, NULL);
			return TRUE;
		}
		if (strcmp(method, "ContextMenu") == 0) {
			g_dbus_method_invocation_return_value(invocation, NULL);
			return TRUE;
		}
	}
	if (strcmp(interface, "org.freedesktop.DBus.Properties") == 0) {
		if (strcmp(method, "Get") == 0) {
			const gchar* iface;
			const gchar* prop;
			g_variant_get(params, "(&s&s)", &iface, &prop);
			GError* error = NULL;
			GVariant* value = tray_get_property(iface, prop, &error);
			if (error) {
				g_dbus_method_invocation_return_error(invocation, G_DBUS_ERROR, G_DBUS_ERROR_UNKNOWN_PROPERTY, error->message);
				g_error_free(error);
			} else {
				g_dbus_method_invocation_return_value(invocation, g_variant_new_tuple(&value, 1));
			}
			return TRUE;
		}
		if (strcmp(method, "GetAll") == 0) {
			const gchar* iface;
			g_variant_get(params, "(&s)", &iface);
			GVariantBuilder builder;
			g_variant_builder_init(&builder, G_VARIANT_TYPE("a{sv}"));

			const gchar* props[] = {"Id", "Title", "Status", "Category", "IconName", "IconPixmap", "OverlayIconName", "OverlayIconPixmap", "ToolTip", "Menu", NULL};
			for (int i = 0; props[i]; i++) {
				GError* error = NULL;
				GVariant* value = tray_get_property(iface, props[i], &error);
				if (!error && value) {
					g_variant_builder_add(&builder, "{sv}", props[i], value);
				}
				if (error) g_error_free(error);
			}
			GVariant* dict = g_variant_builder_end(&builder);
			g_dbus_method_invocation_return_value(invocation, g_variant_new_tuple(&dict, 1));
			return TRUE;
		}
	}
	return FALSE;
}

static const GDBusInterfaceVTable tray_vtable = {
	.method_call = tray_method_call,
	.get_property = tray_get_property,
	.set_property = NULL,
};

static GVariant* tray_build_menu_items(void) {
	// Items format: (id, parent_id, type, label, icon, tooltip, enabled)
	// type: "standard", "check", "radio", "separator"
	GVariantBuilder builder;
	g_variant_builder_init(&builder, G_VARIANT_TYPE("a(iisssis)"));

	// Show Window
	g_variant_builder_add(&builder, "(iisssis)", 1, 0, "standard", "Show Window", "window-new", "", TRUE);
	// Separator
	g_variant_builder_add(&builder, "(iisssis)", 2, 0, "separator", "", "", "", FALSE);
	// Control Center
	g_variant_builder_add(&builder, "(iisssis)", 3, 0, "standard", "Control Center & Settings", "preferences-system", "", TRUE);
	// Separator
	g_variant_builder_add(&builder, "(iisssis)", 4, 0, "separator", "", "", "", FALSE);
	// Toggle Privacy Mode
	g_variant_builder_add(&builder, "(iisssis)", 5, 0, "check", "Privacy Mode", "security-high", "Blur chats and media", FALSE);
	// Toggle Always on Top
	g_variant_builder_add(&builder, "(iisssis)", 6, 0, "check", "Always on Top", "window-pinned", "Keep window above others", FALSE);
	// Toggle Mute Audio
	g_variant_builder_add(&builder, "(iisssis)", 7, 0, "check", "Mute Audio", "audio-volume-muted", "Mute notifications", FALSE);
	// Separator
	g_variant_builder_add(&builder, "(iisssis)", 8, 0, "separator", "", "", "", FALSE);
	// Open Downloads
	g_variant_builder_add(&builder, "(iisssis)", 9, 0, "standard", "Open Downloads Folder", "folder", "", TRUE);
	// Check Updates
	g_variant_builder_add(&builder, "(iisssis)", 10, 0, "standard", "Check for Updates", "system-software-update", "", TRUE);
	// Separator
	g_variant_builder_add(&builder, "(iisssis)", 11, 0, "separator", "", "", "", FALSE);
	// Quit
	g_variant_builder_add(&builder, "(iisssis)", 12, 0, "standard", "Quit WhatsApp Desk", "application-exit", "", TRUE);

	return g_variant_builder_end(&builder);
}

static void tray_update_overlay_icon(int count) {
	if (count > 0) {
		g_has_overlay = 1;
		snprintf(g_overlay_icon_name, sizeof(g_overlay_icon_name), "whatsapp-desk-unread-%d", count);
	} else {
		g_has_overlay = 0;
		g_overlay_icon_name[0] = 0;
	}
	// Emit NewOverlayIcon signal
	if (g_dbus_conn) {
		GError* error = NULL;
		g_dbus_connection_emit_signal(g_dbus_conn, NULL,
			"/org/kde/StatusNotifierItem",
			"org.kde.StatusNotifierItem",
			"NewOverlayIcon",
			NULL, &error);
		if (error) g_error_free(error);
	}
}

// Exported for Go
void tray_update_overlay_icon_go(int count) {
	tray_update_overlay_icon(count);
}

static void tray_on_bus_acquired(GDBusConnection* conn, const gchar* name, gpointer user_data) {
	g_dbus_conn = conn;
	g_introspection = g_dbus_node_info_new_for_xml(tray_introspection_xml, NULL);

	// Register StatusNotifierItem
	GError* error = NULL;
	g_dbus_reg_id = g_dbus_connection_register_object(conn,
		"/org/kde/StatusNotifierItem",
		g_introspection->interfaces[0],
		&tray_vtable,
		NULL, NULL, &error);
	if (error) {
		g_print("Failed to register StatusNotifierItem: %s\n", error->message);
		g_error_free(error);
	}

	// Register Menu
	g_dbus_connection_register_object(conn,
		"/org/kde/StatusNotifierItem/Menu",
		g_introspection->interfaces[1],
		&tray_vtable,
		NULL, NULL, NULL);

	// Register on StatusNotifierWatcher
	GVariant* params = g_variant_new("(ss)", "org.kde.StatusNotifierItem", "/org/kde/StatusNotifierItem");
	g_dbus_connection_call_sync(conn,
		"org.kde.StatusNotifierWatcher",
		"/StatusNotifierWatcher",
		"org.kde.StatusNotifierWatcher",
		"RegisterStatusNotifierItem",
		params, NULL, G_DBUS_CALL_FLAGS_NONE, -1, NULL, NULL);

	g_tray_visible = 1;
}

static void tray_on_name_lost(GDBusConnection* conn, const gchar* name, gpointer user_data) {
	if (g_dbus_reg_id) {
		g_dbus_connection_unregister_object(conn, g_dbus_reg_id);
		g_dbus_reg_id = 0;
	}
	if (g_introspection) {
		g_dbus_node_info_unref(g_introspection);
		g_introspection = NULL;
	}
	g_tray_visible = 0;
}

static void tray_init(const char* icon_path) {
	if (icon_path && icon_path[0]) {
		strncpy(g_tray_icon_path, icon_path, sizeof(g_tray_icon_path) - 1);
	}

	g_bus_own_name(G_BUS_TYPE_SESSION,
		"org.kde.StatusNotifierItem-whatsapp-desk",
		G_BUS_NAME_OWNER_FLAGS_NONE,
		tray_on_bus_acquired,
		NULL,
		tray_on_name_lost,
		NULL, NULL);
}

static void tray_update_icon(const char* icon_path) {
	if (icon_path && icon_path[0] && g_dbus_conn && g_tray_visible) {
		strncpy(g_tray_icon_path, icon_path, sizeof(g_tray_icon_path) - 1);
		// Emit NewIcon signal
		GError* error = NULL;
		g_dbus_connection_emit_signal(g_dbus_conn, NULL,
			"/org/kde/StatusNotifierItem",
			"org.kde.StatusNotifierItem",
			"NewIcon",
			NULL, &error);
		if (error) g_error_free(error);
	}
}

static void tray_shutdown(void) {
	if (g_dbus_conn && g_tray_visible) {
		// Unregister from watcher
		GVariant* params = g_variant_new("(s)", "/org/kde/StatusNotifierItem");
		g_dbus_connection_call_sync(g_dbus_conn,
			"org.kde.StatusNotifierWatcher",
			"/StatusNotifierWatcher",
			"org.kde.StatusNotifierWatcher",
			"UnregisterStatusNotifierItem",
			params, NULL, G_DBUS_CALL_FLAGS_NONE, -1, NULL, NULL);

		if (g_dbus_reg_id) {
			g_dbus_connection_unregister_object(g_dbus_conn, g_dbus_reg_id);
			g_dbus_reg_id = 0;
		}
		if (g_introspection) {
			g_dbus_node_info_unref(g_introspection);
			g_introspection = NULL;
		}
		g_tray_visible = 0;
	}
}
*/
import "C"

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"
)

//go:embed icon.png
var embeddedIconPNG []byte

const (
	userAgentLinux = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
)

var (
	isAlwaysOnTopLinux = false

	linuxCacheHomes   []string
	linuxPurgeTargets []string
)

func applyNativeThemeLinux(theme string) {
	mode := C.int(-1)
	if theme == "dark" {
		mode = 1
	} else if theme == "light" {
		mode = 0
	}
	C.setWhatsAppDeskGTKTheme(mode)
}

func checkSingleInstance() (*os.File, bool) {
	dataDir := getUserDataDir()
	lockPath := filepath.Join(dataDir, "app.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, true
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Already running
		return nil, false
	}
	return file, true
}

func getUserDataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	dir := filepath.Join(configDir, "WhatsAppDesk")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func ensureAppIconFileLinux(dir string) string {
	iconPath := filepath.Join(dir, "app_icon.png")
	if _, err := os.Stat(iconPath); os.IsNotExist(err) && len(embeddedIconPNG) > 0 {
		_ = os.WriteFile(iconPath, embeddedIconPNG, 0644)
	}
	return iconPath
}

func showNativeNotification(title, message, iconPath string) {
	if iconPath != "" {
		_ = exec.Command("notify-send", "-a", "WhatsApp Desk", "-i", iconPath, title, message).Run()
	} else {
		_ = exec.Command("notify-send", "-a", "WhatsApp Desk", title, message).Run()
	}
}

func toggleAlwaysOnTopLinux() bool {
	isAlwaysOnTopLinux = !isAlwaysOnTopLinux
	// Native GTK path works on both X11 and Wayland; wmctrl is only a last
	// resort for exotic setups and must never be the primary mechanism.
	C.setWhatsAppDeskKeepAbove(C.int(b2i(isAlwaysOnTopLinux)))
	if !isAlwaysOnTopLinux {
		if path, err := exec.LookPath("wmctrl"); err == nil && path != "" {
			_ = exec.Command("wmctrl", "-r", windowTitle, "-b", "remove,above").Run()
		}
	}
	return isAlwaysOnTopLinux
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// System Tray bindings
func initSystemTrayLinux(iconPath string) {
	cPath := C.CString(iconPath)
	defer C.free(unsafe.Pointer(cPath))
	C.tray_init(cPath)
}

func shutdownSystemTrayLinux() {
	C.tray_shutdown()
}

func getAutoStartDesktopPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "autostart", "whatsapp-desk.desktop")
}

func toggleAutoStartLinux() bool {
	p := getAutoStartDesktopPath()
	if p == "" {
		return false
	}
	if _, err := os.Stat(p); err == nil {
		_ = os.Remove(p)
		return false
	}

	execPath, err := os.Executable()
	if err != nil {
		return false
	}

	_ = os.MkdirAll(filepath.Dir(p), 0755)
	desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=WhatsApp Desk
Comment=Lightweight WhatsApp Desktop Client
Exec=%s
Icon=whatsapp-desk
Terminal=false
Categories=Network;InstantMessaging;
StartupNotify=true
`, execPath)

	err = os.WriteFile(p, []byte(desktopContent), 0644)
	return err == nil
}

func loadWindowState(dir string) *WindowState {
	path := filepath.Join(dir, "window_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	if state.Width < 450 || state.Height < 320 {
		return nil
	}
	return &state
}

var lastSavedStateLinux *WindowState

func saveWindowState(dir string, width, height int) {
	if width >= 450 && height >= 320 {
		if lastSavedStateLinux != nil &&
			lastSavedStateLinux.Width == float64(width) &&
			lastSavedStateLinux.Height == float64(height) {
			return // Avoid redundant disk writes
		}
		state := WindowState{
			Width:  float64(width),
			Height: float64(height),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		if err == nil {
			_ = os.WriteFile(filepath.Join(dir, "window_state.json"), data, 0644)
			lastSavedStateLinux = &state
		}
	}
}

func runApp() {
	lockFile, isSingle := checkSingleInstance()
	if !isSingle {
		fmt.Println("WhatsApp Desk is already running.")
		os.Exit(0)
	}
	if lockFile != nil {
		defer lockFile.Close()
	}

	userDataDir := getUserDataDir()
	cacheDebugLog("startup: pid=%d profile=%s", os.Getpid(), userDataDir)
	// WebKitGTK writes its network disk cache under ~/.cache keyed by app name.
	// Also covers WebKitGTK 4.0/4.1 (webkitgtk-4.0) and 6.0 (webkitgtk-6.0) cache directories.
	home, _ := os.UserHomeDir()
	linuxCacheHomes = []string{
		filepath.Join(home, ".cache", "whatsapp-desk"),
		filepath.Join(home, ".cache", "WhatsAppDesk"),
		filepath.Join(home, ".cache", "webkitgtk-4.0"),
		filepath.Join(home, ".cache", "webkitgtk-6.0"),
	}
	// Purge targets include NetworkCache subdirectories where HTTP cache lives
	linuxPurgeTargets = []string{
		filepath.Join(home, ".cache", "whatsapp-desk"),
		filepath.Join(home, ".cache", "WhatsAppDesk"),
		filepath.Join(home, ".cache", "webkitgtk-4.0", "NetworkCache"),
		filepath.Join(home, ".cache", "webkitgtk-6.0", "NetworkCache"),
	}
	enforceDiskCacheCapFrom(linuxCacheHomes, linuxPurgeTargets, "startup")

	// Restore window state if previously saved
	initialWidth := windowWidth
	initialHeight := windowHeight
	state := loadWindowState(userDataDir)
	if state != nil {
		initialWidth = int(state.Width)
		initialHeight = int(state.Height)
	}

	w := webview.New(false)
	if w == nil {
		log.Fatalln("Gagal inisialisasi WebKitGTK Webview")
	}
	defer w.Destroy()
	applyNativeThemeLinux(loadSettings().Theme)

	w.SetTitle(windowTitle)
	w.SetSize(initialWidth, initialHeight, webview.HintNone)

	// Bind window state saver from JS resize events
	_ = w.Bind("saveWindowStateNative", func(width, height int) {
		saveWindowState(userDataDir, width, height)
	})

	iconPath := ensureAppIconFileLinux(userDataDir)

	// Initialize system tray
	initSystemTrayLinux(iconPath)
	defer shutdownSystemTrayLinux()

	// Bind native notification bridge
	_ = w.Bind("sendNativeNotification", func(title, body string) {
		go showNativeNotification(title, body, iconPath)
	})

	_ = w.Bind("releaseMemoryNative", func() {
		debug.FreeOSMemory()
		debugLogProcessStats("release-memory")
		enforceDiskCacheCapFrom(linuxCacheHomes, linuxPurgeTargets, "minimize")
	})

	// Bind external link handler (xdg-open)
	_ = w.Bind("openExternalLink", func(rawURL string) {
		if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
			go func() {
				_ = exec.Command("xdg-open", rawURL).Start()
			}()
		}
	})

	// Bind dock badge -> StatusNotifierItem overlay icon (unread count)
	_ = w.Bind("updateDockBadge", func(badge string) {
		count := 0
		if strings.TrimSpace(badge) != "" {
			fmt.Sscanf(strings.TrimSpace(badge), "%d", &count)
			if count == 0 {
				count = 1 // non-numeric badge (e.g. "•") still means unread
			}
		}
		C.tray_update_overlay_icon_go(C.int(count))
	})

	// Bind Always on Top toggle
	_ = w.Bind("toggleAlwaysOnTopNative", func() bool {
		return toggleAlwaysOnTopLinux()
	})

	// Bind Auto-Start toggle
	_ = w.Bind("toggleAutoStartNative", func() bool {
		return toggleAutoStartLinux()
	})

	// Bind in-app auto updater
	_ = w.Bind("checkForUpdateNative", func(manual bool) UpdateInfo {
		info, err := checkForUpdate(appVersion)
		if err != nil {
			return UpdateInfo{CurrentVersion: appVersion, CheckError: err.Error()}
		}
		return *info
	})

	_ = w.Bind("startUpdateNative", func(downloadURL string) {
		go func() {
			_ = executeUpdate(w, downloadURL)
		}()
	})

	// Bind download, preview, and settings handlers
	_ = w.Bind("saveDownloadedFileNative", func(filename, dataURI string) string {
		path, err := saveDownloadedFile(filename, dataURI)
		if err != nil {
			return ""
		}
		return path
	})

	_ = w.Bind("previewDocumentNative", func(filename, dataURI string) string {
		path, err := previewDocument(filename, dataURI)
		if err != nil {
			return ""
		}
		return path
	})

	_ = w.Bind("openFileNative", func(filePath string) bool {
		return openFileInDefaultApp(filePath)
	})

	// Lazy-load SheetJS library for spreadsheet preview
	_ = w.Bind("loadXLSXLibraryNative", func() string {
		return xlsxLibJS
	})

	_ = w.Bind("getDownloadDirNative", func() string {
		s := loadSettings()
		return s.DownloadDir
	})

	_ = w.Bind("getOrganizeByMonthNative", func() bool {
		return loadSettings().OrganizeByMonth
	})

	_ = w.Bind("setOrganizeByMonthNative", func(on bool) bool {
		return setOrganizeByMonth(on)
	})

	_ = w.Bind("checkFileExistsNative", func(filename string) bool {
		return fileExistsInDownloadDir(filename)
	})

	_ = w.Bind("chooseDownloadDirNative", func() string {
		selected, err := chooseFolderDialog()
		if err != nil || selected == "" {
			return ""
		}
		s := loadSettings()
		s.DownloadDir = selected
		_ = saveSettings(s)
		return selected
	})

	_ = w.Bind("openDownloadDirNative", func() bool {
		s := loadSettings()
		_ = openFolderInFileManager(s.DownloadDir)
		return true
	})

	_ = w.Bind("resetDownloadDirNative", func() string {
		s := loadSettings()
		s.DownloadDir = getDefaultDownloadDir()
		_ = saveSettings(s)
		return s.DownloadDir
	})

	_ = w.Bind("getAppThemeNative", func() string {
		s := loadSettings()
		return s.Theme
	})

	_ = w.Bind("setAppThemeNative", func(theme string) string {
		saved := saveTheme(theme)
		applyNativeThemeLinux(saved)
		return saved
	})

	// Spell check bindings
	_ = w.Bind("getSpellCheckEnabledNative", func() bool {
		return getSpellCheckEnabled()
	})
	_ = w.Bind("setSpellCheckEnabledNative", func(enabled bool) bool {
		return setSpellCheckEnabled(enabled)
	})
	_ = w.Bind("getSpellCheckLangNative", func() string {
		return getSpellCheckLang()
	})
	_ = w.Bind("setSpellCheckLangNative", func(lang string) string {
		return setSpellCheckLang(lang)
	})
	_ = w.Bind("getBlurAvatarsNative", func() bool {
		return getBlurAvatars()
	})
	_ = w.Bind("setBlurAvatarsNative", func(on bool) bool {
		return setBlurAvatars(on)
	})

	w.Init(getInitScript(userAgentLinux))
	w.Navigate(appURL)

	// Check for updates in the background after startup & periodically
	go guardGoroutine("update-ticker", func() {
		checkAndNotifyUpdate := func() {
			info, err := checkForUpdate(appVersion)
			if err == nil && info != nil && info.Available {
				w.Dispatch(func() {
					script := fmt.Sprintf("if (window.showUpdateBanner) { window.showUpdateBanner(%q, %q, %q); }",
						info.LatestVersion, info.ReleaseTitle, info.DownloadURL)
					w.Eval(script)
				})
			}
		}

		time.Sleep(5 * time.Second)
		checkAndNotifyUpdate()

		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			checkAndNotifyUpdate()
		}
	})

	defer saveWindowState(userDataDir, initialWidth, initialHeight)
	w.Run()
}
