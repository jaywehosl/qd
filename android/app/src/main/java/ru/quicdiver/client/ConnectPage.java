package ru.quicdiver.client;

import android.animation.ArgbEvaluator;
import android.app.Activity;
import android.content.Intent;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.Choreographer;
import android.view.Gravity;
import android.view.View;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

public class ConnectPage {

    private static final int RED = 0xFFE03A2F;
    private static final int LIVE = 0xFF71D888;

    private final Activity host;
    private final Skin skin;
    private final Stage stage = new Stage();
    private final ArgbEvaluator mix = new ArgbEvaluator();

    private TextView title;
    private TextView refreshLine;
    private TextView powerLabel;
    private TextView trouble;
    private Halo halo;
    private Halo spark;
    private long sparkAt;
    private LinearLayout power;
    private View egress;
    private Mark mark;
    private GradientDrawable powerFace;
    private GradientDrawable egressFace;

    private Flow flow;
    private LinearLayout roster;
    private ScrollView rosterBox;
    private LinearLayout meters;
    private LinearLayout wideBox;
    private TextView downRate;
    private TextView upRate;
    private TextView wideDown;
    private TextView wideUp;
    private boolean wide;

    private long prevIn;
    private long prevOut;
    private long prevAt;
    private boolean busy;
    private volatile boolean flipping;
    private boolean wasUp;
    private String rosterKey = "";
    private boolean beating;
    private boolean fresh = true;
    private View headCard;
    private View areaCard;
    private View controlCard;

    public ConnectPage(Activity host, Skin skin) {
        this.host = host;
        this.skin = skin;
    }

    public View build() {
        LinearLayout root = skin.column();
        root.setClipChildren(false);
        root.setClipToPadding(false);
        root.setPadding(skin.dp(24), 0, skin.dp(24), skin.dp(64));

        headCard = head();
        areaCard = area();
        controlCard = controls();

        root.addView(headCard, skin.gap(24));
        root.addView(areaCard, area(1f));
        root.addView(controlCard, skin.gap(0));

        return root;
    }

    private void glint() {
        if (sparkAt <= 0L) {
            return;
        }
        float turn = (android.os.SystemClock.elapsedRealtime() - sparkAt) / 520f;
        if (turn >= 1f) {
            sparkAt = 0L;
            spark.setStrength(0f);
            return;
        }
        spark.setSweep(turn);
        spark.setStrength(turn < 0.8f ? 1f : (1f - turn) / 0.2f);
    }

