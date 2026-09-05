package ru.quicdiver.client;

import android.Manifest;
import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Insets;
import android.net.Uri;
import android.net.VpnService;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.widget.FrameLayout;

import org.json.JSONObject;



public class MainActivity extends Activity {

    private static final int NOTIFY = 1;
    private static final int CONSENT = 2;

    private static final int ROUTING = 0;
    private static final int CONNECT = 1;
    private static final int SETTINGS = 2;

    private Skin skin;
    private FrameLayout shell;
    private Pager pages;

    private ConnectPage connectPage;
    private RoutingPage routingPage;
    private SettingsPage settingsPage;
    private ImportPage importPage;
    private int shown = -1;
    private int linked = -1;

    private final Handler ticker = new Handler(Looper.getMainLooper());
    private final Runnable tick = new Runnable() {
        @Override
        public void run() {
            draw();
            ticker.postDelayed(this, 1000);
        }
    };

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        skin = new Skin(this);

        pages = new Pager(this);
        pages.setBackground(skin.backdrop());

        routingPage = new RoutingPage(this, skin);
        connectPage = new ConnectPage(this, skin);
        settingsPage = new SettingsPage(this, skin, new Runnable() {
            @Override
            public void run() {
                surface();
            }
        });

        pages.add(sheet(routingPage.build()));
        pages.add(sheet(connectPage.build()));
        pages.add(sheet(settingsPage.build()));
        pages.show(CONNECT);
        pages.onSettle(new Runnable() {
            @Override
            public void run() {
                draw();
            }
        });

        importPage = new ImportPage(this, skin, new Runnable() {
            @Override
            public void run() {
                surface();
            }
        });

        shell = new FrameLayout(this);
        shell.setBackground(skin.backdrop());
        shell.setOnApplyWindowInsetsListener(new View.OnApplyWindowInsetsListener() {
            @Override
            public WindowInsets onApplyWindowInsets(View view, WindowInsets insets) {
                seat(insets.getInsets(
                        WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout()));
                return insets;
            }
        });
        setContentView(shell);

        Snapshot.start(this);
        forget();

        handle(getIntent());
        surface();
        askNotifications();
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handle(intent);
    }

    private void handle(Intent intent) {
        Uri data = intent == null ? null : intent.getData();
        if (data != null && "qd".equals(data.getScheme())) {
            importPage.build();
            importPage.adopt(intent.getDataString());
        }
    }

    private static class Sheet {
        View view;
        int top;
        int bottom;
    }

    private final java.util.List<Sheet> sheets = new java.util.ArrayList<>();
    private Insets bars = Insets.NONE;

    private View sheet(View view) {
        for (Sheet known : sheets) {
            if (known.view == view) {
                return view;
            }
        }
        Sheet held = new Sheet();
        held.view = view;
        held.top = view.getPaddingTop();
        held.bottom = view.getPaddingBottom();
        sheets.add(held);
        if (view instanceof ViewGroup) {
            ((ViewGroup) view).setClipToPadding(false);
        }
        rest(held);
        return view;
    }

    private void seat(Insets around) {
        bars = around;
        if (pages != null) {
            pages.setPadding(bars.left, 0, bars.right, 0);
        }
        for (Sheet held : sheets) {
            rest(held);
        }
    }

    private void rest(Sheet held) {
        held.view.setPadding(
                held.view.getPaddingLeft(), held.top + skin.dp(32) + bars.top,
                held.view.getPaddingRight(), held.bottom + skin.dp(12) + bars.bottom);
    }

    private void askNotifications() {
        if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, NOTIFY);
        }
    }

    private void surface() {
        new Thread(new Runnable() {
            @Override
            public void run() {
                Boolean answer = null;
                try {
                    answer = Core.client(MainActivity.this).imported();
                } catch (Exception e) {
                    Log.e(TunnelService.TAG, "state", e);
                }
                if (answer == null) {
                    return;
                }

                final boolean carried = answer;
                runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        lay(carried);
                    }
                });
            }
        }).start();
    }

    private void lay(boolean imported) {
        if (linked == (imported ? 1 : 0)) {
            if (imported) {
                draw();
            }
            return;
        }
        linked = imported ? 1 : 0;

        shell.removeAllViews();
        if (imported) {
            shell.addView(pages);
            pages.requestApplyInsets();
            pages.show(CONNECT);
            draw();
            maybeAutoConnect();
            return;
        }

        Intent stop = new Intent(this, TunnelService.class);
        stop.setAction(TunnelService.ACTION_STOP);
        startService(stop);

        View screen = sheet(importPage.build());
        shell.addView(screen);
        screen.requestApplyInsets();
    }

    private void maybeAutoConnect() {
        new Thread(new Runnable() {
            @Override
            public void run() {
                boolean wanted = false;
                try {
                    JSONObject settings = new JSONObject(Core.client(MainActivity.this).settingsJSON());
                    wanted = "connect".equals(settings.optString("manualBehaviour", "open")) && !Core.up();
                } catch (Exception ignored) {
                }

                if (!wanted) {
                    return;
                }
                runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        start();
                    }
                });
            }
        }).start();
    }

    private void start() {
        Intent consent = VpnService.prepare(this);
        if (consent != null) {
            startActivityForResult(consent, CONSENT);
            return;
        }

        Intent up = new Intent(this, TunnelService.class);
        up.setAction(TunnelService.ACTION_START);
        startService(up);
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == CONSENT && result == RESULT_OK) {
            start();
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        connectPage.reset();
        connectPage.awake();
        surface();
        ticker.post(tick);
    }

    @Override
    protected void onPause() {
        ticker.removeCallbacks(tick);
        connectPage.sleep();
        super.onPause();
    }

    private void forget() {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Core.client(MainActivity.this).markNoticeRead(0);
                } catch (Exception ignored) {
                }
            }
        }).start();
    }

    private void draw() {
        if (shell.getChildCount() == 0 || shell.getChildAt(0) != pages) {
            return;
        }
        int page = pages.page();
        if (page != shown) {
            shown = page;
            entered(page);
        }

        switch (page) {
            case ROUTING:
                routingPage.render();
                return;
            case SETTINGS:
                settingsPage.render();
                return;
            default:
                connectPage.render();
        }
    }

    private void entered(int page) {
        switch (page) {
            case SETTINGS:
                Snapshot.watch(Snapshot.SETTINGS);
                new Thread(new Runnable() {
                    @Override
                    public void run() {
                        Snapshot.refreshSettings(MainActivity.this);
                        runOnUiThread(new Runnable() {
                            @Override
                            public void run() {
                                settingsPage.render();
                            }
                        });
                    }
                }).start();
                return;
            case ROUTING:
                Snapshot.watch(Snapshot.NOTHING);
                return;
            default:
                Snapshot.watch(Snapshot.CONNECT);
        }
    }

    void requestStart() {
        start();
    }
}
