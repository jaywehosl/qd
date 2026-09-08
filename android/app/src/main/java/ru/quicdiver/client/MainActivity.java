package ru.quicdiver.client;

import android.Manifest;
import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Insets;
import android.graphics.Outline;
import android.net.Uri;
import android.net.VpnService;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewOutlineProvider;
import android.view.ViewTreeObserver;
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
    private Bar bar;
    private FrameLayout dock;
    private Glass glass;

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

        // Уведомление вешаем сразу: через него подключаются, не открывая
        // приложения, поэтому висеть оно должно и до первого подключения.
        Core.readExit(this);
        Notes.wake(this);

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
                bar.show(pages.page());
                draw();
            }
        });

        bar = new Bar(this, skin, new Bar.Pick() {
            @Override
            public void at(int index) {
                pages.show(index);
                bar.show(index);
            }
        });
        bar.show(CONNECT);

        dock = new FrameLayout(this);
        dock.setClipToOutline(true);
        dock.setElevation(skin.dpf(14f));
        dock.setOutlineProvider(new ViewOutlineProvider() {
            @Override
            public void getOutline(View view, Outline shape) {
                shape.setRoundRect(0, 0, view.getWidth(), view.getHeight(), skin.dpf(24f));
            }
        });
        glass = new Glass(this, skin, pages);
        dock.addView(glass, new FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, 0));
        dock.addView(bar, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT));
        importPage = new ImportPage(this, skin, new Runnable() {
            @Override
            public void run() {
                surface();
            }
        });

        shell = new FrameLayout(this);
        shell.getViewTreeObserver().addOnPreDrawListener(
                new ViewTreeObserver.OnPreDrawListener() {
                    @Override
                    public boolean onPreDraw() {
                        if (glass != null && glass.getHeight() != bar.getHeight()) {
                            glass.getLayoutParams().height = bar.getHeight();
                            glass.requestLayout();
                        }
                        if (glass != null) {
                            glass.snap();
                        }
                        return true;
                    }
                });
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
        // Долгий тап по плитке шлёт это действие. Без него система показывает
        // свои параметры приложения, а человек ждёт настроек клиента.
        if (intent != null && android.service.quicksettings.TileService
                .ACTION_QS_TILE_PREFERENCES.equals(intent.getAction())) {
            pages.show(SETTINGS);
        }

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
        if (dock != null && dock.getParent() != null) {
            dock.setLayoutParams(seatBar());
        }
        for (Sheet held : sheets) {
            rest(held);
        }
    }

    // Снизу оставляем место под плавающую строку перехода: она лежит поверх
    // страниц, и без запаса накрывала бы их последнюю карточку.
    private void rest(Sheet held) {
        held.view.setPadding(
                held.view.getPaddingLeft(), held.top + skin.dp(32) + bars.top,
                held.view.getPaddingRight(), held.bottom + skin.dp(108) + bars.bottom);
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
            shell.addView(dock, seatBar());
            pages.requestApplyInsets();
            pages.show(CONNECT);
            bar.show(CONNECT);
            draw();
            warm();
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


    // seatBar сажает строку перехода над системной полосой жестов, а не под неё.
    private FrameLayout.LayoutParams seatBar() {
        FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT);
        lp.gravity = Gravity.BOTTOM;
        int side = skin.dp(24);
        lp.setMargins(side + bars.left, 0, side + bars.right, bars.bottom + skin.dp(20));
        return lp;
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

    // warm заполняет соседние страницы сразу, а не в момент перехода: иначе они
    // въезжают пустыми и на глазах у пользователя доверстываются под свои данные.
    private void warm() {
        routingPage.render();
        settingsPage.render();
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
