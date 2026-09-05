package ru.quicdiver.client;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.net.ConnectivityManager;
import android.net.IpPrefix;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.NetworkRequest;
import android.net.VpnService;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelFileDescriptor;
import android.os.Process;
import android.util.Log;

import java.net.Inet6Address;
import java.net.InetAddress;
import java.net.InetSocketAddress;

import org.json.JSONArray;
import org.json.JSONObject;

import qdmobile.Client;

public class TunnelService extends VpnService {

    public static final String ACTION_START = "ru.quicdiver.client.START";
    public static final String ACTION_STOP = "ru.quicdiver.client.STOP";

    static final String TAG = "quicdiver";

    private static final String CHANNEL = "tunnel";
    private static final int NOTE_ID = 1;


    private static volatile TunnelService live;

    private ParcelFileDescriptor held;
    private Thread worker;
    private volatile Thread watch;
    private ConnectivityManager.NetworkCallback watcher;
    private volatile Network carrier;

    public static TunnelService current() {
        return live;
    }

    void stopTunnel() {
        bringDown();
    }

    @Override
    public void onCreate() {
        super.onCreate();
        live = this;
        watchNetwork();
    }

    private void watchNetwork() {
        ConnectivityManager net = getSystemService(ConnectivityManager.class);
        if (net == null || watcher != null) {
            return;
        }

        watcher = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onAvailable(Network network) {
                carrier = network;
                setUnderlyingNetworks(new Network[]{network});
                Core.say(TunnelService.this, "java: onAvailable " + network);
                tell(network);
            }

            @Override
            public void onLost(Network network) {
                Core.say(TunnelService.this, "java: onLost " + network + " carrier=" + carrier);
                if (network.equals(carrier)) {
                    carrier = null;
                    setUnderlyingNetworks(null);
                }
            }
        };

        NetworkRequest under = new NetworkRequest.Builder()
                .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
                .build();

