package ru.quicdiver.client;

import android.app.AppOpsManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.PowerManager;
import android.provider.Settings;

import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.List;

public final class Guard {

    public static final int ROM_STOCK = 0;
    public static final int ROM_XIAOMI = 1;
    public static final int ROM_SAMSUNG = 2;
    public static final int ROM_OPLUS = 3;
    public static final int ROM_HUAWEI = 4;
    public static final int ROM_VIVO = 5;

    public static final int NO = 0;
    public static final int YES = 1;
    public static final int UNSURE = 2;

    public static final int BATTERY = 0;
    public static final int VPN = 1;
    public static final int NOTIFY = 2;
    public static final int AUTOSTART = 3;

    private static final int MIUI_AUTOSTART = 10008;

    private static int known = -1;

    private Guard() {
    }

    public static int rom() {
        if (known < 0) {
            known = sniff();
        }
        return known;
    }

    private static int sniff() {
        if (!prop("ro.miui.ui.version.name").isEmpty()
                || !prop("ro.mi.os.version.name").isEmpty()
                || maker().contains("xiaomi") || maker().contains("redmi")) {
            return ROM_XIAOMI;
        }
        if (!prop("ro.build.version.oneui").isEmpty() || maker().contains("samsung")) {
            return ROM_SAMSUNG;
        }
        if (!prop("ro.build.version.oplusrom").isEmpty()
                || maker().contains("oneplus") || maker().contains("oppo") || maker().contains("realme")) {
            return ROM_OPLUS;
        }
        if (maker().contains("huawei") || maker().contains("honor")) {
            return ROM_HUAWEI;
        }
        if (maker().contains("vivo")) {
            return ROM_VIVO;
        }
        return ROM_STOCK;
    }

    public static int state(Context host, int what) {
        switch (what) {
            case BATTERY:
                return battery(host);
            case VPN:
                return alwaysOn(host);
            case NOTIFY:
                return notifying(host);
            case AUTOSTART:
                return autostart(host);
            default:
                return UNSURE;
        }
    }

    public static String said(int what, int state) {
        switch (what) {
            case BATTERY:
                return state == YES
                        ? "Приложение исключено из оптимизации батареи"
                        : "Приложение не исключено из оптимизации батареи";
            case VPN:
                if (state == UNSURE) {
                    return "Постоянный VPN — включается только вручную";
                }
                return state == YES ? "Постоянный VPN включен" : "Постоянный VPN выключен";
            case NOTIFY:
                return state == YES
                        ? "Уведомления приложения разрешены"
                        : "Уведомления приложения запрещены";
            case AUTOSTART:
                if (state == UNSURE) {
                    return "Автозапуск приложения — проверь вручную";
                }
                return state == YES
                        ? "Автозапуск приложения включен"
                        : "Автозапуск приложения выключен";
            default:
                return "";
        }
    }

    public static void open(Context host, int what) {
        switch (what) {
            case BATTERY:
                openBattery(host);
                break;
            case VPN:
                openVpn(host);
                break;
            case NOTIFY:
                openNotifications(host);
                break;
            case AUTOSTART:
                openAutostart(host);
                break;
            default:
                break;
        }
    }

    private static int batteryWas = -1;

    public static int battery(Context host) {
        PowerManager power = host.getSystemService(PowerManager.class);
        if (power == null) {
            return UNSURE;
        }
        if (Core.up() && batteryWas >= 0) {
            return batteryWas;
        }
        batteryWas = power.isIgnoringBatteryOptimizations(host.getPackageName()) ? YES : NO;
        return batteryWas;
    }

