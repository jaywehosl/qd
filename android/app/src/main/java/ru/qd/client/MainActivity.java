package ru.qd.client;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.ContentValues;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.content.res.Configuration;
import android.graphics.Color;
import android.graphics.Insets;
import android.net.Uri;
import android.net.VpnService;
import android.os.Bundle;
import android.os.Environment;
import android.os.SystemClock;
import android.provider.MediaStore;
import android.text.format.DateFormat;
import android.util.Log;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;

import qdmobile.Client;

public class MainActivity extends Activity {

    private static final int CONSENT = 2;
    private static final int PICK = 3;

    private static final String SHELF = "face";
    private static final String INK = "ink";

    private static final String ASKED = "asked";

    private static final String[] CHECKS = {"notify", "battery", "vpn", "autostart"};
    private static final int[] WHAT = {Guard.NOTIFY, Guard.BATTERY, Guard.VPN, Guard.AUTOSTART};

    static volatile MainActivity live;

    private FrameLayout shell;
    private WebView web;
    private String home = "";
    private String wanted = "";
    private boolean loaded;
    private boolean shown;
    private long born;
    private volatile String link = "";
    private volatile String seat = "0,0";
    private ValueCallback<Uri[]> picking;

    private final class Bridge {
        @JavascriptInterface
        public String clipboard() {
            try {
                ClipData held = getSystemService(ClipboardManager.class).getPrimaryClip();
                if (held == null || held.getItemCount() == 0) {
                    return "";
                }
                return String.valueOf(held.getItemAt(0).coerceToText(MainActivity.this));
            } catch (Exception e) {
                return "";
            }
        }

        @JavascriptInterface
        public void paint(final String colour) {
            runOnUiThread(new Runnable() {
                @Override
                public void run() {
                    int shade = Color.WHITE;
                    try {
                        String[] parts = colour.replaceAll("[^0-9,.]", "").split(",");
                        shade = Color.rgb(Integer.parseInt(parts[0]), Integer.parseInt(parts[1]),
                                Integer.parseInt(parts[2]));
                    } catch (Exception ignored) {
                    }
                    getSharedPreferences(SHELF, MODE_PRIVATE).edit().putInt(INK, shade).apply();
                    tint(shade);
                }
            });
        }

        @JavascriptInterface
        public String insets() {
            return seat;
        }

        @JavascriptInterface
        public void ready() {
            runOnUiThread(new Runnable() {
                @Override
                public void run() {
                    reveal();
                }
            });
        }

        @JavascriptInterface
        public String pending() {
            String held = link;
            link = "";
            return held;
        }

        @JavascriptInterface
        public boolean dark() {
            return (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)
                    == Configuration.UI_MODE_NIGHT_YES;
        }

        @JavascriptInterface
        public String checks() {
            JSONArray out = new JSONArray();
            for (int i = 0; i < CHECKS.length; i++) {
                if (WHAT[i] == Guard.AUTOSTART && Guard.rom() == Guard.ROM_STOCK) {
                    continue;
                }
                int state = Guard.state(MainActivity.this, WHAT[i]);
                try {
                    out.put(new JSONObject()
                            .put("id", CHECKS[i])
                            .put("state", state == Guard.YES ? "yes" : state == Guard.NO ? "no" : "unsure"));
                } catch (Exception ignored) {
                }
            }
            return out.toString();
        }

        @JavascriptInterface
        public void open(final String id) {
            runOnUiThread(new Runnable() {
                @Override
                public void run() {
                    for (int i = 0; i < CHECKS.length; i++) {
                        if (CHECKS[i].equals(id)) {
                            Guard.open(MainActivity.this, WHAT[i]);
                        }
                    }
                }
            });
        }

        @JavascriptInterface
        public String journal() {
            try {
                Client client = Core.client(MainActivity.this);
                String path = client.logPath();
                File from = path == null || path.isEmpty() ? null : new File(path);
                if (from == null || !from.exists() || from.length() == 0) {
                    return "The journal is empty";
                }
                String name = "qd-" + DateFormat.format("MMdd-HHmmss", System.currentTimeMillis()) + ".log.txt";
                return shelve(name, new File(from.getPath() + ".1"), from);
            } catch (Exception e) {
                return "Not saved: " + e.getMessage();
            }
        }

        @JavascriptInterface
        public String save(String name, String text) {
            return put(name, text.getBytes(StandardCharsets.UTF_8));
        }