        try {
            net.registerBestMatchingNetworkCallback(under, watcher,
                    new Handler(Looper.getMainLooper()));
        } catch (Exception e) {
            Log.e(TAG, "network callback", e);
            watcher = null;
        }
    }

    private void tell(Network network) {
        ConnectivityManager net = getSystemService(ConnectivityManager.class);
        NetworkCapabilities caps = net == null ? null : net.getNetworkCapabilities(network);

        String kind = "other";
        if (caps != null) {
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) {
                kind = "wifi";
            } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) {
                kind = "cell";
            } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) {
                kind = "eth";
            }
        }

        final String tag = kind + ":" + network;
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Core.client(TunnelService.this).networkChanged(tag);
                } catch (Exception e) {
                    Log.e(TAG, "networkChanged", e);
                }
            }
        }).start();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent == null ? ACTION_START : intent.getAction();

        if (ACTION_STOP.equals(action)) {
            bringDown();
            return START_NOT_STICKY;
        }

        showNote();
        bringUp();
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        if (watcher != null) {
            try {
                getSystemService(ConnectivityManager.class).unregisterNetworkCallback(watcher);
            } catch (Exception ignored) {
            }
            watcher = null;
        }
        bringDown();
        live = null;
        super.onDestroy();
    }

    @Override
    public void onRevoke() {
        bringDown();
        super.onRevoke();
    }

    int establish(String plan) {
        try {
            JSONObject p = new JSONObject(plan);

            Builder builder = new Builder();
            builder.addAddress(p.getString("localIp"), p.getInt("prefix"));
            builder.addRoute("0.0.0.0", 0);
            builder.addDnsServer(p.getString("dns"));

            try {
                builder.addAddress("fd00:7::2", 64);
                builder.addRoute("::", 0);
                Core.say(this, "java: v6 held by the tunnel and dropped there");
            } catch (Exception e) {
                Log.e(TAG, "v6", e);
                Core.say(this, "java: v6 could not be held: " + e.getMessage());
            }

            JSONArray peers = p.optJSONArray("peers");
            int spared = 0;
            for (int i = 0; peers != null && i < peers.length(); i++) {
                if (spare(builder, peers.getString(i))) {
                    spared++;
                }
            }
            Core.say(this, "java: kept " + spared + " node addresses off the tunnel");

            builder.setMtu(p.getInt("mtu"));
            builder.setSession("QUIC Diver");
            builder.setBlocking(true);
            builder.setConfigureIntent(openIntent());

            if (p.optBoolean("carveOut")) {
                JSONArray include = p.optJSONArray("include");
                int taken = 0;
                for (int i = 0; include != null && i < include.length(); i++) {
                    if (allow(builder, include.getString(i))) {
                        taken++;
                    }
                }
                if (taken == 0) {
                    builder.addAllowedApplication(getPackageName());
                }
            } else {
                JSONArray exclude = p.optJSONArray("exclude");
                for (int i = 0; exclude != null && i < exclude.length(); i++) {
                    deny(builder, exclude.getString(i));
                }
            }

            ParcelFileDescriptor pfd = builder.establish();
            if (pfd == null) {
                Core.say(this, "java: establish refused");
                return -1;
            }
            held = pfd;

            Network under = carrier;
            if (under != null) {
                setUnderlyingNetworks(new Network[]{under});
            }
            Core.say(this, "java: established under " + under + " mtu=" + p.getInt("mtu"));
            return pfd.detachFd();
        } catch (Exception e) {
            Log.e(TAG, "establish", e);
            return -1;
        }
    }

    boolean bind(int socket) {
        Network under = carrier;
        if (under == null) {
            return true;
        }
        ParcelFileDescriptor copy = null;
        try {
            copy = ParcelFileDescriptor.fromFd(socket);
            under.bindSocket(copy.getFileDescriptor());
            return true;
        } catch (Exception e) {
            Log.e(TAG, "bind socket", e);
            return false;
        } finally {
            if (copy != null) {
                try {
                    copy.close();
                } catch (Exception ignored) {
                }
            }
        }
    }

    private boolean spare(Builder builder, String host) {
        try {
            InetAddress at = InetAddress.getByName(host);
            builder.excludeRoute(new IpPrefix(at, at instanceof Inet6Address ? 128 : 32));
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    private boolean allow(Builder builder, String pkg) {
        try {
            builder.addAllowedApplication(pkg);
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    private void deny(Builder builder, String pkg) {
        try {
            builder.addDisallowedApplication(pkg);
        } catch (Exception ignored) {
        }
    }

    void teardown() {
        held = null;
    }

    void note(String text) {
        update(text);
    }

    String owner(int proto, String source, int sourcePort, String target, int targetPort) {
        try {
            ConnectivityManager net = getSystemService(ConnectivityManager.class);
            if (net == null) {
                return "";
            }

            int uid = net.getConnectionOwnerUid(proto,
                    new InetSocketAddress(InetAddress.getByName(source), sourcePort),
                    new InetSocketAddress(InetAddress.getByName(target), targetPort));
            if (uid == Process.INVALID_UID) {
                return "";
            }

            String[] named = getPackageManager().getPackagesForUid(uid);
            if (named == null || named.length == 0) {
                return "";
            }
            return named[0];
        } catch (Exception e) {
            return "";
        }
    }

    private synchronized void bringUp() {
        if (worker != null) {
            return;
        }

        worker = new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Client client = Core.client(TunnelService.this);
                    client.connect();
                    Core.mark(TunnelService.this, true, client.node());
                    startWatch();
                } catch (Exception e) {
                    Log.e(TAG, "bring up", e);
                    try {
                        Thread.sleep(900);
                        Client again = Core.client(TunnelService.this);
                        again.connect();
                        Core.mark(TunnelService.this, true, again.node());
                        startWatch();
                        return;
                    } catch (Exception twice) {
                        Log.e(TAG, "bring up again", twice);
                    }
                    Core.gaveUp(String.valueOf(e.getMessage()));
                    Core.mark(TunnelService.this, false, "");
                    update("Не удалось: " + e.getMessage());
                    stopSelf();
                } finally {
                    worker = null;
                }
            }
        });
        worker.start();
    }

    private void startWatch() {
        Thread previous = watch;
        if (previous != null) {
            previous.interrupt();
        }

        watch = new Thread(new Runnable() {
            @Override
            public void run() {
                while (!Thread.currentThread().isInterrupted() && Core.up()) {
                    try {
                        Client client = Core.client(TunnelService.this);
                        int ping = (int) client.ping();
                        JSONObject s = new JSONObject(client.statsJSON());

                        String node = client.node();
                        Core.mark(TunnelService.this, true,
                                node + (ping >= 0 ? " · " + ping + " ms" : ""));
                        update(node
                                + (ping >= 0 ? " · " + ping + " ms" : " · нет ответа")
                                + " · ↑" + human(s.optLong("bytesOut"))
                                + " ↓" + human(s.optLong("bytesIn")));
                    } catch (Exception e) {
                        Log.e(TAG, "watch", e);
                    }

                    try {
                        Thread.sleep(10000);
                    } catch (InterruptedException stopped) {
                        return;
                    }
                }
            }
        });
        watch.setDaemon(true);
        watch.start();
    }

    static String human(long n) {
        String[] units = {"Б", "КБ", "МБ", "ГБ", "ТБ"};
        double v = n;
        int i = 0;
        while (v >= 1024 && i < units.length - 1) {
            v /= 1024;
            i++;
        }
        return i == 0 ? String.format("%.0f %s", v, units[i])
                : String.format("%.1f %s", v, units[i]);
    }

    private synchronized void bringDown() {
        Thread ticking = watch;
        if (ticking != null) {
            ticking.interrupt();
            watch = null;
        }

        Core.mark(this, false, "");
        held = null;
        stopForeground(STOP_FOREGROUND_REMOVE);

        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    Core.client(TunnelService.this).disconnect();
                } catch (Exception e) {
                    Log.e(TAG, "stopping", e);
                }
                stopSelf();
            }
        }).start();
    }

    static void refreshNote() {
        TunnelService running = live;
        if (running != null) {
            running.showNote();
        }
    }

    private void showNote() {
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (manager == null) {
            return;
        }
        if (manager.getNotificationChannel(CHANNEL) == null) {
            NotificationChannel channel = new NotificationChannel(
                    CHANNEL, "Туннель", NotificationManager.IMPORTANCE_LOW);
            channel.setShowBadge(false);
            channel.setSound(null, null);
            manager.createNotificationChannel(channel);
        }

        String where = Core.where();
        String text = Core.up()
                ? (where.isEmpty() ? "Подключён" : "Подключён через " + where)
                : "Подключаюсь";

        Notification note = new Notification.Builder(this, CHANNEL)
                .setSmallIcon(R.drawable.ic_tile)
                .setContentTitle("QUIC Diver")
                .setContentText(text)
                .setOngoing(true)
                .setContentIntent(openIntent())
                .build();

        try {
            if (Build.VERSION.SDK_INT >= 34) {
                startForeground(NOTE_ID, note, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
            } else {
                startForeground(NOTE_ID, note);
            }
        } catch (Exception e) {
            Log.e(TAG, "foreground", e);
        }
    }

    private PendingIntent openIntent() {
        Intent open = new Intent(this, MainActivity.class);
        open.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(this, 0, open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private void update(String text) {
        Core.say(this, "state: " + text);
    }
}
