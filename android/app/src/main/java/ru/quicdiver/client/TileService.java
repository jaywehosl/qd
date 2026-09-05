package ru.quicdiver.client;

import android.content.Intent;
import android.graphics.drawable.Icon;
import android.net.VpnService;
import android.service.quicksettings.Tile;

public class TileService extends android.service.quicksettings.TileService {

    private static volatile TileService listening;

    public static void refresh() {
        TileService live = listening;
        if (live != null) {
            live.render();
        }
    }

    @Override
    public void onStartListening() {
        super.onStartListening();
        listening = this;
        render();
    }

    @Override
    public void onStopListening() {
        listening = null;
        super.onStopListening();
    }

    @Override
    public void onClick() {
        super.onClick();

        if (Core.up()) {
            TunnelService live = TunnelService.current();
            if (live != null) {
                live.stopTunnel();
                render();
                return;
            }
            Intent stop = new Intent(this, TunnelService.class);
            stop.setAction(TunnelService.ACTION_STOP);
            startForegroundService(stop);
            return;
        }

        if (VpnService.prepare(this) != null) {
            Intent open = new Intent(this, MainActivity.class);
            open.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK
                    | Intent.FLAG_ACTIVITY_SINGLE_TOP
                    | Intent.FLAG_ACTIVITY_CLEAR_TOP);
            startActivityAndCollapse(android.app.PendingIntent.getActivity(this, 0, open,
                    android.app.PendingIntent.FLAG_UPDATE_CURRENT
                            | android.app.PendingIntent.FLAG_IMMUTABLE));
            return;
        }

        Intent start = new Intent(this, TunnelService.class);
        start.setAction(TunnelService.ACTION_START);
        startForegroundService(start);
    }

    private void render() {
        Tile tile = getQsTile();
        if (tile == null) {
            return;
        }

        boolean on = Core.up();
        tile.setState(on ? Tile.STATE_ACTIVE : Tile.STATE_INACTIVE);
        tile.setIcon(Icon.createWithResource(this, R.drawable.ic_tile));
        tile.setLabel("qd");
        tile.setSubtitle(on ? "вкл" : "выкл");
        tile.updateTile();
    }
}
