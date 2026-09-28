-keep class qdmobile.** { *; }
-keep class go.** { *; }
-keep class ru.qd.client.TunnelService { *; }
-keep class ru.qd.client.TileService { *; }
-assumenosideeffects class android.util.Log {
    public static int v(...);
    public static int d(...);
    public static int i(...);
}