        @JavascriptInterface
        public String saveData(String name, String base64) {
            try {
                return put(name, android.util.Base64.decode(base64, android.util.Base64.DEFAULT));
            } catch (Exception e) {
                return "Not saved: " + e.getMessage();
            }
        }

        private String put(String name, byte[] bytes) {
            try {
                File draft = File.createTempFile("save", null, getCacheDir());
                try (OutputStream out = new java.io.FileOutputStream(draft)) {
                    out.write(bytes);
                }
                String said = shelve(name, draft);
                draft.delete();
                return said;
            } catch (Exception e) {
                return "Not saved: " + e.getMessage();
            }
        }
    }

    private String shelve(String name, File... parts) throws Exception {
        ContentValues row = new ContentValues();
        row.put(MediaStore.Downloads.DISPLAY_NAME, name);
        row.put(MediaStore.Downloads.MIME_TYPE, name.endsWith(".txt") ? "text/plain" : "application/octet-stream");
        row.put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS);

        Uri put = getContentResolver().insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, row);
        if (put == null) {
            return "Not saved: no file";
        }

        long written = 0;
        byte[] buf = new byte[8192];
        try (OutputStream out = getContentResolver().openOutputStream(put)) {
            for (File part : parts) {
                if (!part.exists()) {
                    continue;
                }
                try (InputStream in = new FileInputStream(part)) {
                    int n;
                    while ((n = in.read(buf)) > 0) {
                        out.write(buf, 0, n);
                        written += n;
                    }
                }
            }
        }
        return "Downloads/" + name + " (" + Math.max(1, written / 1024) + " KB)";
    }

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        born = SystemClock.uptimeMillis();
        live = this;

        shell = new FrameLayout(this);
        shell.setOnApplyWindowInsetsListener(new View.OnApplyWindowInsetsListener() {
            @Override
            public WindowInsets onApplyWindowInsets(View view, WindowInsets insets) {
                Insets bars = insets.getInsets(
                        WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
                int keys = insets.getInsets(WindowInsets.Type.ime()).bottom;
                float dense = getResources().getDisplayMetrics().density;
                String fresh = Math.round(bars.top / dense) + "," + Math.round(Math.max(keys, bars.bottom) / dense)
                        + "," + Math.round(bars.left / dense) + "," + Math.round(bars.right / dense);
                if (!fresh.equals(seat)) {
                    seat = fresh;
                    poke();
                }
                return new WindowInsets.Builder(insets).setInsets(WindowInsets.Type.ime(), Insets.NONE).build();
            }
        });
        tint(getSharedPreferences(SHELF, MODE_PRIVATE).getInt(INK, Color.WHITE));

        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
            WebView.setWebContentsDebuggingEnabled(true);
        }
        web = new WebView(this);
        web.setBackgroundColor(Color.TRANSPARENT);
        web.setOverScrollMode(View.OVER_SCROLL_NEVER);
        web.setVerticalScrollBarEnabled(false);
        web.setHorizontalScrollBarEnabled(false);
        web.setAlpha(0f);
        WebSettings set = web.getSettings();
        set.setJavaScriptEnabled(true);
        set.setDomStorageEnabled(true);
        set.setTextZoom(100);
        set.setSupportZoom(false);
        web.addJavascriptInterface(new Bridge(), "qdHost");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri where = request.getUrl();
                if (home.equals(where.getScheme() + "://" + where.getAuthority())) {
                    return false;
                }
                try {
                    startActivity(new Intent(Intent.ACTION_VIEW, where));
                } catch (Exception ignored) {
                }
                return true;
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                loaded = true;
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> back,
                                             FileChooserParams params) {
                if (picking != null) {
                    picking.onReceiveValue(null);
                }
                picking = back;
                Intent pick = new Intent(Intent.ACTION_GET_CONTENT);
                pick.addCategory(Intent.CATEGORY_OPENABLE);
                pick.setType("*/*");
                try {
                    startActivityForResult(pick, PICK);
                } catch (Exception e) {
                    picking = null;
                    back.onReceiveValue(null);
                }
                return true;
            }
        });

        shell.addView(web, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        setContentView(shell);
        shell.postDelayed(new Runnable() {
            @Override
            public void run() {
                reveal();
            }
        }, 4000);

        handle(getIntent());
        Notes.wake(this);

        SharedPreferences shelf = getSharedPreferences(SHELF, MODE_PRIVATE);
        if (Guard.notifying(this) != Guard.YES && !shelf.getBoolean(ASKED, false)) {
            shelf.edit().putBoolean(ASKED, true).apply();
            requestPermissions(new String[]{android.Manifest.permission.POST_NOTIFICATIONS}, 1);
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Client client = Core.client(MainActivity.this);
                    final String page = client.page();
                    runOnUiThread(new Runnable() {
                        @Override
                        public void run() {
                            Uri at = Uri.parse(page);
                            home = at.getScheme() + "://" + at.getAuthority();
                            web.loadUrl(wanted.isEmpty() ? page
                                    : at.buildUpon().path(wanted).build().toString());
                        }
                    });

                    Core.readExit(MainActivity.this);
                    String behaviour = new JSONObject(client.settingsJSON()).optString("manualBehaviour", "open");
                    if (Core.ready() && !Core.up()
                            && ("connect".equals(behaviour) || "openConnect".equals(behaviour))) {
                        consent();
                    }
                } catch (Exception e) {
                    Log.e(TunnelService.TAG, "page", e);
                }
            }
        }).start();
    }

    private void reveal() {
        if (shown) {
            return;
        }
        shown = true;
        Core.say(this, "java: page drawn in " + (SystemClock.uptimeMillis() - born) + " ms");
        web.animate().alpha(1f).setDuration(180).start();
    }

    private void tint(int shade) {
        shell.setBackgroundColor(shade);
        getWindow().getDecorView().setBackgroundColor(shade);

        WindowInsetsController bars = getWindow().getInsetsController();
        if (bars == null) {
            return;
        }
        int light = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS
                | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
        bars.setSystemBarsAppearance(Color.luminance(shade) > 0.5f ? light : 0, light);
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handle(intent);
    }

    private void handle(Intent intent) {
        if (intent == null) {
            return;
        }
        if (android.service.quicksettings.TileService.ACTION_QS_TILE_PREFERENCES.equals(intent.getAction())) {
            go("/client/settings");
        }

        Uri data = intent.getData();
        if (data != null && "qd".equals(data.getScheme())) {
            link = intent.getDataString();
            go("/client");
            poke();
        }
    }

    private void go(String path) {
        if (!loaded) {
            wanted = path;
            return;
        }
        web.evaluateJavascript("if (location.pathname !== '" + path + "') { history.pushState({}, '', '"
                + path + "'); dispatchEvent(new PopStateEvent('popstate')); }", null);
    }

    static void poke() {
        final MainActivity face = live;
        if (face == null) {
            return;
        }
        face.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                if (face.loaded) {
                    face.web.evaluateJavascript("dispatchEvent(new Event('qd-host'))", null);
                }
            }
        });
    }

    boolean consent() {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                Intent ask = VpnService.prepare(MainActivity.this);
                if (ask == null) {
                    begin();
                    return;
                }
                startActivityForResult(ask, CONSENT);
            }
        });
        return true;
    }

    private void begin() {
        Intent up = new Intent(this, TunnelService.class);
        up.setAction(TunnelService.ACTION_START);
        startService(up);
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == PICK) {
            ValueCallback<Uri[]> back = picking;
            picking = null;
            if (back != null) {
                Uri got = result == RESULT_OK && data != null ? data.getData() : null;
                back.onReceiveValue(got == null ? null : new Uri[]{got});
            }
            return;
        }
        if (request != CONSENT) {
            return;
        }
        if (result == RESULT_OK) {
            begin();
            return;
        }
        try {
            Core.client(this).declined();
        } catch (Exception ignored) {
        }
    }

    @Override
    public void onConfigurationChanged(Configuration fresh) {
        super.onConfigurationChanged(fresh);
        poke();
    }

    @Override
    public void onBackPressed() {
        if (!loaded) {
            moveTaskToBack(true);
            return;
        }
        web.evaluateJavascript("window.qdBack ? qdBack() : false", new ValueCallback<String>() {
            @Override
            public void onReceiveValue(String closed) {
                if (!"true".equals(closed)) {
                    moveTaskToBack(true);
                }
            }
        });
    }

    @Override
    protected void onResume() {
        super.onResume();
        web.onResume();
        poke();
    }

    @Override
    protected void onPause() {
        web.onPause();
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        if (live == this) {
            live = null;
        }
        web.destroy();
        super.onDestroy();
    }
}
