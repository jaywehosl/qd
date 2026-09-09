package ru.quicdiver.client;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;

// Notes собирает уведомление туннеля. Живёт отдельно от службы намеренно:
// уведомление переживает смерть процесса, и вернуть его на место должен уметь и
// тот, кто службу не поднимал, — экран приложения и приёмник загрузки.
public final class Notes {

    static final String CHANNEL = "tunnel-quiet";
    static final int ID = 1;

    private Notes() {
    }

    public static void channel(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null || manager.getNotificationChannel(CHANNEL) != null) {
            return;
        }
        manager.deleteNotificationChannel("tunnel");
        NotificationChannel channel = new NotificationChannel(
                CHANNEL, "Туннель", NotificationManager.IMPORTANCE_MIN);
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

        // Вёрстка своя была ровно до тех пор, пока смотрели на неё в HyperOS. У
        // других оболочек ширина и высота своей карточки другие, и плашка лезла
        // поверх заголовка. Уведомление службы всё равно нельзя ни спрятать, ни
        // раскрыть принудительно, поэтому оно теперь штатное и в одну строку.
        Notification.Builder note = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(R.drawable.ic_tile)
                .setContentTitle(up
                        ? (where.isEmpty() ? "Подключён" : "Подключён через " + where)
                        : "Отключён")
                .setContentIntent(openIntent(context))
                .setOngoing(true)
                .setShowWhen(false)
                // Без этого система придерживает уведомление службы до десяти
                // секунд, когда та стартовала из фона — из плитки, например.
                .setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE);

        note.addAction(new Notification.Action.Builder(null,
                Core.turning()
                        ? (up ? "Отключение" : "Подключение")
                        : (up ? "Отключить" : "Подключить"),
                powerIntent(context, up)).build());

        if (Core.mayExit()) {
            note.addAction(new Notification.Action.Builder(null,
                    Core.exitOn() ? "−egress" : "+egress",
                    egressIntent(context)).build());
        }

        return note.build();
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
