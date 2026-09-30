package ru.qd.client;

import android.animation.TimeInterpolator;
import android.app.Activity;
import android.content.Intent;
import android.graphics.RenderEffect;
import android.graphics.Shader;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.text.SpannableStringBuilder;
import android.text.Spanned;
import android.text.TextPaint;
import android.text.method.LinkMovementMethod;
import android.text.style.ClickableSpan;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.animation.LinearInterpolator;
import android.view.animation.PathInterpolator;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.json.JSONObject;

public class Upkeep {

    static volatile float pace = 1f;

    private static final int RED = 0xFFE03A2F;
    private static final int LIVE = 0xFF71D888;

    private static final TimeInterpolator EASE = new PathInterpolator(0.25f, 0.1f, 0.25f, 1f);
    private static final TimeInterpolator SLIDE = new PathInterpolator(0.4f, 0.12f, 0.28f, 1f);
    private static final TimeInterpolator OUT = new PathInterpolator(0.2f, 0.7f, 0.2f, 1f);
    private static final TimeInterpolator BOUNCE = new PathInterpolator(0.34f, 1.56f, 0.64f, 1f);
    private static final TimeInterpolator LINE = new LinearInterpolator();

    private static final int[] MINUTES = {30, 720, 1440, -1};
    private static final String[] MARKS = {"30м", "12ч", "1д", "∞"};
    private static final String[] WHEN = {"30 минут", "12 часов", "1 день", ""};

    private static final int NORMAL = 0;
    private static final int SERVICE = 1;
    private static final int MANUAL = 2;
    private static final int CHEER = 3;

    private static final int AVAILABLE = 0;
    private static final int INSTALLING = 1;
    private static final int FAILED = 2;
    private static final int DELAYED = 3;

    private final Activity host;
    private final Skin skin;
    private final Handler main = new Handler(Looper.getMainLooper());

    private volatile JSONObject info = new JSONObject();
    private volatile boolean asking;

    private final Tween flip = new Tween(0f);
    private final Tween gone = new Tween(0f);
    private final Tween down = new Tween(0f);
    private final Tween out = new Tween(0f);
    private final Tween shade = new Tween(0f);
    private final Tween period = new Tween(0f);
    private final Tween fill = new Tween(0f);
    private final Tween strip = new Tween(0f);
    private final Tween hide = new Tween(0f);
    private final Tween[] fade = {new Tween(0f), new Tween(0f), new Tween(0f), new Tween(0f)};
    private final Tween[] rise = {new Tween(0f), new Tween(0f), new Tween(0f), new Tween(0f)};
    private final Tween[] slide = {new Tween(0f), new Tween(1f), new Tween(1f), new Tween(1f)};

    private final View[] heads = new View[4];
    private final TextView[] says = new TextView[4];
    private final LinearLayout[] notes = new LinearLayout[4];
    private final TextView[] noteTitles = new TextView[4];
    private final TextView[] noteTexts = new TextView[4];
    private final float[] blurWas = {-1f, -1f, -1f, -1f};
    private float liveBlurWas = -1f;

    private final Tween tint = new Tween(0f);
    private final Tween gleam = new Tween(0f);
    private final Tween fadeOut = new Tween(0f);

    private TextView fault;
    private View headCard;
    private View live;
    private Halo later;
    private View periodBox;
    private LinearLayout periodStrip;
    private TextView fromLabel;
    private TextView toLabel;
    private View egress;
    private Halo update;

    private boolean starting;
    private boolean rang;
    private boolean delayed;
    private int delayFor;
    private long delayUntil;
    private boolean cheer;
    private boolean cheerDelayed;
    private long cheerUntil;
    private int headState = -1;
    private int spot;
    private String release = "https://github.com/jaywehosl/qd/releases";

    public Upkeep(Activity host, Skin skin) {
        this.host = host;
        this.skin = skin;
    }

