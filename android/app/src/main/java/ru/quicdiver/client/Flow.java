package ru.quicdiver.client;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.os.SystemClock;
import android.view.View;

public class Flow extends View {

    private static final int SAMPLES = 900;
    private static final float CELL_DP = 10f;
    private static final float PACE_DP = 5.6f;
    private static final float STEP_DP = 2.2f;
    private static final float RAIL = 0.5f;
    private static final float BAND = 0.09f;
    private static final float DASH_Y = 0.80f;
    private static final float TRACE_Y = 0.40f;
    private static final float LINK_Y = 0.70f;
    private static final float RAIL_H = 0.72f;

    private final Skin skin;
    private final Paint dash = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint rail = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint trace = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path path = new Path();

    private static final float[] samples = new float[SAMPLES];
    private static final float[] made = new float[SAMPLES];
    private static int count;
    private static float carried = 0.5f;
    private static float wander;
    private static long slot = Long.MIN_VALUE;

    private static final float[] holeLo = new float[4];
    private static final float[] holeHi = new float[4];
    private static int holes;
    private static boolean cutting;
    private static float clock;
    private static long beat;

    private static float pushL;
    private static float pushR;
    private static float seenL = Float.NaN;
    private static float seenR;

    private float divider;
    private float apart;
    private float fade;
    private static final float[] linked = new float[SAMPLES];
    private static final float[] linkMade = new float[SAMPLES];
    private static int linkCount;
    private static float linkCarried = 0.5f;
    private static float linkWander;
    private static boolean live;

    private final float[] pieces = new float[12];
    private final float[] spare = new float[12];

    public Flow(Context host, Skin skin) {
        super(host);
        this.skin = skin;

        dash.setStyle(Paint.Style.STROKE);
        dash.setStrokeCap(Paint.Cap.ROUND);
        dash.setStrokeWidth(skin.dpf(1.6f));
        dash.setColor(skin.good);

        rail.setStyle(Paint.Style.STROKE);
        rail.setStrokeWidth(skin.dpf(1f));
        rail.setColor(skin.muted);

        trace.setStyle(Paint.Style.STROKE);
        trace.setStrokeCap(Paint.Cap.ROUND);
        trace.setStrokeJoin(Paint.Join.ROUND);
        trace.setStrokeWidth(skin.dpf(1.8f));
        trace.setColor(skin.good);
    }

    public void shape(float divider, float apart, float fade, boolean alive) {
        this.divider = divider;
        this.apart = apart;
        this.fade = fade;

        boolean cut = divider >= (1f - DASH_Y) / RAIL_H;
        if (cut && !cutting) {
            if (holes == holeLo.length) {
                System.arraycopy(holeLo, 1, holeLo, 0, holes - 1);
                System.arraycopy(holeHi, 1, holeHi, 0, holes - 1);
                holes--;
            }
            holeLo[holes] = Float.NaN;
            holeHi[holes] = Float.NaN;
            holes++;
        }
        cutting = cut;

        if (divider <= 0.001f) {
            count = 0;
            linkCount = 0;
        }

        if (alive && !live) {
            carried = 0.5f;
            wander = 0f;
            slot = Long.MIN_VALUE;
        }
        live = alive;

        if (apart > 0.001f && linkCount < SAMPLES) {
            prime(clock);
        }
    }

    private void prime(float slide) {
        float step = skin.dpf(STEP_DP);
        float walk = 0f;
        float held = 0.5f;
        for (int i = SAMPLES - 1; i >= 0; i--) {
            walk = walk * 0.84f + (float) (Math.random() - 0.5) * 0.22f;
            float aim = 0.5f + walk;
            held += (aim - held) * 0.34f;
            held = Math.max(0.08f, Math.min(0.92f, held));
            linked[i] = held;
            linkMade[i] = slide - i * step;
        }
        linkCarried = held;
        linkWander = walk;
        linkCount = SAMPLES;
    }

