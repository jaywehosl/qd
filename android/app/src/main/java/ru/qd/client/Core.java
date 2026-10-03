package ru.qd.client;

import android.annotation.SuppressLint;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;
import android.os.Build;
import android.provider.Settings;

import org.json.JSONObject;

import qdmobile.Client;
import qdmobile.Host;
import qdmobile.Protector;
import qdmobile.Qdmobile;

public final class Core {

    static final boolean ONLY_TCP = false;

    private static Client client;
    private static volatile Context app;
    private static volatile boolean up;
    private static volatile String where = "";
    private static volatile boolean exit;
    private static volatile boolean mayExit;
    private static volatile boolean turning;
    private static volatile boolean ready;

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

        @Override
        public boolean install(String path) {
            Context context = app;
            return context != null && Updater.install(context, path);
        }

        @Override
        public boolean raise() {
            Context context = app;
            if (context == null) {
                return false;
            }
            if (VpnService.prepare(context) != null) {
                MainActivity face = MainActivity.live;
                return face != null && face.consent();
            }
            return turn(context, TunnelService.ACTION_START);
        }

        @Override
        public void lower() {
            Context context = app;
            if (context != null) {
                turn(context, TunnelService.ACTION_STOP);
            }
        }

        @Override
        public String apps() {
            return Apps.json(app);
        }

        @Override
        public void changed() {
            Context context = app;
            if (context != null) {
                readExit(context);
                TunnelService.refreshNote(context);
                TileService.refresh();
                Widget.refresh(context);
            }
        }
    };

    private static boolean turn(Context context, String action) {
        Intent ask = new Intent(context, TunnelService.class);
        ask.setAction(action);
        try {
            context.startService(ask);
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    private static final Protector PROTECTOR = new Protector() {
        @Override
        public boolean protect(long socket) {
            TunnelService live = TunnelService.current();
            if (live == null || !live.protect((int) socket)) {
                return false;
            }
            return live.bind((int) socket);
        }

        @Override
        public String lookup(String host) {
            TunnelService live = TunnelService.current();
            return live == null ? "" : live.lookup(host);
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
            app = context.getApplicationContext();
            String id = Settings.Secure.getString(
                    context.getContentResolver(), Settings.Secure.ANDROID_ID);
            Qdmobile.setZone(java.util.TimeZone.getDefault().getID());
            client = Qdmobile.open(
                    context.getFilesDir().getAbsolutePath(),
                    HOST, PROTECTOR,
                    id == null ? "unknown" : id,
                    Build.MODEL,
                    Build.MANUFACTURER + " " + Build.MODEL);

            client.verbose(true);
            client.onlyTCP(ONLY_TCP);
        }
        return client;
    }


    public static boolean exitOn() {
        return exit;
    }

    public static boolean mayExit() {
        return mayExit;
    }

    public static boolean ready() {
        return ready;
    }

    public static void readExit(Context context) {
        try {
            Client client = client(context);
            ready = client.imported();
            JSONObject state = new JSONObject(client.stateJSON());
            exit = state.optBoolean("egress");
            mayExit = state.optBoolean("allowExit");
        } catch (Exception ignored) {
        }
    }
    public static boolean up() {
        return up;
    }

    public static String where() {
        return where;
    }

    public static String plain(String raw) {
        String why = raw == null ? "" : raw.toLowerCase(java.util.Locale.ROOT);
        if (why.contains("stopped before the tunnel came up")) {
            return "";
        }
        if (why.contains("too old") || why.contains("update qd")) {
            return "Эта версия устарела. Обновите приложение, чтобы подключиться.";
        }
        if (why.contains("refused this subscription") || why.contains("disabled by the administrator")
                || why.contains("no longer valid")) {
            return "Сервер отклонил подписку. Обратитесь к администратору.";
        }
        if (why.contains("expired")) {
            return "Срок подписки истёк.";
        }
        if (why.contains("blocked by the administrator")) {
            return "Это устройство заблокировано администратором.";
        }
        if (why.contains("allowance of devices")) {
            return "Достигнут предел устройств для этой подписки.";
        }
        if (why.contains("nothing imported") || why.contains("no network key")) {
            return "Подписка не добавлена.";
        }
        if (why.contains("no entrypoint to dial")) {
            return "В подписке нет серверов для подключения.";
        }
        if (why.contains("refused to establish the tunnel") || why.contains("establish")) {
            return "Android не дал создать VPN. Проверьте, не включён ли другой VPN.";
        }
        if (why.contains("no entrypoint answered") || why.contains("deadline exceeded")
                || why.contains("timeout") || why.contains("no relay answered")) {
            return "Сервер не отвечает. Проверьте интернет и попробуйте ещё раз.";
        }
        return "Не удалось подключиться. Подробности в журнале.";
    }

    public static boolean turning() {
        return turning;
    }

    public static void turning(Context context, boolean on) {
        turning = on;
        repaint(context);
    }

    public static void repaint(Context context) {
        TunnelService.refreshNote(context);
        TileService.refresh();
        Widget.refresh(context);
        MainActivity.poke();
    }

    public static void mark(Context context, boolean running, String label) {
        up = running;
        where = label == null ? "" : label;

        try {
            android.service.quicksettings.TileService.requestListeningState(
                    context, new ComponentName(context, TileService.class));
        } catch (Exception ignored) {
        }
        repaint(context);
    }
}
