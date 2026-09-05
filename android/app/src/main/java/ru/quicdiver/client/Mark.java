package ru.quicdiver.client;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.DashPathEffect;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.RectF;
import android.os.SystemClock;
import android.view.View;

public class Mark extends View {

    private static final float SLIDE = 0.26f;

    private final Paint line = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint rail = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint dash = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path path = new Path();
    private final Path frame = new Path();
    private final RectF box = new RectF();

    private final float round;
    private float shown;
    private float aim;
    private long beat;

    public Mark(Context host, Skin skin) {
        super(host);
        round = skin.dpf(18f);

        line.setStyle(Paint.Style.STROKE);
        line.setStrokeCap(Paint.Cap.ROUND);
        line.setStrokeJoin(Paint.Join.ROUND);
        line.setStrokeWidth(skin.dpf(1.3f));
        line.setColor(skin.good);

        rail.setStyle(Paint.Style.STROKE);
        rail.setStrokeWidth(skin.dpf(1f));
        rail.setColor(skin.text);

        dash.setStyle(Paint.Style.STROKE);
        dash.setStrokeCap(Paint.Cap.ROUND);
        dash.setStrokeWidth(skin.dpf(1.1f));
        dash.setColor(skin.good);
        dash.setAlpha(130);
        dash.setPathEffect(new DashPathEffect(
                new float[]{skin.dpf(2f), skin.dpf(2.4f)}, 0f));
    }

    public void setOn(boolean on) {
        float want = on ? 1f : 0f;
        if (want == aim) {
            return;
        }
        aim = want;
        postInvalidateOnAnimation();
    }

    public void settle(boolean on) {
        aim = on ? 1f : 0f;
        shown = aim;
        invalidate();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        float w = getWidth();
        float h = getHeight();
        if (w <= 0 || h <= 0) {
            return;
        }

        long now = SystemClock.elapsedRealtime();
        long gap = beat == 0L ? 0L : now - beat;
        beat = now;
        if (shown != aim && gap > 0L && gap < 250L) {
            float pace = gap / 1000f / SLIDE;
            shown = aim > shown
                    ? Math.min(aim, shown + pace)
                    : Math.max(aim, shown - pace);
        }

        box.set(0f, 0f, w, h);
        frame.reset();
        frame.addRoundRect(box, round, round, Path.Direction.CW);

        canvas.save();
        canvas.clipPath(frame);
        face(canvas, false, -shown * w, w, h);
        face(canvas, true, (1f - shown) * w, w, h);
        canvas.restore();

        if (shown != aim) {
            postInvalidateOnAnimation();
        }
    }

    private void face(Canvas canvas, boolean on, float dx, float w, float h) {
        if (dx <= -w || dx >= w) {
            return;
        }
        float amp = h * 0.028f;

        if (on) {
            float lx = w * 0.356f + dx;
            float rx = w * 0.662f + dx;
            wave(canvas, w * 0.202f + dx, lx, h * 0.295f, amp, 0f);
            wave(canvas, lx, rx, h * 0.555f, amp, 2.1f);
            canvas.drawLine(rx + w * 0.02f, h * 0.645f, w * 0.801f + dx, h * 0.645f, dash);
            canvas.drawLine(lx, h * 0.196f, lx, h * 0.815f, rail);
            canvas.drawLine(rx, h * 0.196f, rx, h * 0.815f, rail);
        } else {
            float mx = w * 0.511f + dx;
            wave(canvas, w * 0.198f + dx, mx, h * 0.299f, amp, 1.2f);
            canvas.drawLine(mx + w * 0.02f, h * 0.665f, w * 0.826f + dx, h * 0.665f, dash);
            canvas.drawLine(mx, h * 0.196f, mx, h * 0.845f, rail);
        }
    }

    private void wave(Canvas canvas, float from, float to, float y, float amp, float seed) {
        path.reset();
        int steps = 16;
        for (int i = 0; i <= steps; i++) {
            float t = i / (float) steps;
            float px = from + (to - from) * t;
            float py = y + (float) (Math.sin(t * 11.2 + seed) * 0.42
                    + Math.sin(t * 24.6 + seed * 1.7) * 0.24) * amp;
            if (i == 0) {
                path.moveTo(px, py);
            } else {
                path.lineTo(px, py);
            }
        }
        canvas.drawPath(path, line);
    }
}
