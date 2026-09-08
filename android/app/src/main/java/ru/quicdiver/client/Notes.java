package ru.quicdiver.client;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;
import android.util.TypedValue;
import android.widget.RemoteViews;

// Notes собирает уведомление туннеля. Живёт отдельно от службы намеренно:
// уведомление переживает смерть процесса, и вернуть его на место должен уметь и
// тот, кто службу не поднимал, — экран приложения и приёмник загрузки.
//
// Вёрстка своя, потому что системная прячет кнопки до раскрытия, а раскрыть
// уведомление принудительно нельзя: таким API Android не располагает.
public final class Notes {

    static final String CHANNEL = "tunnel";
    static final int ID = 1;

    private Notes() {
    }

    public static void channel(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null || manager.getNotificationChannel(CHANNEL) != null) {
            return;
        }
        NotificationChannel channel = new NotificationChannel(
                CHANNEL, "Туннель", NotificationManager.IMPORTANCE_LOW);
        channel.setShowBadge(false);
        channel.setSound(null, null);
        manager.createNotificationChannel(channel);
    }

    // post вешает уведомление, когда службы нет: туннель опущен, а управление
    // wake поднимает службу вхолостую — только ради уведомления. Обычное
    // уведомление оболочка стирает вместе с процессом, а уведомление службы
    // переднего плана живёт, пока живёт служба.
    public static void wake(Context context) {
        Intent idle = new Intent(context, TunnelService.class);
        idle.setAction(TunnelService.ACTION_IDLE);
        try {
            context.startForegroundService(idle);
        } catch (Exception e) {
            // Система могла не дать поднять службу из фона: тогда хотя бы
            // повесим обычное уведомление.
            post(context);
        }
    }

    // остаётся под рукой.
    public static void post(Context context) {
        channel(context);
        // Службы сейчас может не быть вовсе, а флаг выхода нужен: читаем у клиента.
        Core.readExit(context);
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager != null) {
            manager.notify(ID, build(context));
        }
    }

    public static void drop(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager != null) {
            manager.cancel(ID);
        }
    }

    public static Notification build(Context context) {
        channel(context);

        boolean up = Core.up();
        String where = Core.where();
        String said = Core.turning()
                ? (up ? "отключение" : "подключение")
                : (up ? "подключено" : "подключить");

        RemoteViews face = new RemoteViews(context.getPackageName(), R.layout.note);
        // Отступы нулевые намеренно. Отрицательные вытягивали плашку за край
        // карточки, но её собственные углы лежат ровно в этом свесе, и карточка
        // их срезала: со скруглениями они несовместимы.
        for (int side : new int[]{RemoteViews.MARGIN_TOP, RemoteViews.MARGIN_BOTTOM,
                RemoteViews.MARGIN_START, RemoteViews.MARGIN_END}) {
            face.setViewLayoutMargin(R.id.note_pill, side, 0, TypedValue.COMPLEX_UNIT_DIP);
        }
        face.setInt(R.id.note_pill, "setBackgroundResource",
                up ? R.drawable.note_pill_on : R.drawable.note_pill_off);
        face.setTextViewText(R.id.note_text, said);
        face.setTextColor(R.id.note_text,
                context.getColor(up ? R.color.ink : R.color.ink_text));
        face.setOnClickPendingIntent(R.id.note_pill, powerIntent(context, up));

        // Выход показывается, только когда есть чем пользоваться: туннель поднят
        // и подписке он разрешён. Подложка у него всегда серая, как на экране, —
        // состояние говорит сам знак.
        // Выход показывается по флагу подписки, а не по туннелю: переключать его
        // можно и с опущенным, ровно как на экране клиента.
        if (Core.mayExit()) {
            face.setViewVisibility(R.id.note_egress, android.view.View.VISIBLE);
            face.setInt(R.id.note_egress, "setBackgroundResource", R.drawable.note_mark);
            // Знаки разные, а не один перекрашенный: у включённого выхода две
            // стойки, у выключенного одна — ровно как рисует Mark на экране.
            face.setImageViewResource(R.id.note_egress,
                    Core.exitOn() ? R.drawable.ic_mark_on : R.drawable.ic_mark_off);
            face.setOnClickPendingIntent(R.id.note_egress, egressIntent(context));
        } else {
            face.setViewVisibility(R.id.note_egress, android.view.View.GONE);
        }

        // Заголовок и текст не видны за своей вёрсткой, но остаются для тех, кто
        // читает уведомление не глазами: экран блокировки, часы, доступность.
        Notification built = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(R.drawable.ic_tile)
                .setContentTitle("qd")
                .setContentText(up
                        ? (where.isEmpty() ? "Подключён" : "Подключён через " + where)
                        : "Отключён")
                .setOngoing(true)
                .setDeleteIntent(keepIntent(context))
                .setCustomContentView(face)
                .setCustomBigContentView(face)
                // Без этого система придерживает уведомление службы до десяти
                // секунд, когда та стартовала из фона — из плитки, например.
                .setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE)
                .build();

        return built;
    }

    // powerIntent: поднять туннель из уведомления можно, только если согласие на
    // VPN уже дано. Диалог согласия показывает лишь активность, поэтому в первый
    // раз кнопка открывает приложение.
    static PendingIntent powerIntent(Context context, boolean up) {
        if (up) {
            return service(context, 1, TunnelService.ACTION_STOP);
        }
        // Подписки нет — подключаться нечем, поэтому кнопка открывает клиент, а
        // не молчит в ответ на нажатие.
        if (!Core.ready()) {
            return openIntent(context);
        }
        if (VpnService.prepare(context) != null) {
            return openIntent(context);
        }
        return service(context, 1, TunnelService.ACTION_START);
    }

    // keepIntent срабатывает, когда уведомление смахнули, и вешает его заново.
    private static PendingIntent keepIntent(Context context) {
        Intent keep = new Intent(context, Buttons.class);
        keep.setAction(Buttons.ACTION_KEEP);
        return PendingIntent.getBroadcast(context, 3, keep,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    static PendingIntent egressIntent(Context context) {
        Intent flip = new Intent(context, Buttons.class);
        flip.setAction(Buttons.ACTION_EGRESS);
        return PendingIntent.getBroadcast(context, 2, flip,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private static PendingIntent service(Context context, int code, String action) {
        Intent want = new Intent(context, TunnelService.class);
        want.setAction(action);
        return PendingIntent.getForegroundService(context, code, want,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private static PendingIntent openIntent(Context context) {
        Intent open = new Intent(context, MainActivity.class);
        open.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(context, 0, open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }
}
