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

public class Toggle extends View {

    private static final long SLIDE = 200L;

    private final Paint track = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Paint knob = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF box = new RectF();
    private final Interpolator ease = new PathInterpolator(0.2f, 0.8f, 0.2f, 1f);

    private final int off;
    private final int on;
    private final float wide;
    private final float tall;
    private final float inset;

    private boolean checked;
    private float shown;
    private float base;
    private float aim;
    private long began;

    public Toggle(Context host, Skin skin) {
        super(host);
        off = skin.idle;
        on = skin.good;
        wide = skin.dpf(40f);
        tall = skin.dpf(22f);
        inset = skin.dpf(3f);

        knob.setColor(Color.WHITE);
        knob.setShadowLayer(skin.dpf(2f), 0f, skin.dpf(1f), 0x2E000000);
        setLayerType(LAYER_TYPE_SOFTWARE, null);
    }

    public boolean isChecked() {
        return checked;
    }

    public void setChecked(boolean want) {
        if (want == checked) {
            return;
        }
        checked = want;
        base = shown;
        aim = want ? 1f : 0f;
        began = SystemClock.uptimeMillis();
        invalidate();
    }

    public void settle(boolean want) {
        checked = want;
        aim = want ? 1f : 0f;
        base = aim;
        shown = aim;
        began = 0L;
        invalidate();
    }

    @Override
    protected void onMeasure(int wideSpec, int tallSpec) {
        setMeasuredDimension(Math.round(wide), Math.round(tall));
    }

    @Override
    protected void onDraw(Canvas canvas) {
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

        track.setColor(mix(off, on, shown));
        box.set(0f, 0f, wide, tall);
        canvas.drawRoundRect(box, tall / 2f, tall / 2f, track);

        float size = tall - inset * 2f;
        float travel = wide - size - inset * 2f;
        canvas.drawCircle(inset + size / 2f + travel * shown, tall / 2f, size / 2f, knob);

        if (moving) {
            postInvalidateOnAnimation();
        }
    }

    private static int mix(int from, int to, float at) {
        return Color.rgb(
                Math.round(Color.red(from) + (Color.red(to) - Color.red(from)) * at),
                Math.round(Color.green(from) + (Color.green(to) - Color.green(from)) * at),
                Math.round(Color.blue(from) + (Color.blue(to) - Color.blue(from)) * at));
    }
}
