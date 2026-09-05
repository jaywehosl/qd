-keep class qdmobile.** { *; }
-keep class go.** { *; }
-keep class ru.quicdiver.client.TunnelService { *; }
-keep class ru.quicdiver.client.TileService { *; }
-assumenosideeffects class android.util.Log {
    public static int v(...);
    public static int d(...);
    public static int i(...);
}
