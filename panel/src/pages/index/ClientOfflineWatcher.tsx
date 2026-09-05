import { useEffect, useRef, useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';

import { clientsApi } from '@/generated/client';
import { parseMsg } from '@/utils/zodValidate';
import { LastOnlineMapSchema, type LastOnlineMap } from '@/schemas/inbound';
import { subscribe, getSnapshot, pushEvent } from '@/stores/notificationStore';

const POLL_MS = 60000;

/**
 * Headless Phase-2 sensor: notify when a client that HAS connected before goes
 * silent for longer than the configured threshold (hours). Polls the per-email
 * lastOnline map (epoch ms) only while the sensor is enabled, and edge-triggers
 * per client — fires once on the false→true crossing and re-arms when the client
 * reappears (a fresh disconnect later = a new notification). Clients that have
 * never connected (lastOnline<=0) are skipped so brand-new clients don't alert.
 *
 * Note: the lastOnline map carries no enable flag, so a disabled-but-formerly-
 * online client can still alert — acceptable for now; refine with enable later.
 */
export default function ClientOfflineWatcher() {
  const { sensors } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const cfg = sensors.clientOffline;
  const enabled = !!cfg?.enabled;
  const firing = useRef<Record<string, boolean>>({});

  const { data } = useQuery<LastOnlineMap>({
    queryKey: ['notif', 'clientLastOnline'],
    queryFn: async () => {
      const msg = await clientsApi.lastOnline(undefined, { silent: true });
      const v = parseMsg(msg, LastOnlineMapSchema, 'clients/lastOnline');
      return v?.obj ?? {};
    },
    enabled,
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: false,
    staleTime: 0,
  });

  useEffect(() => {
    if (!enabled || !data) return;
    const thresholdMs = cfg.threshold * 3600000;
    const now = Date.now();
    Object.entries(data).forEach(([email, ts]) => {
      if (!ts || ts <= 0) { firing.current[email] = false; return; }
      const over = now - ts >= thresholdMs;
      if (over && !firing.current[email]) {
        firing.current[email] = true;
        const hours = Math.floor((now - ts) / 3600000);
        pushEvent('warning', `Client "${email}" offline for ${hours}h (threshold ${cfg.threshold}h)`, `client:${email}`);
      } else if (!over && firing.current[email]) {
        firing.current[email] = false;
      }
    });
  }, [enabled, data, cfg?.threshold]);

  return null;
}
