package ru.quicdiver.client;

import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.widget.RemoteViews;

public class Widget extends AppWidgetProvider {

    int layout() {
        return R.layout.widget_wide;
    }

    @Override
    public void onUpdate(Context context, AppWidgetManager manager, int[] ids) {
        RemoteViews face = paint(context, layout());
        for (int id : ids) {
            manager.updateAppWidget(id, face);
        }
    }

    public static void refresh(Context context) {
        AppWidgetManager manager = AppWidgetManager.getInstance(context);
        if (manager == null) {
            return;
        }
        repaint(context, manager, Widget.class, R.layout.widget_wide);
        repaint(context, manager, Mid.class, R.layout.widget_mid);
        repaint(context, manager, Small.class, R.layout.widget_small);
    }

    private static void repaint(Context context, AppWidgetManager manager,
                                Class<?> kind, int layout) {
        int[] ids = manager.getAppWidgetIds(new ComponentName(context, kind));
        if (ids == null || ids.length == 0) {
            return;
        }
        manager.updateAppWidget(ids, paint(context, layout));
    }

    private static RemoteViews paint(Context context, int layout) {
        boolean up = Core.up();

        RemoteViews view = new RemoteViews(context.getPackageName(), layout);
        view.setInt(R.id.widget_pill, "setBackgroundResource",
                up ? R.drawable.note_pill_on : R.drawable.note_pill_off);
        view.setTextViewText(R.id.widget_text, Core.turning()
                ? (up ? "отключение" : "подключение")
                : (up ? "подключено" : "подключить"));
        view.setTextColor(R.id.widget_text,
                context.getColor(up ? R.color.ink : R.color.ink_text));
        view.setOnClickPendingIntent(R.id.widget_pill, Notes.powerIntent(context, up));

        if (Core.mayExit()) {
            view.setViewVisibility(R.id.widget_egress, android.view.View.VISIBLE);
            view.setInt(R.id.widget_egress, "setBackgroundResource", R.drawable.note_mark);
            view.setImageViewResource(R.id.widget_egress,
                    Core.exitOn() ? R.drawable.ic_mark_on : R.drawable.ic_mark_off);
            view.setOnClickPendingIntent(R.id.widget_egress, Notes.egressIntent(context));
        } else {
            view.setViewVisibility(R.id.widget_egress, android.view.View.GONE);
        }
        return view;
    }

    public static class Mid extends Widget {
        @Override
        int layout() {
            return R.layout.widget_mid;
        }
    }

    public static class Small extends Widget {
        @Override
        int layout() {
            return R.layout.widget_small;
        }
    }
}
