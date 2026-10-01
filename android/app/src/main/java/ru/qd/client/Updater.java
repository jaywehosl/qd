package ru.qd.client;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageInstaller;
import android.os.Build;

import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.OutputStream;

public final class Updater {

    static final String CHANNEL = "updates";
    static final int NOTE = 7;

    private Updater() {
    }

    static boolean install(Context context, String path) {
        try {
            File apk = new File(path);
            PackageInstaller installer = context.getPackageManager().getPackageInstaller();
            PackageInstaller.SessionParams params =
                    new PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL);
            params.setAppPackageName(context.getPackageName());
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                params.setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED);
            }
            int id = installer.createSession(params);
            try (PackageInstaller.Session session = installer.openSession(id)) {
                try (InputStream in = new FileInputStream(apk);
                     OutputStream out = session.openWrite("qd.apk", 0, apk.length())) {
                    byte[] buf = new byte[1 << 16];
                    int n;
                    while ((n = in.read(buf)) > 0) {
                        out.write(buf, 0, n);
                    }
                    session.fsync(out);
                }
                Intent status = new Intent(context, Status.class);
                PendingIntent back = PendingIntent.getBroadcast(context, id, status,
                        PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_MUTABLE);
                context.getSharedPreferences(Boot.SHELF, Context.MODE_PRIVATE)
                        .edit().putBoolean(Boot.RESUME, Core.up()).commit();
                session.commit(back.getIntentSender());
            }
            Core.say(context, "java: update handed to the package installer, session " + id);
            return true;
        } catch (Exception e) {
            Core.say(context, "java: update could not be installed: " + e);
            return false;
        }
    }

    public static final class Status extends BroadcastReceiver {
        @Override
        public void onReceive(Context context, Intent intent) {
            int status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE);
            String message = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE);
            Core.say(context, "java: package installer says " + status + " " + message);

            if (status != PackageInstaller.STATUS_PENDING_USER_ACTION) {
                NotificationManager manager = context.getSystemService(NotificationManager.class);
                if (manager != null) {
                    manager.cancel(NOTE);
                }
                if (status != PackageInstaller.STATUS_SUCCESS) {
                    context.getSharedPreferences(Boot.SHELF, Context.MODE_PRIVATE)
                            .edit().remove(Boot.RESUME).apply();
                    try {
                        Core.client(context).updateRefused("the package installer did not install the update: " + message);
                    } catch (Exception ignored) {
                    }
                }
                return;
            }
            Intent confirm = intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent.class);
            if (confirm == null) {
                return;
            }
            confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            try {
                context.startActivity(confirm);
            } catch (Exception ignored) {
            }
            ask(context, confirm);
        }
    }

    private static void ask(Context context, Intent confirm) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null) {
            return;
        }
        if (manager.getNotificationChannel(CHANNEL) == null) {
            manager.createNotificationChannel(new NotificationChannel(
                    CHANNEL, "Обновления", NotificationManager.IMPORTANCE_HIGH));
        }
        PendingIntent open = PendingIntent.getActivity(context, NOTE, confirm,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification note = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(R.drawable.ic_tile)
                .setContentTitle("Обновление qd готово")
                .setContentText("Нажмите, чтобы установить")
                .setContentIntent(open)
                .setAutoCancel(true)
                .build();
        manager.notify(NOTE, note);
    }
}
