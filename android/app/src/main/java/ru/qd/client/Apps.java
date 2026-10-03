package ru.qd.client;

import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.drawable.Drawable;
import android.util.Base64;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

final class Apps {

    private static final int FACE = 64;

    private static final Map<String, String> faces = new ConcurrentHashMap<>();

    private Apps() {
    }

    static String json(Context host) {
        JSONArray out = new JSONArray();
        if (host == null) {
            return "[]";
        }

        PackageManager packages = host.getPackageManager();
        Set<String> launchable = launchable(packages);
        for (PackageInfo info : packages.getInstalledPackages(PackageManager.GET_PERMISSIONS)) {
            if (info.applicationInfo == null || info.packageName.equals(host.getPackageName())) {
                continue;
            }
            if (!networked(info) && !launchable.contains(info.packageName)) {
                continue;
            }
            try {
                JSONObject app = new JSONObject();
                app.put("name", info.packageName);
                app.put("title", String.valueOf(packages.getApplicationLabel(info.applicationInfo)));
                app.put("icon", face(packages, info.packageName));
                out.put(app);
            } catch (Exception ignored) {
            }
        }
        return out.toString();
    }

    private static Set<String> launchable(PackageManager packages) {
        Set<String> out = new HashSet<>();
        Intent home = new Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER);
        for (ResolveInfo found : packages.queryIntentActivities(home, 0)) {
            if (found.activityInfo != null) {
                out.add(found.activityInfo.packageName);
            }
        }
        return out;
    }

    private static boolean networked(PackageInfo info) {
        if (info.requestedPermissions == null) {
            return false;
        }
        for (String permission : info.requestedPermissions) {
            if ("android.permission.INTERNET".equals(permission)) {
                return true;
            }
        }
        return false;
    }

    private static String face(PackageManager packages, String pkg) {
        String known = faces.get(pkg);
        if (known != null) {
            return known;
        }

        String drawn = "";
        try {
            Drawable icon = packages.getApplicationIcon(pkg);
            Bitmap sheet = Bitmap.createBitmap(FACE, FACE, Bitmap.Config.ARGB_8888);
            icon.setBounds(0, 0, FACE, FACE);
            icon.draw(new Canvas(sheet));

            ByteArrayOutputStream png = new ByteArrayOutputStream();
            sheet.compress(Bitmap.CompressFormat.PNG, 100, png);
            sheet.recycle();
            drawn = "data:image/png;base64," + Base64.encodeToString(png.toByteArray(), Base64.NO_WRAP);
        } catch (Exception ignored) {
        }
        faces.put(pkg, drawn);
        return drawn;
    }
}
