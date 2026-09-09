package ru.quicdiver.client;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.res.ColorStateList;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.inputmethod.EditorInfo;
import android.widget.EditText;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONObject;

import qdmobile.Client;

public class SettingsPage {

    private final Activity host;
    private final Skin skin;
    private final Runnable onUnlinked;

    private TextView subLine;
    // Уведомления сюда не входят: туннель переживает и сон, и очистку памяти с
    // отозванным разрешением на них -- служба остаётся передней и без показа.
    private final int[] guardWhat = {Guard.BATTERY, Guard.VPN, Guard.AUTOSTART};
    private final TextView[] guardDot = new TextView[3];
    private final TextView[] guardSaid = new TextView[3];
    private final int[] guardWas = {-1, -1, -1};
    private EditText refresh;
    private EditText upload;
    private Toggle connectOnOpen;
    private Toggle adblock;
    private boolean touched;
    private volatile boolean writing;

    private final Runnable settle = new Runnable() {
        @Override
        public void run() {
            if (touched && !writing) {
                save();
            }
        }
    };

    public SettingsPage(Activity host, Skin skin, Runnable onUnlinked) {
        this.host = host;
        this.skin = skin;
        this.onUnlinked = onUnlinked;
    }

    public View build() {
        LinearLayout page = skin.column();
        page.setPadding(skin.dp(24), 0, skin.dp(24), 0);

        LinearLayout sub = skin.card();
        sub.addView(skin.title("Подписка"));
        subLine = skin.note("");
        sub.addView(subLine);
        page.addView(sub, skin.gap(14));

        LinearLayout local = skin.card();
        local.addView(skin.title("На этом устройстве"));

        refresh = number(local, "Обновлять подписку, минут");
        upload = number(local, "Ограничение отдачи, Мбит/с (0 — без)");

        LinearLayout row = new LinearLayout(host);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(0, skin.dp(12), 0, 0);
        row.addView(skin.label("Подключаться при запуске", skin.text, 15),
                new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        connectOnOpen = new Toggle(host, skin);
        connectOnOpen.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                connectOnOpen.setChecked(!connectOnOpen.isChecked());
                patch("manualBehaviour", connectOnOpen.isChecked() ? "connect" : "open");
            }
        });
        row.addView(connectOnOpen);
        local.addView(row);

        LinearLayout adRow = new LinearLayout(host);
        adRow.setOrientation(LinearLayout.HORIZONTAL);
        adRow.setGravity(Gravity.CENTER_VERTICAL);
        adRow.setPadding(0, skin.dp(12), 0, 0);
        adRow.addView(skin.label("Блокировать рекламу", skin.text, 15),
                new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        adblock = new Toggle(host, skin);
        adblock.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                adblock.setChecked(!adblock.isChecked());
                flipAdblock(adblock.isChecked());
            }
        });
        adRow.addView(adblock);
        local.addView(adRow);

        page.addView(local, skin.gap(14));


        LinearLayout guard = skin.card();
        guard.addView(skin.title("Устойчивость фонового сервиса"));

        for (int i = 0; i < guardWhat.length; i++) {
            if (guardWhat[i] == Guard.AUTOSTART && Guard.rom() == Guard.ROM_STOCK) {
                continue;
            }
            guardRow(guard, i);
        }
        page.addView(guard, skin.gap(14));

        LinearLayout diag = skin.card();
        diag.addView(skin.title("Журнал"));
        final View dump = chip("Выгрузить журнал в Загрузки", skin.good, 0xFFFFFFFF,
                R.drawable.ic_journal, corners(true, true));
        dump.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                boast(dump, saveJournal());
            }
        });
        LinearLayout.LayoutParams wide = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        wide.topMargin = skin.dp(12);
        diag.addView(dump, wide);
        page.addView(diag, skin.gap(14));

        LinearLayout danger = skin.card();
        danger.addView(skin.title("Сброс"));

        LinearLayout pair = new LinearLayout(host);
        pair.setOrientation(LinearLayout.HORIZONTAL);

        View wipe = chip("Сброс настроек", skin.idle, skin.text,
                R.drawable.ic_reset, corners(true, false));
        wipe.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                confirm("Сбросить настройки?", "Подписка останется.", false);
            }
        });
        pair.addView(wipe, new LinearLayout.LayoutParams(
                0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        View drop = chip("Отвязать подписку", skin.bad, 0xFFFFFFFF,
                R.drawable.ic_unlink, corners(false, true));
        drop.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                confirm("Отвязать подписку?",
                        "Туннель остановится, ключ и узлы будут забыты.", true);
            }
        });
        LinearLayout.LayoutParams right = new LinearLayout.LayoutParams(
                0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        right.leftMargin = skin.dp(3);
        pair.addView(drop, right);

        LinearLayout.LayoutParams seat = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        seat.topMargin = skin.dp(12);
        danger.addView(pair, seat);
        page.addView(danger, skin.gap(14));

        Scroller scroll = new Scroller(host, skin);
        scroll.setClipChildren(false);
        scroll.setClipToPadding(false);
        page.setClipChildren(false);
        page.setClipToPadding(false);
        scroll.addView(page);
        scroll.setFillViewport(true);
        return scroll;
    }

    public void render() {
        guardStatus();
        try {
            JSONObject state = Snapshot.state();
            JSONObject about = Snapshot.about();
            JSONObject settings = Snapshot.settings();

            String tag = about.optString("label", "");
            long expires = about.optLong("expiresAt");
            subLine.setText((tag.isEmpty() ? "без метки" : tag)
                    + (expires > 0 ? " · до " + date(expires) : " · бессрочно")
                    + " · трафик всего "
                    + TunnelService.human(about.optLong("up") + about.optLong("down")));

            if (!writing) {
                if (touched && !refresh.hasFocus() && !upload.hasFocus()) {
                    save();
                } else if (!touched) {
                    put(refresh, settings.optInt("refreshMinutes", 60));
                    put(upload, settings.optInt("fixedRate", 0));
                }
                connectOnOpen.settle("connect".equals(settings.optString("manualBehaviour", "open")));
                adblock.settle(state.optBoolean("adblock"));
            }
        } catch (Exception e) {
            subLine.setText(String.valueOf(e.getMessage()));
        }
    }

    private void save() {
        JSONObject body = new JSONObject();
        try {
            body.put("refreshMinutes", asInt(refresh.getText().toString(), 60));
            body.put("fixedRate", asInt(upload.getText().toString(), 0));
        } catch (Exception e) {
            toast(e.getMessage());
            return;
        }
        push(body.toString(), true);
    }

    private void flipAdblock(final boolean on) {
        writing = true;
        new Thread(new Runnable() {
            @Override
            public void run() {
                String problem = null;
                try {
                    Core.client(host).setAdblock(on);
                } catch (Exception e) {
                    problem = e.getMessage();
                }
                Snapshot.refreshSettings(host);
                Snapshot.refreshState(host);

                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        writing = false;
                        if (said != null) {
                            toast(said);
                        }
                        render();
                    }
                });
            }
        }).start();
    }

    private void patch(String key, String value) {
        JSONObject body = new JSONObject();
        try {
            body.put(key, value);
        } catch (Exception e) {
            return;
        }
        push(body.toString(), false);
    }

    private void patchFlag(String key, boolean value) {
        JSONObject body = new JSONObject();
        try {
            body.put(key, value);
        } catch (Exception e) {
            return;
        }
        push(body.toString(), false);
    }

    private void push(final String body, final boolean clears) {
        writing = true;
        new Thread(new Runnable() {
            @Override
            public void run() {
                String problem = null;
                try {
                    Core.client(host).saveSettingsJSON(body);
                } catch (Exception e) {
                    problem = e.getMessage();
                }
                Snapshot.refreshSettings(host);
                Snapshot.refreshState(host);

                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        writing = false;
                        if (said == null && clears) {
                            touched = false;
                            toast("Параметры сохранены");
                        } else if (said != null) {
                            toast(said);
                        }
                        render();
                    }
                });
            }
        }).start();
    }

    private void confirm(String heading, String body, final boolean subscription) {
        AlertDialog box = new AlertDialog.Builder(host, R.style.RoundDialog)
                .setTitle(heading)
                .setMessage(body)
                .setNegativeButton("Отмена", null)
                .setPositiveButton("Да", (dialog, which) -> reset(subscription))
                .create();
        box.show();
        skin.frame(box);
    }

    private void reset(final boolean subscription) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                String problem = null;
                try {
                    Core.client(host).reset(subscription);
                } catch (Exception e) {
                    problem = e.getMessage();
                }
                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        touched = false;
                        toast(said == null ? "Готово" : said);
                        if (said == null && subscription) {
                            onUnlinked.run();
                            return;
                        }
                        render();
                    }
                });
            }
        }).start();
    }

    private EditText number(LinearLayout parent, String caption) {
        TextView name = skin.note(caption);
        name.setPadding(0, skin.dp(10), 0, skin.dp(2));
        parent.addView(name);

        final EditText field = new EditText(host);
        field.setInputType(InputType.TYPE_CLASS_NUMBER);
        field.setTextColor(skin.bold);
        field.setTextSize(TypedValue.COMPLEX_UNIT_SP, 14);
        field.setSingleLine(true);
        field.setPadding(skin.dp(14), skin.dp(11), skin.dp(14), skin.dp(11));
        field.setBackground(skin.field());
        field.addTextChangedListener(new TextWatcher() {
            @Override
            public void beforeTextChanged(CharSequence s, int a, int b, int c) {
            }

            @Override
            public void onTextChanged(CharSequence s, int a, int b, int c) {
                if (field.hasFocus()) {
                    touched = true;
                }
            }

            @Override
            public void afterTextChanged(Editable s) {
                if (!field.hasFocus()) {
                    return;
                }
                field.removeCallbacks(settle);
                field.postDelayed(settle, 800);
            }
        });
        field.setImeOptions(EditorInfo.IME_ACTION_DONE);
        field.setOnFocusChangeListener(new View.OnFocusChangeListener() {
            @Override
            public void onFocusChange(View v, boolean has) {
                if (!has && touched) {
                    save();
                }
            }
        });
        field.setOnEditorActionListener(new TextView.OnEditorActionListener() {
            @Override
            public boolean onEditorAction(TextView v, int id, KeyEvent event) {
                if (id == EditorInfo.IME_ACTION_DONE) {
                    field.clearFocus();
                    return true;
                }
                return false;
            }
        });
        parent.addView(field);
        return field;
    }

    private void recheck() {
        host.getWindow().getDecorView().postDelayed(new Runnable() {
            @Override
            public void run() {
                guardStatus();
            }
        }, 700);
    }

    private void guardRow(LinearLayout parent, final int which) {
        LinearLayout row = new LinearLayout(host);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(0, skin.dp(12), 0, skin.dp(12));
        row.setClickable(true);
        row.setBackground(skin.touchable(new GradientDrawable()));
        row.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                Guard.open(host, guardWhat[which]);
                recheck();
            }
        });

        TextView dot = new TextView(host);
        dot.setTextColor(0xFFFFFFFF);
        dot.setTextSize(TypedValue.COMPLEX_UNIT_SP, 12);
        dot.setGravity(Gravity.CENTER);
        dot.setIncludeFontPadding(false);
        LinearLayout.LayoutParams badge =
                new LinearLayout.LayoutParams(skin.dp(20), skin.dp(20));
        badge.rightMargin = skin.dp(10);
        row.addView(dot, badge);

        TextView said = skin.label("", skin.text, 15);
        LinearLayout.LayoutParams rest = new LinearLayout.LayoutParams(0,
                LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        row.addView(said, rest);

        guardDot[which] = dot;
        guardSaid[which] = said;
        parent.addView(row);
    }

    private void guardStatus() {
        for (int i = 0; i < guardWhat.length; i++) {
            if (guardDot[i] == null) {
                continue;
            }
            int state = Guard.state(host, guardWhat[i]);
            if (state == guardWas[i]) {
                continue;
            }
            guardWas[i] = state;


            GradientDrawable face = new GradientDrawable();
            face.setShape(GradientDrawable.OVAL);
            face.setColor(state == Guard.YES ? skin.good : state == Guard.NO ? skin.bad : skin.idle);
            guardDot[i].setBackground(face);
            guardDot[i].setText(state == Guard.YES ? "\u2713" : state == Guard.NO ? "\u2715" : "?");

            guardSaid[i].setText(Guard.said(guardWhat[i], state));
        }
    }

    // boast говорит о сделанном на самой кнопке: отдельная строка под ней жила
    // ниже всякого ритма карточки и оставалась там навсегда.
    private void boast(final View button, String news) {
        final TextView caps = (TextView) ((LinearLayout) button).getChildAt(1);
        if (button.getTag() == null) {
            button.setTag(caps.getText());
        }
        final CharSequence was = (CharSequence) button.getTag();
        caps.setText(news);
        caps.postDelayed(new Runnable() {
            @Override
            public void run() {
                caps.setText(was);
                button.setTag(null);
            }
        }, 10000L);
    }

    private float[] corners(boolean first, boolean last) {
        float wide = skin.dpf(14f);
        float tight = skin.dpf(5f);
        return new float[]{
                first ? wide : tight, first ? wide : tight,
                last ? wide : tight, last ? wide : tight,
                last ? wide : tight, last ? wide : tight,
                first ? wide : tight, first ? wide : tight,
        };
    }

    private View chip(String caption, int face, int letter, int glyph, float[] radii) {
        LinearLayout row = new LinearLayout(host);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER);
        row.setPadding(skin.dp(9), skin.dp(10), skin.dp(9), skin.dp(10));

        ImageView mark = new ImageView(host);
        mark.setImageResource(glyph);
        mark.setImageTintList(ColorStateList.valueOf(letter));
        row.addView(mark, new LinearLayout.LayoutParams(skin.dp(15), skin.dp(15)));

        TextView caps = skin.label(caption, letter, 13);
        caps.setGravity(Gravity.CENTER);
        skin.shrink(caps, 10, 13);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        lp.leftMargin = skin.dp(6);
        row.addView(caps, lp);

        // Пустая распорка справа шириной со значок: без неё текст считает своей
        // серединой всё, что осталось от значка, и уезжает вправо.
        row.addView(new View(host), new LinearLayout.LayoutParams(
                skin.dp(15) + skin.dp(6), skin.dp(1)));

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(face);
        bg.setCornerRadii(radii);
        row.setBackground(skin.touchable(bg));
        return row;
    }

    private void toast(String message) {
    }

    private static void put(EditText field, int value) {
        String want = String.valueOf(value);
        if (want.equals(field.getText().toString())) {
            return;
        }
        field.setText(want);
        field.setSelection(want.length());
    }

    private static int asInt(String raw, int fallback) {
        try {
            return Integer.parseInt(raw.trim());
        } catch (Exception e) {
            return fallback;
        }
    }

    private static String date(long ms) {
        return android.text.format.DateFormat.format("dd.MM.yyyy", ms).toString();
    }

    private String saveJournal() {
        java.io.File from = null;
        try {
            Client client = Core.client(host);
            String path = client.logPath();
            if (path == null || path.isEmpty()) {
                return "Журнал не ведётся";
            }
            from = new java.io.File(path);
            if (!from.exists() || from.length() == 0) {
                return "Журнал пуст";
            }
        } catch (Exception e) {
            return "Журнал недоступен: " + e.getMessage();
        }

        String name = "qd-" + android.text.format.DateFormat
                .format("MMdd-HHmmss", System.currentTimeMillis()) + ".log";
        try {
            android.content.ContentValues row = new android.content.ContentValues();
            row.put(android.provider.MediaStore.Downloads.DISPLAY_NAME, name);
            row.put(android.provider.MediaStore.Downloads.MIME_TYPE, "text/plain");
            row.put(android.provider.MediaStore.Downloads.RELATIVE_PATH,
                    android.os.Environment.DIRECTORY_DOWNLOADS);

            android.net.Uri put = host.getContentResolver().insert(
                    android.provider.MediaStore.Downloads.EXTERNAL_CONTENT_URI, row);
            if (put == null) {
                return "Не удалось создать файл";
            }

            java.io.OutputStream out = host.getContentResolver().openOutputStream(put);
            byte[] buf = new byte[8192];
            long written = 0;
            for (java.io.File part : new java.io.File[]{
                    new java.io.File(from.getPath() + ".1"), from}) {
                if (!part.exists()) {
                    continue;
                }
                java.io.InputStream in = new java.io.FileInputStream(part);
                int n;
                while ((n = in.read(buf)) > 0) {
                    out.write(buf, 0, n);
                    written += n;
                }
                in.close();
            }
            out.close();
            return "Загрузки/" + name + " (" + (written / 1024) + " КБ)";
        } catch (Exception e) {
            return "Не сохранилось: " + e.getMessage();
        }
    }
}