    public void head(FrameLayout stack, View row, View card) {
        headCard = card;
        heads[NORMAL] = row;
        String[] words = {"", "Требуется обслуживание", "Требуется ручное обслуживание", ""};
        for (int i = SERVICE; i <= CHEER; i++) {
            TextView say = skin.label(words[i], skin.bold, 24);
            say.setTypeface(Typeface.DEFAULT_BOLD);
            say.setGravity(Gravity.CENTER);
            skin.shrink(say, 13, 22);
            say.setTranslationY(10000f);
            stack.addView(say, new FrameLayout.LayoutParams(
                    FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
            says[i] = say;
            heads[i] = say;
        }
    }

    public void stage(FrameLayout card, View liveLayer) {
        live = liveLayer;
        for (int i = 0; i < notes.length; i++) {
            LinearLayout note = new LinearLayout(host);
            note.setOrientation(LinearLayout.VERTICAL);
            note.setGravity(Gravity.CENTER_VERTICAL);
            note.setPadding(skin.dp(26), skin.dp(24), skin.dp(26), skin.dp(24));
            note.setAlpha(0f);

            TextView title = skin.label("", skin.bold, 24);
            title.setTypeface(Typeface.DEFAULT_BOLD);
            note.addView(title);

            TextView text = skin.label("", skin.text, 15);
            text.setLineSpacing(0f, 1.3f);
            text.setPadding(0, skin.dp(10), 0, 0);
            note.addView(text);

            noteTitles[i] = title;
            noteTexts[i] = text;
            notes[i] = note;
            card.addView(note, new FrameLayout.LayoutParams(
                    FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        }
        fault = skin.label("", RED, 13);
        fault.setPadding(0, skin.dp(10), 0, 0);
        fault.setVisibility(View.GONE);
        notes[AVAILABLE].addView(fault);

        noteTitles[INSTALLING].setText("Устанавливается обновление");
        noteTexts[INSTALLING].setText("qd перезапустится сам и попробует восстановить активное подключение.");
        noteTitles[FAILED].setText("Обновление не удалось");
        noteTitles[DELAYED].setText("Обновление отложено");

        SpannableStringBuilder link = new SpannableStringBuilder(
                "Скачанный пакет повреждён. Скачайте и обновите вручную ");
        int from = link.length();
        link.append("здесь");
        link.setSpan(new ClickableSpan() {
            @Override
            public void onClick(View widget) {
                openRelease();
            }

            @Override
            public void updateDrawState(TextPaint paint) {
                paint.setColor(LIVE);
                paint.setUnderlineText(true);
                paint.setFakeBoldText(true);
            }
        }, from, link.length(), Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
        link.append(".");
        noteTexts[FAILED].setText(link);
        noteTexts[FAILED].setMovementMethod(LinkMovementMethod.getInstance());
        noteTexts[FAILED].setHighlightColor(0);
    }

    public Halo later() {
        later = new Halo(host, skin);
        later.setStrength(0f);
        later.setVisibility(View.INVISIBLE);

        LinearLayout pill = new LinearLayout(host);
        pill.setOrientation(LinearLayout.HORIZONTAL);
        pill.setGravity(Gravity.CENTER_VERTICAL);
        pill.setPadding(skin.dp(24), skin.dp(14), skin.dp(14), skin.dp(14));
        GradientDrawable face = new GradientDrawable();
        face.setCornerRadius(skin.dp(27));
        face.setColor(RED);
        pill.setBackground(skin.touchable(face));
        pill.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                putOff();
            }
        });

        TextView word = skin.label("отложить", 0xFFFFFFFF, 27);
        word.setGravity(Gravity.CENTER);
        word.setTypeface(Typeface.DEFAULT_BOLD);
        skin.shrink(word, 17, 27);
        pill.addView(word, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        FrameLayout box = new FrameLayout(host);
        GradientDrawable boxFace = new GradientDrawable();
        boxFace.setCornerRadius(skin.dp(18));
        boxFace.setColor(skin.idle);
        box.setBackground(skin.touchable(boxFace));
        final float round = skin.dpf(18f);
        box.setOutlineProvider(new android.view.ViewOutlineProvider() {
            @Override
            public void getOutline(View view, android.graphics.Outline shape) {
                shape.setRoundRect(0, 0, view.getWidth(), view.getHeight(), round);
            }
        });
        box.setClipToOutline(true);
        box.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                cycle();
            }
        });

        periodStrip = new LinearLayout(host);
        periodStrip.setOrientation(LinearLayout.HORIZONTAL);
        for (int i = 0; i <= MARKS.length; i++) {
            TextView mark = skin.label(MARKS[i % MARKS.length], RED, 17);
            mark.setTypeface(Typeface.DEFAULT_BOLD);
            mark.setFontFeatureSettings("tnum");
            mark.setGravity(Gravity.CENTER);
            periodStrip.addView(mark, new LinearLayout.LayoutParams(
                    skin.dp(60), LinearLayout.LayoutParams.MATCH_PARENT));
        }
        box.addView(periodStrip, new FrameLayout.LayoutParams(
                skin.dp(60) * (MARKS.length + 1), FrameLayout.LayoutParams.MATCH_PARENT));
        periodBox = box;
        pill.addView(box, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.MATCH_PARENT));

