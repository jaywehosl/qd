package ru.quicdiver.client;

import android.content.Context;
import android.graphics.BlurMaskFilter;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PathMeasure;
import android.graphics.RectF;
import android.widget.FrameLayout;

public class Halo extends FrameLayout {

    private final Skin skin;
    private final Paint glow = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path outline = new Path();
    private final Path piece = new Path();
    private final PathMeasure meter = new PathMeasure();
    private final RectF box = new RectF();

    private float sweep;
    private float strength;
    private int tone;

    public Halo(Context host, Skin skin) {
        super(host);
        this.skin = skin;
        this.tone = skin.good;

        setWillNotDraw(false);
        setLayerType(LAYER_TYPE_SOFTWARE, null);

        glow.setStyle(Paint.Style.STROKE);
        glow.setStrokeCap(Paint.Cap.ROUND);
        glow.setStrokeWidth(skin.dpf(5f));
        glow.setMaskFilter(new BlurMaskFilter(skin.dpf(7f), BlurMaskFilter.Blur.NORMAL));

        int pad = skin.dp(12);
        setPadding(pad, pad, pad, pad);
        girth = skin.dp(27);
    }

    private float girth;

    public void setGirth(float value) {
        girth = value;
    }

    public void setInset(int pad) {
        setPadding(pad, pad, pad, pad);
    }

    public void setWeight(float stroke, float blur) {
        glow.setStrokeWidth(stroke);
        glow.setMaskFilter(new BlurMaskFilter(blur, BlurMaskFilter.Blur.NORMAL));
    }

    public void setSweep(float value) {
        float want = Math.max(0f, Math.min(1f, value));
        if (Math.abs(want - sweep) < 0.004f) {
            return;
        }
        sweep = want;
        if (strength > 0.01f) {
            invalidate();
        }
    }

    public void setStrength(float value) {
        float want = Math.max(0f, Math.min(1f, value));
        if (Math.abs(want - strength) < 0.004f) {
            return;
        }
        boolean was = strength > 0.01f;
        strength = want;
        if (was || strength > 0.01f) {
            invalidate();
        }
    }

    public void setTone(int value) {
        tone = value;
        invalidate();
    }

    private final RectF corner = new RectF();

    private void ring(RectF box, float r) {
        float cx = box.centerX();
        float d = r * 2f;

        outline.reset();
        outline.moveTo(cx, box.top);
        outline.lineTo(box.right - r, box.top);

        corner.set(box.right - d, box.top, box.right, box.top + d);
        outline.arcTo(corner, 270f, 90f, false);
        outline.lineTo(box.right, box.bottom - r);

        corner.set(box.right - d, box.bottom - d, box.right, box.bottom);
        outline.arcTo(corner, 0f, 90f, false);
        outline.lineTo(box.left + r, box.bottom);

        corner.set(box.left, box.bottom - d, box.left + d, box.bottom);
        outline.arcTo(corner, 90f, 90f, false);
        outline.lineTo(box.left, box.top + r);

        corner.set(box.left, box.top, box.left + d, box.top + d);
        outline.arcTo(corner, 180f, 90f, false);
        outline.lineTo(cx, box.top);
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (sweep <= 0.001f || strength <= 0.01f) {
            return;
        }

        box.set(getPaddingLeft(), getPaddingTop(),
                getWidth() - getPaddingRight(), getHeight() - getPaddingBottom());
        if (box.width() <= 0 || box.height() <= 0) {
            return;
        }

        ring(box, girth);

        meter.setPath(outline, true);
        float span = meter.getLength();
        if (span <= 0) {
            return;
        }

        float start = 0f;
        float reach = span * sweep;

        glow.setColor(tone);
        glow.setAlpha((int) (210 * strength));

        piece.reset();
        if (start + reach <= span) {
            meter.getSegment(start, start + reach, piece, true);
        } else {
            meter.getSegment(start, span, piece, true);
            meter.getSegment(0, start + reach - span, piece, true);
        }
        canvas.drawPath(piece, glow);
    }
}
