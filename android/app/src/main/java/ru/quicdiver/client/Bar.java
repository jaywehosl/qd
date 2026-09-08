package ru.quicdiver.client;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.RectF;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.os.SystemClock;
import android.view.Gravity;
import android.view.View;
import android.widget.LinearLayout;
import android.widget.TextView;

// Bar — плавающая строка перехода по страницам. До неё страницы менялись только
// свайпом и подсказкой со стрелками: догадаться было можно, увидеть — нет.
public class Bar extends LinearLayout {

    public interface Pick {
        void at(int index);
    }

    private static final float SLIDE = 260f;

    private final Skin skin;
    private final TextView[] chips;
    private final Drawable[] glyphs;
    private final Paint fill = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF slot = new RectF();
    private final RectF was = new RectF();
    private final RectF aim = new RectF();
    private final float round;
    private int chosen = -1;
    private long began;

    public Bar(Context host, Skin skin, final Pick pick) {
        super(host);
        this.skin = skin;
        this.round = skin.dpf(18f);

        setOrientation(HORIZONTAL);
        setGravity(Gravity.CENTER);
        setWillNotDraw(false);
        setPadding(skin.dp(7), skin.dp(7), skin.dp(7), skin.dp(7));

        fill.setColor(skin.idle);

        GradientDrawable tray = new GradientDrawable();
        tray.setColor((skin.solid(skin.card) & 0x00FFFFFF) | 0xD9000000);
        tray.setCornerRadius(skin.dp(24));
        tray.setStroke(Math.max(1, skin.dp(1) / 2), skin.edge);
        setBackground(tray);

        // Подключение посередине: это то, ради чего клиент открывают, и рука
        // тянется к центру.
        String[] names = {"маршруты", "подключение", "настройки"};
        int[] icons = {R.drawable.ic_routing, R.drawable.ic_connect, R.drawable.ic_settings};
        int[] pages = {0, 1, 2};

        chips = new TextView[names.length];
        glyphs = new Drawable[names.length];

        for (int i = 0; i < names.length; i++) {
            final int at = pages[i];

            Drawable glyph = host.getResources().getDrawable(icons[i]).mutate();
            glyph.setBounds(0, 0, skin.dp(17), skin.dp(17));
            glyph.setTint(skin.muted);
            glyphs[i] = glyph;

            TextView chip = skin.label(names[i], skin.muted, 12);
            chip.setGravity(Gravity.CENTER);
            chip.setSingleLine(true);
            chip.setPadding(skin.dp(11), skin.dp(19), skin.dp(11), skin.dp(19));
            chip.setCompoundDrawables(glyph, null, null, null);
            chip.setCompoundDrawablePadding(skin.dp(5));
            chip.setBackground(skin.touchable(new GradientDrawable()));
            chip.setOnClickListener(new View.OnClickListener() {
                @Override
                public void onClick(View v) {
                    pick.at(at);
                }
            });

            LayoutParams lp = new LayoutParams(
                    LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, 1f);
            addView(chip, lp);
            chips[i] = chip;
        }
    }

    public void show(int page) {
        if (page == chosen) {
            return;
        }
        chosen = page;

        for (int i = 0; i < chips.length; i++) {
            boolean on = i == page;
            chips[i].setTextColor(on ? skin.bold : skin.muted);
            glyphs[i].setTint(on ? skin.bold : skin.muted);
        }

        if (getWidth() <= 0) {
            began = 0L;
            return;
        }
        was.set(slot.isEmpty() ? seat(page) : slot);
        aim.set(seat(page));
        began = slot.isEmpty() ? 0L : SystemClock.elapsedRealtime();
        if (began == 0L) {
            slot.set(aim);
        }
        invalidate();
    }

    private RectF seat(int page) {
        View chip = chips[page];
        RectF at = new RectF(chip.getLeft(), chip.getTop(), chip.getRight(), chip.getBottom());
        return at;
    }

    @Override
    protected void onLayout(boolean changed, int l, int t, int r, int b) {
        super.onLayout(changed, l, t, r, b);
        if (chosen >= 0 && (changed || slot.isEmpty())) {
            slot.set(seat(chosen));
            aim.set(slot);
            began = 0L;
        }
    }

    @Override
    protected void onDraw(Canvas canvas) {
        super.onDraw(canvas);
        if (chosen < 0 || slot.isEmpty()) {
            return;
        }

        boolean moving = false;
        if (began != 0L) {
            float turn = (SystemClock.elapsedRealtime() - began) / SLIDE;
            if (turn >= 1f) {
                slot.set(aim);
                began = 0L;
            } else {
                float p = turn - 1f;
                float eased = 1f + p * p * p;
                slot.set(
                        was.left + (aim.left - was.left) * eased,
                        was.top + (aim.top - was.top) * eased,
                        was.right + (aim.right - was.right) * eased,
                        was.bottom + (aim.bottom - was.bottom) * eased);
                moving = true;
            }
        }

        canvas.drawRoundRect(slot, round, round, fill);

        if (moving) {
            postInvalidateOnAnimation();
        }
    }
}
