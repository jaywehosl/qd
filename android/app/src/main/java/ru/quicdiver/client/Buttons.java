package ru.quicdiver.client;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.util.Log;

import qdmobile.Client;

// Buttons принимает нажатия из уведомления, которым не нужен туннель.
//
// Через приёмник, а не через службу, намеренно: выход переключается и с
// опущенным туннелем, а служба в этот момент не запущена — поднимать её ради
// одного флага значило бы держать пустую службу переднего плана.
public class Buttons extends BroadcastReceiver {

    public static final String ACTION_EGRESS = "ru.quicdiver.client.EGRESS";
    public static final String ACTION_KEEP = "ru.quicdiver.client.KEEP";

    @Override
    public void onReceive(Context context, Intent intent) {
        if (intent == null) {
            return;
        }

        // С Android 14 пользователь может смахнуть даже уведомление службы
        // переднего плана: setOngoing больше не держит. Возвращаем на место —
        // через него подключаются, и без него клиент становится недоступен.
        if (ACTION_KEEP.equals(intent.getAction())) {
            Notes.wake(context);
            return;
        }

        if (!ACTION_EGRESS.equals(intent.getAction())) {
            return;
        }

        final Context app = context.getApplicationContext();
        final PendingResult done = goAsync();

        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Client client = Core.client(app);
                    client.setEgress(!Core.exitOn());
                } catch (Exception e) {
                    Log.e("qd", "egress", e);
                }
                Core.readExit(app);
                Core.repaint(app);
                done.finish();
            }
        }).start();
    }
}
