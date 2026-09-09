package ru.quicdiver.client;

import android.content.Context;
import android.content.res.ColorStateList;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.RectF;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.os.SystemClock;
import android.text.TextPaint;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.widget.ImageView;
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
    private final View[] chips;
    private final TextView[] caps;
    private final ImageView[] glyphs;
    private final Paint fill = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF slot = new RectF();
    private final RectF was = new RectF();
    private final RectF aim = new RectF();
    private final float round;
    private int chosen = -1;
    private long began;
    private int room = -1;

    public Bar(Context host, Skin skin, final Pick pick) {
        super(host);
        this.skin = skin;
        this.round = skin.dpf(22f);

        setOrientation(HORIZONTAL);
        setGravity(Gravity.CENTER);
        setWillNotDraw(false);
        setPadding(skin.dp(7), skin.dp(7), skin.dp(7), skin.dp(7));

        fill.setColor(skin.idle);

        GradientDrawable tray = new GradientDrawable();
        tray.setColor(skin.over(skin.solid(skin.card), 0xD9));
        tray.setStroke(Math.max(1, skin.dp(1) / 2), skin.edge);
        tray.setCornerRadius(skin.dp(30));
        setBackground(tray);

        // Подключение посередине: это то, ради чего клиент открывают, и рука
        // тянется к центру.
        String[] names = {"маршруты", "подключение", "настройки"};
        int[] icons = {R.drawable.ic_routing, R.drawable.ic_connect, R.drawable.ic_settings};
        int[] pages = {0, 1, 2};

        chips = new View[names.length];
        caps = new TextView[names.length];
        glyphs = new ImageView[names.length];

        for (int i = 0; i < names.length; i++) {
            final int at = pages[i];

            LinearLayout chip = new LinearLayout(host);
            chip.setOrientation(HORIZONTAL);
            chip.setGravity(Gravity.CENTER);
            chip.setPadding(skin.dp(9), skin.dp(17), skin.dp(9), skin.dp(17));
            chip.setBackground(skin.touchable(new GradientDrawable()));
            chip.setOnClickListener(new View.OnClickListener() {
                @Override
                public void onClick(View v) {
                    pick.at(at);
                }
            });

            // Иконка отдельной вью, а не значком при тексте: подбор размера
            // считает свободное место без значка и на узком экране уверенно
            // оставляет надпись обрезанной.
            ImageView glyph = new ImageView(host);
            glyph.setImageResource(icons[i]);
            glyph.setImageTintList(ColorStateList.valueOf(skin.muted));
            chip.addView(glyph, new LayoutParams(skin.dp(16), skin.dp(16)));
            glyphs[i] = glyph;

            TextView name = skin.label(names[i], skin.muted, 12);
            name.setGravity(Gravity.CENTER);
            name.setMaxLines(1);
            LayoutParams nameAt = new LayoutParams(0, LayoutParams.WRAP_CONTENT, 1f);
            nameAt.leftMargin = skin.dp(5);
            chip.addView(name, nameAt);
            caps[i] = name;

            addView(chip, new LayoutParams(0, LayoutParams.WRAP_CONTENT, 1f));
            chips[i] = chip;
        }
    }

    // Подписи ужимаются под ширину экрана вручную: встроенный подбор размера
    // отмеряет текст до того, как вес растянет ячейку, и оставляет максимум.
    // Ширина в dp у соседних по диагонали телефонов разнится на десятую часть,
    // а системный масштаб шрифта добавляет ещё столько же.
    private void measureCaps(int wide) {
        int free = (wide - getPaddingLeft() - getPaddingRight()) / chips.length
                - skin.dp(9) * 2 - skin.dp(16) - skin.dp(5);
        if (free <= 0 || free == room) {
            return;
        }
        room = free;

        float best = 8f;
        for (float sp = 12f; sp >= 8f; sp -= 0.5f) {
            float px = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_SP, sp,
                    getResources().getDisplayMetrics());
            boolean fits = true;
            for (TextView cap : caps) {
                TextPaint ink = new TextPaint(cap.getPaint());
                ink.setTextSize(px);
                if (ink.measureText(cap.getText().toString()) > free) {
                    fits = false;
                    break;
                }
            }
            if (fits) {
                best = sp;
                break;
            }
        }
        for (TextView cap : caps) {
            cap.setTextSize(TypedValue.COMPLEX_UNIT_SP, best);
        }
    }

    @Override
    protected void onMeasure(int wide, int tall) {
        measureCaps(MeasureSpec.getSize(wide));
        super.onMeasure(wide, tall);
    }

    public void show(int page) {
        if (page == chosen) {
            return;
        }
        chosen = page;

        for (int i = 0; i < chips.length; i++) {
            boolean on = i == page;
            caps[i].setTextColor(on ? skin.bold : skin.muted);
            glyphs[i].setImageTintList(ColorStateList.valueOf(on ? skin.bold : skin.muted));
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
        return new RectF(chip.getLeft(), chip.getTop(), chip.getRight(), chip.getBottom());
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