        later.addView(pill, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        return later;
    }

    public void power(TextView from, TextView to, View exit, Halo ring) {
        fromLabel = from;
        toLabel = to;
        egress = exit;
        update = ring;
        ring.addOnLayoutChangeListener(new View.OnLayoutChangeListener() {
            @Override
            public void onLayoutChange(View v, int l, int t, int r, int b,
                                       int ol, int ot, int or2, int ob) {
                final int tall = b - t;
                final ViewGroup.LayoutParams lp = later.getLayoutParams();
                if (lp == null || lp.height == tall || tall <= 0) {
                    return;
                }
                later.post(new Runnable() {
                    @Override
                    public void run() {
                        lp.height = tall;
                        later.setLayoutParams(lp);
                    }
                });
            }
        });
    }

    public void poll() {
        if (asking) {
            return;
        }
        asking = true;
        new Thread(new Runnable() {
            @Override
            public void run() {
                JSONObject got = null;
                try {
                    got = new JSONObject(Core.client(host).updateJSON());
                } catch (Exception ignored) {
                }
                asking = false;
                if (got == null) {
                    return;
                }
                info = got;
                final boolean busy = !got.optString("status").isEmpty();
                if (busy) {
                    main.postDelayed(new Runnable() {
                        @Override
                        public void run() {
                            poll();
                        }
                    }, 400L);
                }
            }
        }).start();
    }

    public boolean owns() {
        return noticed();
    }

    private JSONObject offer() {
        return info.optJSONObject("offer");
    }

    private boolean noticed() {
        JSONObject offer = offer();
        if (offer == null) {
            return false;
        }
        return "required".equals(offer.optString("state")) || !info.has("postponedUntil");
    }

    private boolean required() {
        JSONObject offer = offer();
        return offer != null && "required".equals(offer.optString("state"));
    }

    private boolean failed() {
        return noticed() && "corrupt".equals(info.optString("failed"));
    }

    private boolean installing() {
        return noticed() && "installing".equals(info.optString("status"));
    }

    public void onPower(Runnable tunnel) {
        if (!noticed()) {
            tunnel.run();
            return;
        }
        if (failed()) {
            openRelease();
            return;
        }
        if (starting || !info.optString("status").isEmpty()) {
            return;
        }
        starting = true;
        new Thread(new Runnable() {
            @Override
            public void run() {
                String said = "";
                try {
                    said = Core.client(host).checkUpdate();
                } catch (Exception e) {
                    said = String.valueOf(e.getMessage());
                }
                final String news = said;
                main.post(new Runnable() {
                    @Override
                    public void run() {
                        starting = false;
                        if (!news.isEmpty() && !"latest".equals(news)) {
                            Core.say(host, "update: " + news);
                        }
                        poll();
                    }
                });
            }
        }).start();
    }

