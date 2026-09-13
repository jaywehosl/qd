package ru.quicdiver.client;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.util.Log;

import qdmobile.Client;

public class Buttons extends BroadcastReceiver {

    public static final String ACTION_EGRESS = "ru.quicdiver.client.EGRESS";
    public static final String ACTION_KEEP = "ru.quicdiver.client.KEEP";

    @Override
    public void onReceive(Context context, Intent intent) {
        if (intent == null) {
            return;
        }

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
