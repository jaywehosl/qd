package ru.quicdiver.client;

import android.annotation.SuppressLint;
import android.content.ComponentName;
import android.content.Context;
import android.os.Build;
import android.provider.Settings;

import qdmobile.Client;
import qdmobile.Host;
import qdmobile.Protector;
import qdmobile.Qdmobile;

public final class Core {

    private static Client client;
    private static volatile boolean up;
    private static volatile String where = "";
    private static volatile long since;
    private static volatile String woe = "";

    private Core() {
    }

    private static final Host HOST = new Host() {
        @Override
        public long establish(String plan) {
            TunnelService live = TunnelService.current();
            return live == null ? -1 : live.establish(plan);
        }

        @Override
        public void teardown() {
            TunnelService live = TunnelService.current();
            if (live != null) {
                live.teardown();
            }
        }

        @Override
        public void note(String text) {
            TunnelService live = TunnelService.current();
            if (live != null) {
                live.note(text);
            }
        }

        @Override
        public String owner(long proto, String source, long sourcePort,
                            String target, long targetPort) {
            TunnelService live = TunnelService.current();
            if (live == null) {
                return "";
            }
            return live.owner((int) proto, source, (int) sourcePort, target, (int) targetPort);
        }
    };

    private static final Protector PROTECTOR = new Protector() {
        @Override
        public boolean protect(long socket) {
            TunnelService live = TunnelService.current();
            if (live == null || !live.protect((int) socket)) {
                return false;
            }
            return live.bind((int) socket);
        }
    };

    public static void say(Context context, String text) {
        try {
            client(context).say(text);
        } catch (Exception ignored) {
        }
    }

    @SuppressLint("HardwareIds")
    public static synchronized Client client(Context context) throws Exception {
        if (client == null) {
            String id = Settings.Secure.getString(
                    context.getContentResolver(), Settings.Secure.ANDROID_ID);
            client = Qdmobile.open(
                    context.getFilesDir().getAbsolutePath(),
                    HOST, PROTECTOR,
                    id == null ? "unknown" : id,
                    Build.MODEL,
                    Build.MANUFACTURER + " " + Build.MODEL);

            boolean loud = (context.getApplicationInfo().flags
                    & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) != 0;
            client.verbose(loud);
        }
        return client;
    }

    public static boolean up() {
        return up;
    }

    public static String where() {
        return where;
    }

    public static long since() {
        return since;
    }

    public static void gaveUp(String why) {
        woe = why == null ? "" : why;
    }

    public static String tookIt() {
        String said = woe;
        woe = "";
        return said;
    }

    public static void mark(Context context, boolean running, String label) {
        if (running && !up) {
            since = System.currentTimeMillis();
        }
        if (!running) {
            since = 0;
        }
        up = running;
        where = label == null ? "" : label;

        TunnelService.refreshNote();

        try {
            android.service.quicksettings.TileService.requestListeningState(
                    context, new ComponentName(context, TileService.class));
        } catch (Exception ignored) {
        }
        TileService.refresh();
    }
}