    private void putOff() {
        if (!noticed() || required() || starting || !info.optString("status").isEmpty()) {
            return;
        }
        final int chosen = spot % MINUTES.length;
        delayFor = chosen;
        delayed = true;
        delayUntil = SystemClock.elapsedRealtime() + (long) (4000f * pace);
        new Thread(new Runnable() {
            @Override
            public void run() {
                String said;
                try {
                    said = Core.client(host).postponeUpdate(MINUTES[chosen]);
                } catch (Exception e) {
                    said = String.valueOf(e.getMessage());
                }
                final String news = said;
                main.post(new Runnable() {
                    @Override
                    public void run() {
                        if (!news.isEmpty()) {
                            delayed = false;
                            Core.say(host, "update: " + news);
                        }
                        poll();
                    }
                });
            }
        }).start();
    }

    private void cycle() {
        if (spot >= MINUTES.length) {
            return;
        }
        spot++;
        strip.aim(-spot, 0f, 0.45f, BOUNCE, SystemClock.elapsedRealtime());
    }

    private void openRelease() {
        try {
            Intent view = new Intent(Intent.ACTION_VIEW, Uri.parse(release));
            view.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            host.startActivity(view);
        } catch (Exception e) {
            Core.say(host, "update: no browser for " + release);
        }
    }

    public void frame(float tunnelAt) {
        long now = SystemClock.elapsedRealtime();
        JSONObject offer = offer();
        release = info.optString("release", release);

        if (info.has("updatedFrom")) {
            info.remove("updatedFrom");
            cheer = true;
            cheerDelayed = false;
            cheerUntil = now + 10000L;
        }
        if (delayed && now >= delayUntil) {
            delayed = false;
            cheer = true;
            cheerDelayed = true;
            cheerUntil = now + 10000L;
        }
        if (cheer && now >= cheerUntil) {
            cheer = false;
        }

        boolean noticed = noticed();
        boolean required = required();
        boolean failed = failed();
        boolean installing = installing();
        boolean leaving = failed || installing;
        boolean open = noticed && !required && !leaving;
        String status = info.optString("status");

        if (noticed) {
            flip.aim(1f, 0f, 0.26f, LINE, now);
            gone.aim(1f, 0.26f, 0.42f, SLIDE, now);
            fadeOut.aim(1f, 0.26f, 0.3f, EASE, now);
        } else {
            flip.aim(0f, 1.74f, 0.26f, LINE, now);
            gone.aim(0f, 1.32f, 0.42f, SLIDE, now);
            fadeOut.aim(0f, 1.32f, 0.3f, EASE, now);
        }
        if (open) {
            down.aim(1f, 0.68f, 0.45f, SLIDE, now);
            out.aim(1f, 1.13f, 0.45f, OUT, now);
            shade.aim(1f, 1.13f, 0.45f, EASE, now);
            period.aim(1f, 1.58f, 0.42f, SLIDE, now);
            gleam.aim(1f, 1.58f, 0.3f, EASE, now);
        } else {
            down.aim(0f, 0.87f, 0.45f, SLIDE, now);
            out.aim(0f, 0.42f, 0.45f, SLIDE, now);
            shade.aim(0f, 0.42f, 0.3f, EASE, now);
            period.aim(0f, 0f, 0.42f, SLIDE, now);
            gleam.aim(0f, 0f, 0.3f, EASE, now);
        }

        boolean stageNotice = noticed || delayed;
        if (stageNotice) {
            hide.aim(1f, 0f, 0.42f, EASE, now);
        } else {
            hide.aim(0f, 0.18f, 0.45f, EASE, now);
        }
        boolean[] shown = {noticed && !leaving, installing, failed, delayed && !noticed};
        for (int i = 0; i < notes.length; i++) {
            if (shown[i]) {
                fade[i].aim(1f, 0.18f, 0.45f, EASE, now);
                rise[i].aim(1f, 0.18f, 0.55f, OUT, now);
            } else {
                fade[i].aim(0f, 0f, 0.42f, EASE, now);
                rise[i].aim(0f, 0f, 0.42f, SLIDE, now);
            }
        }

        float progress = 0f;
        if ("installing".equals(status)) {
            progress = 1f;
        } else if (info.optLong("total") > 0) {
            progress = Math.min(1f, info.optLong("done") / (float) info.optLong("total"));
        }
        boolean ringing = noticed && !leaving && (starting || !status.isEmpty());
        if (ringing && !rang) {
            fill.set(progress);
        }
        rang = ringing;
        fill.aimReal(progress, 0.4f, LINE, now);

        int wantHead = cheer ? CHEER : failed ? MANUAL : stageNotice ? SERVICE : NORMAL;
        if (headState < 0) {
            headState = wantHead;
            for (int i = 0; i < slide.length; i++) {
                slide[i].set(i == wantHead ? 0f : 1f);
            }
        } else if (wantHead != headState) {
            slide[headState].play(0f, -1f, 0.26f, LINE, now);
            slide[wantHead].play(1f, 0f, 0.26f, LINE, now);
            for (int i = 0; i < slide.length; i++) {
                if (i != headState && i != wantHead) {
                    slide[i].set(1f);
                }
            }
            headState = wantHead;
        }

        texts(offer, required, status, tunnelAt);
        apply(now, ringing);

        if (strip.done(now) && spot >= MINUTES.length) {
            spot = 0;
            strip.set(0f);
        }
    }