    private void chew(float mark, float slide) {
        int kept = 0;
        for (int i = 0; i < holes; i++) {
            if (Float.isNaN(holeLo[i]) || holeHi[i] - slide + pushL > 0f) {
                holeLo[kept] = holeLo[i];
                holeHi[kept] = holeHi[i];
                kept++;
            }
        }
        holes = kept;

        if (cutting && holes > 0) {
            int last = holes - 1;
            if (Float.isNaN(holeLo[last])) {
                holeLo[last] = mark;
            }
            holeHi[last] = mark;
        }
    }

    private void carve(Canvas canvas, float from, float to,
                       float left, float right, float y, float slide) {
        int held = 0;
        pieces[held++] = from;
        pieces[held++] = to;

        for (int g = 0; g < holes && held < pieces.length - 2; g++) {
            if (Float.isNaN(holeLo[g])) {
                continue;
            }
            float lo = holeLo[g] - slide + pushL;
            float hi = holeHi[g] - slide + pushL;

            int fresh = 0;
            for (int i = 0; i < held; i += 2) {
                float a = pieces[i];
                float b = pieces[i + 1];
                if (b <= lo || a >= hi) {
                    spare[fresh++] = a;
                    spare[fresh++] = b;
                    continue;
                }
                if (a < lo) {
                    spare[fresh++] = a;
                    spare[fresh++] = lo;
                }
                if (b > hi) {
                    spare[fresh++] = hi;
                    spare[fresh++] = b;
                }
            }
            System.arraycopy(spare, 0, pieces, 0, fresh);
            held = fresh;
        }

        float least = dash.getStrokeWidth();
        for (int i = 0; i < held; i += 2) {
            float a = Math.max(pieces[i], left);
            float b = Math.min(pieces[i + 1], right);
            if (b - a > least) {
                canvas.drawLine(a, y, b, y, dash);
            }
        }
    }

    private void advance() {
        long now = SystemClock.elapsedRealtime();
        long gap = beat == 0L ? 0L : now - beat;
        beat = now;
        if (gap > 0L && gap < 250L) {
            clock += gap / 1000f * skin.dpf(PACE_DP);
        }
    }

    private void feed(float slide) {
        float step = skin.dpf(STEP_DP);
        long now = (long) Math.floor(slide / step);
        if (slot == Long.MIN_VALUE) {
            slot = now;
            return;
        }
        long due = now - slot;
        if (due <= 0) {
            return;
        }
        slot = now;

        for (long i = 0; i < due && i < 8; i++) {
            wander = wander * 0.84f + (float) (Math.random() - 0.5) * 0.22f;
            float aim = 0.5f + wander;
            carried += (aim - carried) * 0.34f;
            carried = Math.max(0.08f, Math.min(0.92f, carried));

            System.arraycopy(samples, 0, samples, 1, SAMPLES - 1);
            System.arraycopy(made, 0, made, 1, SAMPLES - 1);
            samples[0] = carried;
            made[0] = slide - (due - 1 - i) * step;
            if (count < SAMPLES) {
                count++;
            }

            linkWander = linkWander * 0.84f + (float) (Math.random() - 0.5) * 0.22f;
            float linkAim = 0.5f + linkWander;
            linkCarried += (linkAim - linkCarried) * 0.34f;
            linkCarried = Math.max(0.08f, Math.min(0.92f, linkCarried));

            System.arraycopy(linked, 0, linked, 1, SAMPLES - 1);
            System.arraycopy(linkMade, 0, linkMade, 1, SAMPLES - 1);
            linked[0] = linkCarried;
            linkMade[0] = slide - (due - 1 - i) * step;
            if (linkCount < SAMPLES) {
                linkCount++;
            }
        }
    }

    private void drawLink(Canvas canvas, float from, float to, float h, float slide) {
        float step = skin.dpf(STEP_DP);
        float band = h * BAND;
        float y = h * LINK_Y;
        float split = step * 2.5f;
        float lip = rail.getStrokeWidth();

        path.reset();
        boolean began = false;
        float last = 0f;

        if (live) {
            path.moveTo(to + lip, y - (linkCarried - 0.5f) * band);
            began = true;
            last = to + lip;
        }

        for (int i = 0; i < linkCount; i++) {
            float px = to - (slide - linkMade[i]);
            if (px > to + lip) {
                continue;
            }
            if (px < from - lip) {
                break;
            }
            float py = y - (linked[i] - 0.5f) * band;
            if (!began || last - px > split) {
                path.moveTo(px, py);
                began = true;
            } else {
                path.lineTo(px, py);
            }
            last = px;
        }

        if (began) {
            trace.setAlpha(255);
            canvas.drawPath(path, trace);
        }
    }