    private LinearLayout.LayoutParams area(float weight) {
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, 0, weight);
        lp.bottomMargin = skin.dp(24);
        return lp;
    }

    private View head() {
        LinearLayout card = skin.card();
        card.setOrientation(LinearLayout.HORIZONTAL);
        card.setGravity(Gravity.CENTER_VERTICAL);

        title = skin.label("qdiver", skin.bold, 24);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        card.addView(title);

        refreshLine = skin.note("");
        refreshLine.setGravity(Gravity.CENTER);
        refreshLine.setPadding(skin.dp(10), 0, skin.dp(10), 0);
        card.addView(refreshLine, new LinearLayout.LayoutParams(0,
                LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        TextView again = skin.label("⟳", 0xFFFFFFFF, 20);
        again.setGravity(Gravity.CENTER);
        again.setIncludeFontPadding(false);
        again.setPadding(0, 0, 0, skin.dp(4));

        GradientDrawable ring = new GradientDrawable();
        ring.setShape(GradientDrawable.OVAL);
        ring.setColor(skin.good);
        again.setBackground(skin.touchable(ring));

        again.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                refresh();
            }
        });

        spark = new Halo(host, skin);
        spark.setInset(skin.dp(7));
        spark.setGirth(skin.dpf(19f));
        spark.setWeight(skin.dpf(3f), skin.dpf(4f));
        spark.setTone(skin.good);
        spark.addView(again, new FrameLayout.LayoutParams(
                skin.dp(38), skin.dp(38)));

        card.addView(spark, new LinearLayout.LayoutParams(skin.dp(52), skin.dp(52)));
        return card;
    }

    private View area() {
        FrameLayout card = new FrameLayout(host);
        GradientDrawable face = new GradientDrawable();
        face.setColor(skin.card);
        face.setCornerRadius(skin.dp(30));
        face.setStroke(Math.max(1, skin.dp(1) / 2), skin.edge);
        card.setBackground(face);
        card.setClipToOutline(true);
        card.setElevation(skin.dp(4));

        flow = new Flow(host, skin);
        card.addView(flow, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));

        roster = skin.column();
        rosterBox = new ScrollView(host);
        rosterBox.setVerticalScrollBarEnabled(false);
        rosterBox.addView(roster);
        rosterBox.setAlpha(0f);

        FrameLayout.LayoutParams rosterAt = new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT);
        rosterAt.gravity = Gravity.BOTTOM | Gravity.START;
        rosterAt.leftMargin = skin.dp(18);

        card.addView(rosterBox, rosterAt);
        card.addOnLayoutChangeListener(new View.OnLayoutChangeListener() {
            @Override
            public void onLayoutChange(View v, int l, int t, int r, int b,
                                       int ol, int ot, int or2, int ob) {
                place();
            }
        });

        meters = new LinearLayout(host);
        meters.setOrientation(LinearLayout.HORIZONTAL);
        meters.setAlpha(0f);
        downRate = pill("0 Б/с");
        upRate = pill("0 Б/с");
        meters.addView(half(downRate), halfAt());
        meters.addView(half(upRate), halfAt());

        FrameLayout.LayoutParams metersAt = new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT);
        metersAt.gravity = Gravity.BOTTOM;
        metersAt.bottomMargin = skin.dp(16);
        card.addView(meters, metersAt);

        LinearLayout bothPill = new LinearLayout(host);
        bothPill.setOrientation(LinearLayout.HORIZONTAL);
        bothPill.setGravity(Gravity.CENTER_VERTICAL);
        bothPill.setPadding(skin.dp(9), skin.dp(5), skin.dp(9), skin.dp(5));

        GradientDrawable wideFace = new GradientDrawable();
        wideFace.setColor(skin.idle);
        wideFace.setCornerRadius(skin.dp(16));
        bothPill.setBackground(wideFace);

        wideDown = tick(Gravity.END);
        wideUp = tick(Gravity.START);

        TextView downSign = tick(Gravity.CENTER_HORIZONTAL);
        downSign.setText("↓");
        downSign.setPadding(skin.dp(4), 0, skin.dp(8), 0);

        TextView upSign = tick(Gravity.CENTER_HORIZONTAL);
        upSign.setText("↑");
        upSign.setPadding(skin.dp(8), 0, skin.dp(4), 0);

        bothPill.addView(wideDown, halfAt());
        bothPill.addView(downSign, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        bothPill.addView(upSign, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        bothPill.addView(wideUp, halfAt());

        wideBox = new LinearLayout(host);
        wideBox.setOrientation(LinearLayout.HORIZONTAL);
        wideBox.setGravity(Gravity.CENTER);
        wideBox.setAlpha(0f);
        wideBox.addView(bothPill, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));

        FrameLayout.LayoutParams wideAt = new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.WRAP_CONTENT, FrameLayout.LayoutParams.WRAP_CONTENT);
        wideAt.gravity = Gravity.BOTTOM | Gravity.CENTER_HORIZONTAL;
        wideAt.bottomMargin = skin.dp(16);
        card.addView(wideBox, wideAt);

        trouble = skin.label("", RED, 14);
        trouble.setGravity(Gravity.CENTER);
        trouble.setAlpha(0f);
        FrameLayout.LayoutParams troubleAt = new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT);
        troubleAt.gravity = Gravity.CENTER;
        card.addView(trouble, troubleAt);

        return card;
    }

    private void place() {
        if (areaCard == null || rosterBox == null) {
            return;
        }
        int w = areaCard.getWidth();
        int h = areaCard.getHeight();
        if (w <= 0 || h <= 0) {
            return;
        }

        int room = (int) (w * 0.5f) - skin.dp(wide ? 40 : 36);
        if (room <= 0) {
            return;
        }
        int tall = (int) (h * (wide ? 0.24f : 0.26f));
        int above = (int) (h * (wide ? 0.34f : 0.20f));
        int lean = wide ? Gravity.BOTTOM | Gravity.CENTER_HORIZONTAL : Gravity.BOTTOM | Gravity.START;
        int side = wide ? 0 : skin.dp(18);

        FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) rosterBox.getLayoutParams();
        if (lp.width != room || lp.height != tall || lp.bottomMargin != above
                || lp.gravity != lean || lp.leftMargin != side) {
            lp.width = room;
            lp.height = tall;
            lp.bottomMargin = above;
            lp.gravity = lean;
            lp.leftMargin = side;
            rosterBox.requestLayout();
        }

        int span = (int) (w * 0.5f) - skin.dp(62);
        FrameLayout.LayoutParams wp = (FrameLayout.LayoutParams) wideBox.getLayoutParams();
        if (span > 0 && wp.width != span) {
            wp.width = span;
            wideBox.requestLayout();
        }
    }

    private LinearLayout half(TextView inner) {
        LinearLayout box = new LinearLayout(host);
        box.setOrientation(LinearLayout.HORIZONTAL);
        box.setGravity(Gravity.CENTER);
        box.addView(inner, new LinearLayout.LayoutParams(
                skin.dp(96), LinearLayout.LayoutParams.WRAP_CONTENT));
        return box;
    }

    private LinearLayout.LayoutParams halfAt() {
        return new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
    }

    private TextView tick(int lean) {
        TextView view = skin.label("", skin.muted, 11);
        view.setTypeface(Typeface.MONOSPACE);
        view.setGravity(lean | Gravity.CENTER_VERTICAL);
        view.setSingleLine(true);
        return view;
    }

    private TextView pill(String caption) {
        TextView view = skin.label(caption, skin.muted, 11);
        view.setTypeface(Typeface.MONOSPACE);
        view.setGravity(Gravity.CENTER);
        view.setPadding(skin.dp(9), skin.dp(5), skin.dp(9), skin.dp(5));

        GradientDrawable face = new GradientDrawable();
        face.setColor(skin.idle);
        face.setCornerRadius(skin.dp(14));
        view.setBackground(face);
        return view;
    }

    private View controls() {
        LinearLayout card = skin.card();
        card.setPadding(skin.dp(6), skin.dp(6), skin.dp(6), skin.dp(12));

        halo = new Halo(host, skin);

        power = new LinearLayout(host);
        power.setOrientation(LinearLayout.HORIZONTAL);
        power.setGravity(Gravity.CENTER_VERTICAL);
        power.setPadding(skin.dp(24), skin.dp(14), skin.dp(14), skin.dp(14));

        powerFace = new GradientDrawable();
        powerFace.setCornerRadius(skin.dp(27));
        powerFace.setColor(skin.idle);
        power.setBackground(skin.touchable(powerFace));
        power.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                flip();
            }
        });

        powerLabel = skin.label("подключиться", skin.bold, 27);
        powerLabel.setGravity(Gravity.CENTER);
        powerLabel.setTypeface(Typeface.DEFAULT_BOLD);
        power.addView(powerLabel, new LinearLayout.LayoutParams(0,
                LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        mark = new Mark(host, skin);
        egress = mark;
        egressFace = new GradientDrawable();
        egressFace.setCornerRadius(skin.dp(18));
        egressFace.setColor(skin.idle);
        egress.setBackground(skin.touchable(egressFace));
        egress.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                flipExit();
            }
        });
        power.addView(egress, new LinearLayout.LayoutParams(skin.dp(60), skin.dp(60)));

        halo.addView(power, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT));
        card.addView(halo, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));

        TextView feet = skin.label("←   маршрутизация          настройки   →", skin.muted, 16);
        feet.setGravity(Gravity.CENTER);
        feet.setPadding(0, skin.dp(10), 0, skin.dp(2));
        card.addView(feet);

        return card;
    }

    public void reset() {
        prevAt = 0;
    }

    public void awake() {
        if (beating) {
            return;
        }
        beating = true;
        Choreographer.getInstance().postFrameCallback(beat);
    }

    public void sleep() {
        beating = false;
        Choreographer.getInstance().removeFrameCallback(beat);
    }

    private final Choreographer.FrameCallback beat = new Choreographer.FrameCallback() {
        @Override
        public void doFrame(long moment) {
            if (moment - beatAt > 0L) {
                beatAt = moment;
                step();
            }
            if (beating) {
                Choreographer.getInstance().postFrameCallback(this);
            }
        }
    };

    private long beatAt;

    private void step() {
        boolean up = Core.up();
        boolean out = Snapshot.state().optBoolean("egress")
                && Snapshot.state().optBoolean("allowExit");

        if (fresh) {
            fresh = false;
            wasUp = up;
            stage.settle(up);
            stage.spread(up && out);
        }
        if (up != wasUp) {
            wasUp = up;
            if (up) {
                stage.live();
            } else if (!stage.falling()) {
                stage.close();
            }
        }

        String woe = Core.tookIt();
        if (!woe.isEmpty() && !up) {
            stage.failed(woe);
        }

        stage.exit(out);
        stage.tick();
        float at = stage.at();

        paint(at);
        glint();

        float spread = stage.spread();
        flow.shape(span(at, 0f, Stage.GATE), span(spread, 0.30f, 0.85f),
                span(at, Stage.GATE - 0.21f, Stage.GATE),
                at > Stage.GATE + 0.02f);

        boolean apart = spread > 0.55f;
        if (apart != wide) {
            wide = apart;
            place();
        }

        rosterBox.setAlpha(span(at, 0.58f, 0.86f) * dip(spread, 0.30f, 0.80f));
        meters.setAlpha(span(at, 0.88f, 1f) * (1f - span(spread, 0f, 0.15f)));
        wideBox.setAlpha(span(at, 0.88f, 1f) * span(spread, 0.88f, 1f));
        trouble.setAlpha(stage.trouble() == null ? 0f : 1f);
        if (stage.trouble() != null) {
            trouble.setText(stage.trouble());
        }
    }

    public void render() {
        try {
            JSONObject state = Snapshot.state();
            JSONObject about = Snapshot.about();

            String label = about.optString("label", "");
            title.setText(label.isEmpty() ? "qdiver" : label);

            JSONObject sub = state.optJSONObject("subscription");
            refreshLine.setText("обновится " + nextRefresh(sub));

            boolean allowed = state.optBoolean("allowExit");
            egress.setVisibility(allowed ? View.VISIBLE : View.GONE);
            mark.setOn(state.optBoolean("egress") && allowed);

            fillRoster();

            JSONObject s = Snapshot.stats();
            long in = s.optLong("bytesIn");
            long out = s.optLong("bytesOut");
            long now = System.currentTimeMillis();
            if (prevAt > 0 && now > prevAt) {
                double secs = (now - prevAt) / 1000.0;
                double down = (in - prevIn) / secs;
                double up = (out - prevOut) / secs;
                downRate.setText(rate(down));
                upRate.setText(rate(up));
                wideDown.setText(bare(down));
                wideUp.setText(bare(up));
            }
            prevIn = in;
            prevOut = out;
            prevAt = now;
        } catch (Exception ignored) {
        }
    }

    private void fillRoster() {
        JSONArray rows = Snapshot.nodes();
        StringBuilder key = new StringBuilder();
        for (int i = 0; i < rows.length(); i++) {
            JSONObject n = rows.optJSONObject(i);
            if (n == null) {
                continue;
            }
            key.append(n.optString("name")).append(n.optInt("latencyMs", -1))
                    .append(n.optBoolean("selected") ? '*' : '.');
        }
        if (key.toString().equals(rosterKey)) {
            return;
        }
        rosterKey = key.toString();

        roster.removeAllViews();
        for (int i = 0; i < rows.length(); i++) {
            JSONObject n = rows.optJSONObject(i);
            if (n == null) {
                continue;
            }
            roster.addView(line(n));
        }
    }

    private View line(JSONObject node) {
        boolean carrying = node.optBoolean("selected");
        int tone = skin.text;

        LinearLayout row = new LinearLayout(host);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(0, skin.dp(3), 0, skin.dp(3));

        TextView name = skin.label(shorten(node.optString("name")), tone, 12);
        name.setTypeface(Typeface.MONOSPACE);
        name.setSingleLine(true);
        row.addView(name, new LinearLayout.LayoutParams(0,
                LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        TextView dot = skin.label("●", node.optBoolean("reachable") ? LIVE : skin.idle, 8);
        dot.setPadding(skin.dp(8), 0, skin.dp(4), 0);
        row.addView(dot);

        int ms = node.optInt("latencyMs", -1);
        TextView delay = skin.label(ms >= 0 ? ms + "ms" : "—", tone, 12);
        delay.setTypeface(Typeface.MONOSPACE);
        delay.setGravity(Gravity.END);
        row.addView(delay, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        return row;
    }

    private void paint(float at) {
        halo.setSweep(stage.spin());
        halo.setStrength(span(at, 0.02f, 0.12f) * (1f - span(at, Stage.GATE + 0.10f, 0.80f)));

        int tone = skin.idle;
        if (at > 0.01f) {
            float toward = span(at, 0.01f, Stage.GATE);
            tone = (Integer) mix.evaluate(toward, skin.idle, RED);
        }
        if (at > Stage.GATE) {
            float toward = span(at, Stage.GATE, Stage.GATE + 0.18f);
            tone = (Integer) mix.evaluate(toward, RED, LIVE);
        }
        powerFace.setColor(tone);
        halo.setTone(at > Stage.GATE ? LIVE : RED);

        String word = at <= 0.01f ? "подключиться"
                : at < Stage.GATE + 0.09f ? "подключение"
                : stage.falling() ? "отключение" : "подключено";
        if (!word.contentEquals(powerLabel.getText())) {
            powerLabel.setText(word);
        }
        powerLabel.setTextColor(at > 0.12f ? 0xFFFFFFFF : skin.bold);
    }

    private static String shorten(String name) {
        int at = name.lastIndexOf(':');
        return at > 0 ? name.substring(0, at) : name;
    }

    private static float dip(float spread, float gone, float back) {
        return Math.max(1f - span(spread, 0f, gone), span(spread, back, 1f));
    }

    private static float span(float at, float from, float to) {
        if (to <= from) {
            return at >= to ? 1f : 0f;
        }
        float v = (at - from) / (to - from);
        return v < 0f ? 0f : v > 1f ? 1f : v;
    }

    private void flip() {
        if (Core.up()) {
            Core.mark(host, false, "");
            stage.close();
            Intent stop = new Intent(host, TunnelService.class);
            stop.setAction(TunnelService.ACTION_STOP);
            host.startService(stop);
            return;
        }
        Core.tookIt();
        stage.open();
        if (host instanceof MainActivity) {
            ((MainActivity) host).requestStart();
        }
    }

    private void flipExit() {
        if (flipping) {
            return;
        }
        flipping = true;
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    boolean now = Snapshot.state().optBoolean("egress");
                    Core.client(host).setEgress(!now);
                } catch (Exception ignored) {
                }
                Snapshot.refreshState(host);
                flipping = false;
            }
        }).start();
    }

    private void refresh() {
        sparkAt = android.os.SystemClock.elapsedRealtime();
        if (busy) {
            return;
        }
        busy = true;
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Core.client(host).refresh();
                } catch (Exception ignored) {
                }
                busy = false;
            }
        }).start();
    }

    private String nextRefresh(JSONObject sub) {
        if (sub == null) {
            return "при запуске";
        }
        long last = sub.optLong("lastRefresh");
        int minutes = sub.optInt("intervalMinutes");
        if (last <= 0 || minutes <= 0) {
            return "при запуске";
        }
        long left = last + minutes * 60000L - System.currentTimeMillis();
        return left <= 0 ? "сейчас" : "через " + clock(left);
    }

    private static String clock(long ms) {
        long total = ms / 1000;
        long h = total / 3600;
        long m = (total % 3600) / 60;
        long s = total % 60;
        if (h > 0) {
            return String.format("%d:%02d:%02d", h, m, s);
        }
        return String.format("%d:%02d", m, s);
    }

    private static String bare(double perSecond) {
        if (perSecond < 0) {
            perSecond = 0;
        }
        String text = TunnelService.human((long) perSecond);
        int at = text.indexOf(' ');
        return at > 0 ? text.substring(0, at) : text;
    }

    private static String rate(double perSecond) {
        if (perSecond < 0) {
            perSecond = 0;
        }
        return TunnelService.human((long) perSecond) + "/с";
    }
}
