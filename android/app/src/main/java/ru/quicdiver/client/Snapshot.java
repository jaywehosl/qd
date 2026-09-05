package ru.quicdiver.client;

import android.content.Context;

import org.json.JSONObject;

import qdmobile.Client;

public final class Snapshot {

    public static final int NOTHING = 0;
    public static final int CONNECT = 1;
    public static final int SETTINGS = 2;

    private static final JSONObject EMPTY = new JSONObject();

    private static volatile JSONObject state = EMPTY;
    private static volatile JSONObject about = EMPTY;
    private static volatile JSONObject stats = EMPTY;
    private static volatile JSONObject settings = EMPTY;

    private static volatile org.json.JSONArray nodes = new org.json.JSONArray();

    private static volatile int watching = CONNECT;
    private static Thread reader;
    private static int beat;

    private Snapshot() {
    }

    public static void watch(int page) {
        watching = page;
    }

    public static synchronized void start(final Context context) {
        if (reader != null) {
            return;
        }

        reader = new Thread(new Runnable() {
            @Override
            public void run() {
                while (!Thread.currentThread().isInterrupted()) {
                    tick(context);
                    try {
                        Thread.sleep(1000);
                    } catch (InterruptedException stopped) {
                        return;
                    }
                }
            }
        });
        reader.setDaemon(true);
        reader.start();
    }

    private static void tick(Context context) {
        int page = watching;
        try {
            Client client = Core.client(context);
            state = parse(client.stateJSON());
            if (page == CONNECT) {
                about = parse(client.aboutJSON());
                stats = parse(client.statsJSON());
                if (beat++ % 5 == 0) {
                    nodes = rows(client.nodesJSON());
                }
            } else if (page == SETTINGS) {
                about = parse(client.aboutJSON());
            }
        } catch (Exception ignored) {
        }
    }

    public static void refreshSettings(Context context) {
        try {
            settings = parse(Core.client(context).settingsJSON());
        } catch (Exception ignored) {
        }
    }

    public static void refreshState(Context context) {
        try {
            state = parse(Core.client(context).stateJSON());
        } catch (Exception ignored) {
        }
    }

    public static JSONObject state() {
        return state;
    }

    public static JSONObject about() {
        return about;
    }

    public static JSONObject stats() {
        return stats;
    }

    public static JSONObject settings() {
        return settings;
    }


    public static org.json.JSONArray nodes() {
        return nodes;
    }

    private static org.json.JSONArray rows(String raw) {
        try {
            return new org.json.JSONArray(raw);
        } catch (Exception e) {
            return new org.json.JSONArray();
        }
    }

    private static JSONObject parse(String raw) {
        try {
            return new JSONObject(raw);
        } catch (Exception e) {
            return EMPTY;
        }
    }
}
