package ru.quicdiver.client;

import android.content.Context;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.Outline;
import android.graphics.drawable.GradientDrawable;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewOutlineProvider;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class Skin {

    public final int ink;
    public final int text;
    public final int bold;
    public final int muted;
    public final int good;
    public final int bad;
    public final int idle;
    public final int card;
    public final int field;
    public final int edge;
    public final int press;

    private final Context host;

    public Skin(Context host) {
        this.host = host;
        ink = host.getColor(R.color.ink);
        text = host.getColor(R.color.ink_text);
        bold = host.getColor(R.color.ink_bold);
        muted = host.getColor(R.color.ink_muted);
        good = host.getColor(R.color.ink_good);
        bad = host.getColor(R.color.ink_bad);
        idle = host.getColor(R.color.ink_idle);
        card = host.getColor(R.color.ink_card);
        field = host.getColor(R.color.ink_field);
        edge = host.getColor(R.color.ink_edge);
        press = host.getColor(R.color.ink_press);
    }

    public int over(int want, int alpha) {
        float a = alpha / 255f;
        return Color.argb(alpha,
                back(Color.red(want), Color.red(ink), a),
                back(Color.green(want), Color.green(ink), a),
                back(Color.blue(want), Color.blue(ink), a));
    }

    private static int back(int want, int under, float a) {
        return Math.max(0, Math.min(255, Math.round((want - (1f - a) * under) / a)));
    }

    public GradientDrawable backdrop() {
        GradientDrawable bg = new GradientDrawable();
        bg.setColor(ink);
        return bg;
    }

    public android.graphics.drawable.Drawable touchable(GradientDrawable face) {
        return new android.graphics.drawable.RippleDrawable(
                android.content.res.ColorStateList.valueOf(press), face, null);
    }

    public float dpf(float value) {
        return value * host.getResources().getDisplayMetrics().density;
    }

    public int dp(int value) {
        return Math.round(value * host.getResources().getDisplayMetrics().density);
    }

    public LinearLayout column() {
        LinearLayout box = new LinearLayout(host);
        box.setOrientation(LinearLayout.VERTICAL);
        return box;
    }

    public LinearLayout card() {
        LinearLayout box = column();
        box.setPadding(dp(16), dp(14), dp(16), dp(16));

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(card);
        bg.setCornerRadius(dp(30));
        bg.setStroke(Math.max(1, dp(1) / 2), edge);
        box.setBackground(bg);

        final float round = dpf(30f);
        box.setOutlineProvider(new ViewOutlineProvider() {
            @Override
            public void getOutline(View view, Outline shape) {
                shape.setRoundRect(0, 0, view.getWidth(), view.getHeight(), round);
            }
        });
        box.setOutlineSpotShadowColor(0x4D000000);
        box.setOutlineAmbientShadowColor(0x4D000000);
        box.setElevation(dpf(20f));
        return box;
    }

    public TextView label(String value, int colour, int size) {
        TextView view = new TextView(host);
        view.setText(value);
        view.setTextColor(colour);
        view.setTextSize(TypedValue.COMPLEX_UNIT_SP, size);
        return view;
    }

    public void shrink(TextView view, int least, int most) {
        view.setMaxLines(1);
        view.setAutoSizeTextTypeUniformWithConfiguration(
                least, most, 1, TypedValue.COMPLEX_UNIT_SP);
    }

    public TextView title(String value) {
        TextView view = label(value, bold, 17);
        view.setTypeface(Typeface.DEFAULT_BOLD);
        view.setPadding(0, 0, 0, dp(10));
        return view;
    }

    public TextView head(String value) {
        TextView view = label(value, text, 26);
        view.setPadding(0, 0, 0, dp(16));
        return view;
    }

    public TextView note(String value) {
        TextView view = label(value, muted, 13);
        view.setPadding(0, dp(4), 0, 0);
        return view;
    }

    public TextView chip(String value, int tone) {
        TextView view = label(value, 0xFFFFFFFF, 14);
        view.setPadding(dp(14), dp(7), dp(14), dp(7));
        view.setGravity(Gravity.CENTER);

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(tone);
        bg.setCornerRadius(dp(12));
        view.setBackground(bg);
        return view;
    }

    public interface Pick {
        void at(int index);
    }

    public LinearLayout segments(String[] names, int chosen, final Pick pick) {
        LinearLayout row = new LinearLayout(host);
        row.setOrientation(LinearLayout.HORIZONTAL);

        for (int i = 0; i < names.length; i++) {
            final int at = i;
            boolean picked = i == chosen;

            TextView cell = label(names[i], picked ? 0xFFFFFFFF : muted, 13);
            cell.setGravity(Gravity.CENTER);
            shrink(cell, 9, 13);
            cell.setPadding(dp(2), dp(10), dp(2), dp(10));
            cell.setBackground(touchable(pill(picked, i == 0, i == names.length - 1)));
            cell.setOnClickListener(new android.view.View.OnClickListener() {
                @Override
                public void onClick(android.view.View v) {
                    pick.at(at);
                }
            });

            LinearLayout.LayoutParams lp =
                    new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
            if (i > 0) {
                lp.leftMargin = dp(3);
            }
            row.addView(cell, lp);
        }
        return row;
    }

    private GradientDrawable pill(boolean picked, boolean first, boolean last) {
        float wide = dp(14);
        float tight = dp(5);

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(picked ? good : idle);
        bg.setCornerRadii(new float[]{
                first ? wide : tight, first ? wide : tight,
                last ? wide : tight, last ? wide : tight,
                last ? wide : tight, last ? wide : tight,
                first ? wide : tight, first ? wide : tight,
        });
        return bg;
    }

    public GradientDrawable field() {
        GradientDrawable bg = new GradientDrawable();
        bg.setColor(field);
        bg.setCornerRadius(dp(14));
        bg.setStroke(Math.max(1, dp(1) / 2), edge);
        return bg;
    }

    public void frame(android.app.Dialog box) {
        android.view.Window pane = box.getWindow();
        if (pane == null) {
            return;
        }
        pane.setLayout(
                host.getResources().getDisplayMetrics().widthPixels - dp(24) * 2,
                android.view.ViewGroup.LayoutParams.WRAP_CONTENT);
    }

    public TextView cross(int size) {
        TextView shut = label("✕", muted, 15);
        shut.setGravity(Gravity.CENTER);

        GradientDrawable bg = new GradientDrawable();
        bg.setShape(GradientDrawable.OVAL);
        bg.setColor(card);
        bg.setStroke(Math.max(1, dp(1) / 2), edge);
        shut.setBackground(touchable(bg));
        shut.setMinWidth(dp(size));
        shut.setMinHeight(dp(size));
        return shut;
    }

    public TextView button(String caption, int tone) {
        TextView view = label(caption, tone == good ? 0xFFFFFFFF : text, 14);
        view.setGravity(Gravity.CENTER);
        view.setPadding(dp(16), dp(9), dp(16), dp(9));

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(tone);
        bg.setCornerRadius(dp(14));
        view.setBackground(touchable(bg));
        return view;
    }

    public LinearLayout.LayoutParams gap(int bottom) {
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        lp.bottomMargin = dp(bottom);
        return lp;
    }
}
