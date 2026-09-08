package ru.quicdiver.client;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.PorterDuff;
import android.graphics.Rect;
import android.graphics.RenderEffect;
import android.graphics.Shader;
import android.view.View;

// Glass — матовая подложка плавающей строки. Размыть уже нарисованное родителем
// в Android нечем, поэтому строка снимает то, что под ней, в уменьшенный снимок
// и размывает его. Снимок делается перед кадром: рисовать чужую вью прямо в
// аппаратный холст нельзя, она уезжает из своего места вместе с деревом команд.
public class Glass extends View {

    private static final float SHRINK = 0.25f;

    private final View source;
    private final int ink;
    private final Paint spread = new Paint(Paint.FILTER_BITMAP_FLAG);
    private final int[] mine = new int[2];
    private final int[] theirs = new int[2];
    private final Rect spot = new Rect();

    private Bitmap shot;
    private Canvas into;

    public Glass(Context host, Skin skin, View source) {
        super(host);
        this.source = source;
        this.ink = skin.ink;
        setRenderEffect(RenderEffect.createBlurEffect(
                skin.dpf(6f), skin.dpf(6f), Shader.TileMode.CLAMP));
    }

    public void snap() {
        int wide = getWidth();
        int tall = getHeight();
        if (wide <= 0 || tall <= 0 || source == null || source.getWidth() <= 0) {
            return;
        }
        int bw = Math.max(1, Math.round(wide * SHRINK));
        int bh = Math.max(1, Math.round(tall * SHRINK));
        if (shot == null || shot.getWidth() != bw || shot.getHeight() != bh) {
            shot = Bitmap.createBitmap(bw, bh, Bitmap.Config.ARGB_8888);
            into = new Canvas(shot);
        }

        getLocationInWindow(mine);
        source.getLocationInWindow(theirs);

        into.drawColor(ink, PorterDuff.Mode.SRC);
        into.save();
        into.scale(SHRINK, SHRINK);
        into.translate(theirs[0] - mine[0], theirs[1] - mine[1]);
        try {
            source.draw(into);
        } catch (Throwable ignored) {
        }
        into.restore();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (shot == null) {
            canvas.drawColor(ink);
            return;
        }
        spot.set(0, 0, getWidth(), getHeight());
        canvas.drawBitmap(shot, null, spot, spread);
    }
}
