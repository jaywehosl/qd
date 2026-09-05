package ru.quicdiver.client;

import android.os.SystemClock;

public class Stage {

    public static final float GATE = 0.45f;
    public static final float SEAM = 0.30f;

    private static final float RISE = 1.30f;
    private static final float FINISH = 0.80f;
    private static final float LAP = 1.10f;
    private static final float SPLIT = 1.60f;

    private float at;
    private float aim;
    private boolean held;
    private long stamp;
    private long gateAt;
    private String trouble;

    public float at() {
        return at;
    }

    public boolean falling() {
        return aim < at;
    }

    public boolean held() {
        return held && at >= GATE - 0.001f;
    }

    public String trouble() {
        return trouble;
    }

    public void open() {
        trouble = null;
        held = true;
        aim = GATE;
        gateAt = 0;
    }

    public void live() {
        trouble = null;
        held = false;
        aim = 1f;
    }

    public void close() {
        held = false;
        aim = 0f;
    }

    public void failed(String why) {
        trouble = why;
        held = false;
        aim = 0f;
    }

    public void settle(boolean up) {
        at = up ? 1f : 0f;
        aim = at;
        held = false;
    }

    public void exit(boolean on) {
        spreadAim = on ? 1f : 0f;
    }

    public void spread(boolean on) {
        spread = on ? 1f : 0f;
        spreadAim = spread;
    }

    public float spread() {
        return spread;
    }

    private float spread;
    private float spreadAim;

    public float spin() {
        if (!held()) {
            return at < GATE ? at / GATE : 1f;
        }
        if (gateAt == 0) {
            gateAt = SystemClock.elapsedRealtime();
        }
        float turns = (SystemClock.elapsedRealtime() - gateAt) / (LAP * 1000f);
        return turns - (float) Math.floor(turns);
    }

    public void tick() {
        long now = SystemClock.elapsedRealtime();
        if (stamp == 0) {
            stamp = now;
            return;
        }
        float step = (now - stamp) / 1000f;
        stamp = now;
        if (step > 0.25f) {
            step = 0.25f;
        }

        boolean shut = !held && aim <= 0f;

        if (!shut && !held && at >= GATE - 0.001f && at < 0.999f
                && spreadAim > 0f && spread < SEAM) {
            spread = SEAM;
        }

        float wantSpread;
        if (shut) {
            wantSpread = spread > 0f && at > GATE ? SEAM : 0f;
        } else if (!held && at >= GATE - 0.001f) {
            wantSpread = spreadAim;
        } else {
            wantSpread = spread;
        }

        if (spread != wantSpread) {
            float pace = step / SPLIT;
            if (wantSpread > spread) {
                spread = Math.min(wantSpread, spread + pace);
            } else {
                spread = Math.max(wantSpread, spread - pace);
            }
        }

        if (shut && spread > SEAM + 0.001f) {
            return;
        }

        float goal = held ? GATE : aim;
        if (Math.abs(goal - at) < 0.0005f) {
            at = goal;
            return;
        }

        float pace = at < GATE ? GATE / RISE : (1f - GATE) / FINISH;
        float move = pace * step;

        if (goal > at) {
            at = Math.min(goal, at + move);
        } else {
            at = Math.max(goal, at - move * 1.25f);
        }
    }
}
