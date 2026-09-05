package ru.quicdiver.client;

import android.animation.ObjectAnimator;
import android.content.Context;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewConfiguration;
import android.view.ViewGroup;
import android.view.animation.DecelerateInterpolator;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;

public class Pager extends HorizontalScrollView {

    private static final float TRIP = 0.16f;
    private static final int TOSS = 420;
    private static final int GLIDE = 320;

    private final LinearLayout strip;
    private final int slop;

    private ObjectAnimator settling;
    private Runnable onSettle;

    private int page;
    private int from;
    private int settled = -1;
    private float downX;
    private float downY;
    private boolean claiming;

    public Pager(Context context) {
        super(context);
        setHorizontalScrollBarEnabled(false);
        setOverScrollMode(OVER_SCROLL_NEVER);
        setFillViewport(true);
        slop = ViewConfiguration.get(context).getScaledTouchSlop();

        strip = new LinearLayout(context);
        setClipChildren(false);
        setClipToPadding(false);
        strip.setClipChildren(false);
        strip.setClipToPadding(false);
        strip.setOrientation(LinearLayout.HORIZONTAL);
        super.addView(strip, new LayoutParams(
                LayoutParams.WRAP_CONTENT, LayoutParams.MATCH_PARENT));
    }

    public void onSettle(Runnable listener) {
        onSettle = listener;
    }

    public void add(View child) {
        strip.addView(child, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
    }

    public void show(final int index) {
        page = clamp(index);
        if (getWidth() == 0) {
            post(new Runnable() {
                @Override
                public void run() {
                    show(index);
                }
            });
            return;
        }
        glide(page);
    }

    public int page() {
        return page;
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        for (int i = 0; i < strip.getChildCount(); i++) {
            View child = strip.getChildAt(i);
            LinearLayout.LayoutParams lp = (LinearLayout.LayoutParams) child.getLayoutParams();
            lp.width = w;
            child.setLayoutParams(lp);
        }
        post(new Runnable() {
            @Override
            public void run() {
                scrollTo(page * getWidth(), 0);
            }
        });
    }

    @Override
    public void fling(int velocity) {
        settle(velocity);
    }

    // A page can hold a vertical scroller, and that child grabs the gesture
    // first. Claim anything that moves sideways before it gets the chance.
    @Override
    public boolean onInterceptTouchEvent(MotionEvent event) {
        switch (event.getActionMasked()) {
            case MotionEvent.ACTION_DOWN:
                downX = event.getX();
                downY = event.getY();
                claiming = false;
                stopSettling();
                from = nearest();
                break;
            case MotionEvent.ACTION_MOVE:
                if (!claiming) {
                    float dx = Math.abs(event.getX() - downX);
                    float dy = Math.abs(event.getY() - downY);
                    if (dx > slop && dx > dy * 1.2f) {
                        claiming = true;
                    }
                }
                break;
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL:
                claiming = false;
                break;
        }
        return claiming || super.onInterceptTouchEvent(event);
    }

    @Override
    public boolean onTouchEvent(MotionEvent event) {
        int action = event.getActionMasked();
        if (action == MotionEvent.ACTION_DOWN) {
            stopSettling();
            from = nearest();
        }

        boolean handled = super.onTouchEvent(event);

        if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
            settle(0);
        }
        return handled;
    }

    private void settle(int velocity) {
        int width = getWidth();
        if (width == 0) {
            return;
        }

        int drift = getScrollX() - from * width;
        int target = from;

        if (velocity > TOSS || drift > width * TRIP) {
            target = from + 1;
        } else if (velocity < -TOSS || drift < -width * TRIP) {
            target = from - 1;
        }

        glide(clamp(target));
    }

    private void glide(int target) {
        stopSettling();
        page = target;

        int to = target * getWidth();
        if (getScrollX() == to) {
            return;
        }

        settling = ObjectAnimator.ofInt(this, "scrollX", to);
        settling.setDuration(GLIDE);
        settling.setInterpolator(new DecelerateInterpolator(1.6f));
        settling.start();
    }

    private void stopSettling() {
        if (settling != null) {
            settling.cancel();
            settling = null;
        }
    }

    private int nearest() {
        int width = getWidth();
        return width == 0 ? page : clamp((getScrollX() + width / 2) / width);
    }

    @Override
    protected void onScrollChanged(int l, int t, int oldl, int oldt) {
        super.onScrollChanged(l, t, oldl, oldt);
        int width = getWidth();
        if (width == 0 || onSettle == null) {
            return;
        }
        if (l % width == 0) {
            int now = l / width;
            if (now != settled) {
                settled = now;
                page = clamp(now);
                onSettle.run();
            }
        }
    }

    private int clamp(int index) {
        int last = strip.getChildCount() - 1;
        if (index < 0) {
            return 0;
        }
        return index > last ? last : index;
    }
}