    private void texts(JSONObject offer, boolean required, String status, float tunnelAt) {
        if (offer != null) {
            text(noteTitles[AVAILABLE], required ? "Требуется обновление" : "Доступно обновление");
            text(noteTexts[AVAILABLE], required
                    ? "Эта версия больше не может подключаться к сети. Обновите, чтобы продолжить пользоваться qd."
                    : "Вышла новая версия. Обновите сейчас или отложите на время.");
        }
        String error = info.optString("error");
        text(fault, error);
        fault.setVisibility(error.isEmpty() ? View.GONE : View.VISIBLE);
        text(noteTexts[DELAYED], WHEN[delayFor].isEmpty()
                ? "qd больше не предложит это обновление — обновитесь из настроек, когда захотите. Подключение не прервано, можно продолжать работу."
                : "qd снова предложит обновиться через " + WHEN[delayFor] + ". Подключение не прервано, можно продолжать работу.");

        boolean up = Core.up();
        String cheerWord = cheerDelayed
                ? (up ? "Обновление отложено, подключение активно" : "Обновление отложено")
                : (up ? "Успешно обновлено и подключено" : "Успешно обновлено");
        text(says[CHEER], cheerWord);

        String word;
        if (failed()) {
            word = "скачать вручную";
        } else if ("installing".equals(status)) {
            word = "перезапуск";
        } else if ("fetching".equals(status)) {
            long total = info.optLong("total");
            word = "скачивание " + (total > 0 ? Math.round(100f * Math.min(1f, info.optLong("done") / (float) total)) : 0) + "%";
        } else if (starting) {
            word = "проверка";
        } else {
            word = "обновить";
        }
        text(toLabel, word);
        toLabel.setTextColor(fromLabel.getCurrentTextColor());
    }