    public static int notifying(Context host) {
        if (Build.VERSION.SDK_INT < 33) {
            return YES;
        }
        return host.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)
                == PackageManager.PERMISSION_GRANTED ? YES : NO;
    }

    public static int alwaysOn(Context host) {
        try {
            String on = Settings.Secure.getString(host.getContentResolver(), "always_on_vpn_app");
            if (on == null) {
                return UNSURE;
            }
            return host.getPackageName().equals(on) ? YES : NO;
        } catch (Throwable ignored) {
            return UNSURE;
        }
    }

    public static int autostart(Context host) {
        if (rom() != ROM_XIAOMI) {
            return UNSURE;
        }
        try {
            AppOpsManager ops = host.getSystemService(AppOpsManager.class);
            Method check = AppOpsManager.class.getMethod(
                    "checkOpNoThrow", int.class, int.class, String.class);
            Object mode = check.invoke(ops, MIUI_AUTOSTART, android.os.Process.myUid(), host.getPackageName());
            if (!(mode instanceof Integer)) {
                return UNSURE;
            }
            return (Integer) mode == AppOpsManager.MODE_ALLOWED ? YES : NO;
        } catch (Throwable ignored) {
            return UNSURE;
        }
    }

    public static void openBattery(Context host) {
        Intent ask = new Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS);
        ask.setData(Uri.parse("package:" + host.getPackageName()));
        if (rom() != ROM_XIAOMI && open(host, ask)) {
            return;
        }
        if (open(host, new Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS))) {
            return;
        }
        if (open(host, ask)) {
            return;
        }
        openAppDetails(host);
    }

    public static void openVpn(Context host) {
        if (open(host, new Intent("android.net.vpn.SETTINGS"))) {
            return;
        }
        if (open(host, new Intent(Settings.ACTION_VPN_SETTINGS))) {
            return;
        }
        open(host, new Intent(Settings.ACTION_SETTINGS));
    }

    public static void openNotifications(Context host) {
        Intent channel = new Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS);
        channel.putExtra(Settings.EXTRA_APP_PACKAGE, host.getPackageName());
        if (open(host, channel)) {
            return;
        }
        openAppDetails(host);
    }

    public static void openAutostart(Context host) {
        for (Intent candidate : autostartCandidates(host)) {
            if (open(host, candidate)) {
                return;
            }
        }
        openAppDetails(host);
    }

    public static boolean openAppDetails(Context host) {
        Intent details = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS);
        details.setData(Uri.parse("package:" + host.getPackageName()));
        return open(host, details);
    }

    private static List<Intent> autostartCandidates(Context host) {
        List<Intent> out = new ArrayList<>();
        switch (rom()) {
            case ROM_XIAOMI:
                out.add(component("com.miui.securitycenter",
                        "com.miui.permcenter.autostart.AutoStartManagementActivity"));
                Intent keeper = component("com.miui.powerkeeper",
                        "com.miui.powerkeeper.ui.HiddenAppsConfigActivity");
                keeper.putExtra("package_name", host.getPackageName());
                keeper.putExtra("package_label", "qd");
                out.add(keeper);
                break;
            case ROM_SAMSUNG:
                out.add(component("com.samsung.android.lool",
                        "com.samsung.android.sm.battery.ui.BatteryActivity"));
                out.add(component("com.samsung.android.lool",
                        "com.samsung.android.sm.ui.battery.BatteryActivity"));
                out.add(component("com.samsung.android.sm",
                        "com.samsung.android.sm.ui.battery.BatteryActivity"));
                break;
            case ROM_OPLUS:
                out.add(component("com.oplus.safecenter",
                        "com.oplus.safecenter.startupapp.StartupAppListActivity"));
                out.add(component("com.coloros.safecenter",
                        "com.coloros.safecenter.startupapp.StartupAppListActivity"));
                out.add(component("com.coloros.safecenter",
                        "com.coloros.privacypermissionsentry.PermissionTopActivity"));
                out.add(component("com.oppo.safe",
                        "com.oppo.safe.permission.startup.StartupAppListActivity"));
                break;
            case ROM_HUAWEI:
                out.add(component("com.huawei.systemmanager",
                        "com.huawei.systemmanager.startupmgr.ui.StartupNormalAppListActivity"));
                out.add(component("com.huawei.systemmanager",
                        "com.huawei.systemmanager.optimize.process.ProtectActivity"));
                break;
            case ROM_VIVO:
                out.add(component("com.vivo.permissionmanager",
                        "com.vivo.permissionmanager.activity.BgStartUpManagerActivity"));
                out.add(component("com.iqoo.secure",
                        "com.iqoo.secure.ui.phoneoptimize.BgStartUpManager"));
                break;
            default:
                break;
        }
        return out;
    }

    private static Intent component(String pkg, String cls) {
        Intent out = new Intent();
        out.setComponent(new ComponentName(pkg, cls));
        return out;
    }

    private static boolean open(Context host, Intent what) {
        what.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            if (what.resolveActivity(host.getPackageManager()) == null) {
                return false;
            }
            host.startActivity(what);
            return true;
        } catch (Exception ignored) {
            return false;
        }
    }

    private static String maker() {
        return (Build.MANUFACTURER == null ? "" : Build.MANUFACTURER).toLowerCase();
    }

    private static String prop(String name) {
        try {
            Process p = Runtime.getRuntime().exec(new String[]{"getprop", name});
            java.io.BufferedReader in = new java.io.BufferedReader(
                    new java.io.InputStreamReader(p.getInputStream()));
            String line = in.readLine();
            in.close();
            return line == null ? "" : line.trim();
        } catch (Exception ignored) {
            return "";
        }
    }
}