    @Override
    protected void onDraw(Canvas canvas) {
        float w = getWidth();
        float h = getHeight();
        if (w <= 0 || h <= 0) {
            return;
        }

        float mid = w * RAIL;
        float half = mid * apart * 0.5f;
        float leftX = mid - half;
        float x = mid + half;

        float base = h * DASH_Y;
        float cell = skin.dpf(CELL_DP);
        float run = cell * 0.48f;
        advance();
        float slide = clock;
        float phase = slide % cell;

        if (Float.isNaN(seenL)) {
            seenL = leftX;
            seenR = x;
        }
        float wentL = leftX - seenL;
        if (wentL < 0f) {
            pushL += wentL;
        }
        seenL = leftX;

        float wentR = x - seenR;
        if (wentR < 0f) {
            pushR += wentR;
        }
        seenR = x;

        chew(leftX + slide - pushL, slide);

        float gridL = pushL % cell;
        float gridR = pushR % cell;

        float rim = rail.getStrokeWidth() * 0.5f;

        dash.setAlpha(120);
        canvas.save();
        canvas.clipOutRect(leftX - rim, 0f, x + rim, h);
        for (float a = w + cell * 2f; a > -cell * 2f; a -= cell) {
            float from = a - phase;

            float rs = from + gridR;
            if (rs + run > x && rs < w) {
                lay(canvas, rs, rs + run, x, w, base);
            }

            float ls = from + gridL;
            if (ls + run > 0 && ls < leftX) {
                carve(canvas, ls, ls + run, 0f, leftX, base, slide);
            }
        }
        canvas.restore();

        if (live) {
            feed(slide);
        }
        if (apart > 0.01f && linkCount > 0) {
            canvas.save();
            canvas.clipRect(leftX + rim, 0f, x - rim, h);
            drawLink(canvas, leftX, x, h, slide);
            canvas.restore();
        }
        if (count > 0 && fade > 0.004f) {
            canvas.save();
            canvas.clipRect(0f, 0f, leftX - rim, h);
            drawTrace(canvas, leftX, h, slide);
            canvas.restore();
        }

        if (divider > 0.02f) {
            float top = h * (1f - RAIL_H * divider);
            canvas.drawLine(x, top, x, h, rail);
            if (apart > 0.01f) {
                canvas.drawLine(leftX, top, leftX, h, rail);
            }
        }

        if (isShown()) {
            postInvalidateOnAnimation();
        }
    }

    private void lay(Canvas canvas, float from, float to,
                     float left, float right, float y) {
        float a = Math.max(from, left);
        float b = Math.min(to, right);
        if (b - a > dash.getStrokeWidth()) {
            canvas.drawLine(a, y, b, y, dash);
        }
    }

    private void drawTrace(Canvas canvas, float x, float h, float slide) {
        float step = skin.dpf(STEP_DP);
        float band = h * BAND;
        float split = step * 2.5f;
        float lip = rail.getStrokeWidth();

        path.reset();
        boolean began = false;
        float last = 0f;

        if (live && count > 0) {
            path.moveTo(x + lip, h * TRACE_Y - (carried - 0.5f) * band);
            began = true;
            last = x + lip;
        }

        for (int i = 0; i < count; i++) {
            float px = x - (slide - made[i]);
            if (px > x + lip) {
                continue;
            }
            if (px < 0) {
                break;
            }

            float py = h * TRACE_Y - (samples[i] - 0.5f) * band;
            if (!began || last - px > split) {
                path.moveTo(px, py);
                began = true;
            } else {
                path.lineTo(px, py);
            }
            last = px;
        }

        if (began) {
            trace.setAlpha((int) (255f * fade));
            canvas.drawPath(path, trace);
        }
    }

    @Override
    protected void onVisibilityChanged(View from, int visibility) {
        super.onVisibilityChanged(from, visibility);
        if (visibility == VISIBLE) {
            postInvalidateOnAnimation();
        }
    }
}
