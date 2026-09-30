package ru.qd.client;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.VpnService;

public class Boot extends BroadcastReceiver {

    static final String SHELF = "upkeep";
    static final String RESUME = "resume";

    @Override
    public void onReceive(Context context, Intent intent) {
        String action = intent == null ? null : intent.getAction();
        if (Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)) {
            SharedPreferences shelf = context.getSharedPreferences(SHELF, Context.MODE_PRIVATE);
            boolean resume = shelf.getBoolean(RESUME, false);
            shelf.edit().remove(RESUME).apply();
            if (resume && VpnService.prepare(context) == null) {
                Intent start = new Intent(context, TunnelService.class);
                start.setAction(TunnelService.ACTION_START);
                try {
                    context.startForegroundService(start);
                    return;
                } catch (Exception e) {
                    Core.say(context, "java: tunnel not resumed after update: " + e);
                }
            }
            Notes.wake(context);
            return;
        }
        if (Intent.ACTION_BOOT_COMPLETED.equals(action)) {
            Notes.wake(context);
        }
    }
}