    private void apply(long now, boolean ringing) {
        float f = flip.tick(now);
        float g = gone.tick(now);
        float d = down.tick(now);
        float o = out.tick(now);
        float s = shade.tick(now);
        float p = period.tick(now);
        float r = fill.tick(now);
        float k = strip.tick(now);
        float h = hide.tick(now);

        float travel = 2.4f * fromLabel.getTextSize();
        fromLabel.setTranslationY(-f * travel);
        toLabel.setTranslationY((1f - f) * travel);

        if (egress.getVisibility() != View.GONE) {
            ViewGroup.LayoutParams lp = egress.getLayoutParams();
            int want = Math.round(skin.dp(60) * (1f - g));
            if (lp.width != want) {
                lp.width = want;
                egress.setLayoutParams(lp);
            }
            egress.setAlpha(1f - fadeOut.tick(now));
        }

        update.setTranslationY(-update.getHeight() * (1f - d));
        later.setTranslationY(-(later.getHeight() + skin.dp(56)) * (1f - o));
        later.setVisibility(o > 0.0005f ? View.VISIBLE : View.INVISIBLE);
        later.setElevation(skin.dpf(12f) * s);

        ViewGroup.LayoutParams bp = periodBox.getLayoutParams();
        int wantBox = Math.round(skin.dp(60) * p);
        if (bp.width != wantBox) {
            bp.width = wantBox;
            periodBox.setLayoutParams(bp);
        }
        periodBox.setAlpha(gleam.tick(now));
        periodStrip.setTranslationX(k * skin.dp(60));

        live.setAlpha(1f - h);
        liveBlurWas = blur(live, h, liveBlurWas);
        for (int i = 0; i < notes.length; i++) {
            float a = fade[i].tick(now);
            float lift = rise[i].tick(now);
            notes[i].setAlpha(a);
            notes[i].setTranslationY((1f - lift) * skin.dp(14));
            notes[i].setVisibility(a > 0.001f ? View.VISIBLE : View.INVISIBLE);
            blurWas[i] = blur(notes[i], 1f - a, blurWas[i]);
        }

        int cardHeight = headCard.getHeight();
        for (int i = 0; i < heads.length; i++) {
            heads[i].setTranslationY(slide[i].tick(now) * cardHeight);
        }

        if (ringing) {
            update.setTone(LIVE);
            update.setSweep(r);
            update.setStrength(1f);
        }
    }

    private float blur(View view, float amount, float was) {
        float radius = Math.round(amount * skin.dpf(8f) * 2f) / 2f;
        if (radius == was) {
            return was;
        }
        view.setRenderEffect(radius <= 0f ? null
                : RenderEffect.createBlurEffect(radius, radius, Shader.TileMode.DECAL));
        return radius;
    }

    public float tint(boolean on) {
        long now = SystemClock.elapsedRealtime();
        tint.aim(on ? 1f : 0f, 0f, 0.3f, EASE, now);
        return tint.tick(now);
    }

    public boolean paintsPower() {
        return noticed() && !info.optString("status").isEmpty();
    }

    private static void text(TextView view, CharSequence want) {
        if (!want.toString().contentEquals(view.getText())) {
            view.setText(want);
        }
    }

    private static final class Tween {
        private float value;
        private float from;
        private float to;
        private long start;
        private long length = 1L;
        private TimeInterpolator ease = LINE;

        Tween(float value) {
            this.value = value;
            this.from = value;
            this.to = value;
        }

        void set(float v) {
            value = v;
            from = v;
            to = v;
            start = 0L;
        }

        void aim(float target, float delay, float seconds, TimeInterpolator how, long now) {
            if (target == to) {
                return;
            }
            from = value;
            to = target;
            start = now + (long) (delay * pace * 1000f);
            length = Math.max(1L, (long) (seconds * pace * 1000f));
            ease = how;
        }

        void aimReal(float target, float seconds, TimeInterpolator how, long now) {
            if (target == to) {
                return;
            }
            from = value;
            to = target;
            start = now;
            length = Math.max(1L, (long) (seconds * 1000f));
            ease = how;
        }

        void play(float a, float b, float seconds, TimeInterpolator how, long now) {
            value = a;
            from = a;
            to = b;
            start = now;
            length = Math.max(1L, (long) (seconds * pace * 1000f));
            ease = how;
        }

        boolean done(long now) {
            return now >= start + length;
        }

        float tick(long now) {
            if (now <= start) {
                value = from;
                return value;
            }
            float t = Math.min(1f, (now - start) / (float) length);
            value = from + (to - from) * ease.getInterpolation(t);
            return value;
        }
    }
}
