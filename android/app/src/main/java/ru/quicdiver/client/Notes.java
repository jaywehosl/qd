package ru.quicdiver.client;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;

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

    public static void wake(Context context) {
        Intent idle = new Intent(context, TunnelService.class);
        idle.setAction(TunnelService.ACTION_IDLE);
        try {
            context.startForegroundService(idle);
        } catch (Exception e) {
            post(context);
        }
    }

    public static void post(Context context) {
        channel(context);
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

        Notification.Builder note = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(R.drawable.ic_tile)
                .setContentTitle(up
                        ? (where.isEmpty() ? "Подключён" : "Подключён через " + where)
                        : "Отключён")
                .setContentIntent(openIntent(context))
                .setOngoing(true)
                .setShowWhen(false)
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

    static PendingIntent powerIntent(Context context, boolean up) {
        if (up) {
            return service(context, 1, TunnelService.ACTION_STOP);
        }
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
