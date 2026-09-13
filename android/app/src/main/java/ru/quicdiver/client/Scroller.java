package ru.quicdiver.client;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.RectF;
import android.os.SystemClock;
import android.view.View;
import android.view.animation.Interpolator;
import android.view.animation.PathInterpolator;
import android.widget.ScrollView;

public class Scroller extends ScrollView {

    private static final long HIDE_AFTER = 1400L;
    private static final long SLIDE = 280L;
    private static final float RAIL_SHARE = 0.2f;
    private static final float THUMB_SHARE = 0.3f;

    private final Paint rail = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint grip = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF box = new RectF();
    private final Interpolator ease = new PathInterpolator(0.4f, 0.12f, 0.28f, 1f);

    private final float railMin;
    private final float railWide;
    private final float gripWide;
    private final float gripMin;
    private final float inset;
    private final float edge;
    private final float out;
    private final int railShade;
    private final int gripShade;

    private float shown;
    private float base;
    private float aim;
    private long began;

    private final Runnable sleep = new Runnable() {
        @Override
        public void run() {
            if (aim == 0f) {
                return;
            }
            base = shown;
            aim = 0f;
            began = SystemClock.uptimeMillis();
            invalidate();
        }
    };

    public Scroller(Context host, Skin skin) {
        super(host);
        setVerticalScrollBarEnabled(false);

        railMin = skin.dpf(64f);
        railWide = skin.dpf(10f);
        gripWide = skin.dpf(6f);
        gripMin = skin.dpf(10f);
        inset = skin.dpf(2f);
        edge = skin.dpf(3f);
        out = skin.dpf(16f);

        rail.setColor(skin.edge);
        grip.setColor(skin.text);
        railShade = Color.alpha(skin.edge);
        gripShade = Math.round(Color.alpha(skin.text) * 0.85f);
    }

    @Override
    protected int computeVerticalScrollRange() {
        if (getChildCount() == 0) {
            return getHeight();
        }
        return getChildAt(0).getHeight() + getPaddingTop() + getPaddingBottom();
    }

    @Override
    protected int computeVerticalScrollExtent() {
        return getHeight();
    }

    @Override
    protected void onScrollChanged(int x, int y, int wasX, int wasY) {
        super.onScrollChanged(x, y, wasX, wasY);
        removeCallbacks(sleep);
        postDelayed(sleep, HIDE_AFTER);
        if (aim != 1f) {
            base = shown;
            aim = 1f;
            began = SystemClock.uptimeMillis();
        }
        invalidate();
    }

    @Override
    public void draw(Canvas canvas) {
        super.draw(canvas);

        boolean moving = false;
        if (began != 0L) {
            float turn = (SystemClock.uptimeMillis() - began) / (float) SLIDE;
            if (turn >= 1f) {
                shown = aim;
                began = 0L;
            } else {
                shown = base + (aim - base) * ease.getInterpolation(turn);
                moving = true;
            }
        }
        if (shown > 0.002f) {
            paint(canvas);
        }
        if (moving) {
            postInvalidateOnAnimation();
        }
    }

    private void paint(Canvas canvas) {
        View kid = getChildCount() > 0 ? getChildAt(0) : null;
        if (kid == null) {
            return;
        }
        float view = getHeight() - getPaddingTop() - getPaddingBottom();
        float full = kid.getHeight();
        if (full <= view + 1f || view < 1f) {
            return;
        }

        float railLen = Math.max(railMin, view * RAIL_SHARE);
        float gripLen = Math.max(gripMin, view / full * railLen * THUMB_SHARE);
        float travel = Math.max(0f, railLen - gripLen - inset * 2f);
        float progress = Math.max(0f, Math.min(1f, getScrollY() / (full - view)));

        float right = getWidth() - edge;
        float top = (view - railLen) / 2f;
        float middle = top + railLen / 2f;

        canvas.save();
        canvas.translate((1f - shown) * out, getScrollY());
        canvas.scale(1f, 0.4f + 0.6f * shown, right, middle);

        rail.setAlpha(Math.round(railShade * shown));
        grip.setAlpha(Math.round(gripShade * shown));

        box.set(right - railWide, top, right, top + railLen);
        canvas.drawRoundRect(box, railWide / 2f, railWide / 2f, rail);

        float gripLeft = right - railWide + inset;
        float gripTop = top + inset + travel * progress;
        box.set(gripLeft, gripTop, gripLeft + gripWide, gripTop + gripLen);
        canvas.drawRoundRect(box, gripWide / 2f, gripWide / 2f, grip);

        canvas.restore();
    }
}
