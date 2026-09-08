package ru.quicdiver.client;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

// Boot возвращает уведомление после перезагрузки: система стирает все
// уведомления при выключении, а это — основной способ поднять туннель, и без
// него пришлось бы каждый раз открывать приложение.
public class Boot extends BroadcastReceiver {

    @Override
    public void onReceive(Context context, Intent intent) {
        if (intent == null || !Intent.ACTION_BOOT_COMPLETED.equals(intent.getAction())) {
            return;
        }
        Notes.wake(context);
    }
}
