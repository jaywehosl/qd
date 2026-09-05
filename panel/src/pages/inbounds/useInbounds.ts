import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { DBInbound, coerceInboundJsonField } from '@/models/dbinbound';
import { Protocols } from '@/schemas/primitives';
import { isSSMultiUser } from '@/lib/qd/entry-compat';
import { setDatepicker } from '@/hooks/useDatepicker';
import { keys } from '@/api/queryKeys';
import { SlimInboundListSchema, LastOnlineMapSchema, InboundDetailSchema } from '@/schemas/inbound';
import { OnlinesSchema, OnlineByNodeSchema, ActiveInboundsByNodeSchema } from '@/schemas/client';
import { DefaultsPayloadSchema, type DefaultsPayload } from '@/schemas/defaults';

export interface SubSettings {
  enable: boolean;
  subTitle: string;
  subURI: string;
  subJsonURI: string;
  subJsonEnable: boolean;
  publicHost: string;
}

type DBInboundInstance = InstanceType<typeof DBInbound>;

interface ClientRollup {
  clients: number;
  active: string[];
  deactive: string[];
  depleted: string[];
  expiring: string[];
  online: string[];
  comments: Map<string, string>;
}

const TRACKED_PROTOCOLS: readonly string[] = [
  'qd',
  Protocols.VMESS,
  Protocols.VLESS,
  Protocols.TROJAN,
  Protocols.SHADOWSOCKS,
  Protocols.HYSTERIA,
];

async function fetchSlimInbounds(): Promise<unknown[]> {
  const msg = await HttpUtil.get('/panel/api/inbounds/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch inbounds');
  const validated = parseMsg(msg, SlimInboundListSchema, 'inbounds/list');
  return Array.isArray(validated.obj) ? validated.obj : [];
}

async function fetchOnlineClients(): Promise<string[]> {
  const msg = await HttpUtil.post('/panel/api/clients/onlines', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch onlines');
  const validated = parseMsg(msg, OnlinesSchema, 'clients/onlines');
  return Array.isArray(validated.obj) ? validated.obj : [];
}

async function fetchOnlineClientsByNode(): Promise<Record<string, string[]>> {
  const msg = await HttpUtil.post('/panel/api/clients/onlinesByNode', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch onlinesByNode');
  const validated = parseMsg(msg, OnlineByNodeSchema, 'clients/onlinesByNode');
  return (validated.obj && typeof validated.obj === 'object') ? (validated.obj as Record<string, string[]>) : {};
}

async function fetchActiveInboundsByNode(): Promise<Record<string, string[]>> {
  const msg = await HttpUtil.post('/panel/api/clients/activeInbounds', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch activeInbounds');
  const validated = parseMsg(msg, ActiveInboundsByNodeSchema, 'clients/activeInbounds');
  return (validated.obj && typeof validated.obj === 'object') ? (validated.obj as Record<string, string[]>) : {};
}

function toNodeOnlineMap(data: Record<string, string[]>): Map<number, Set<string>> {
  const map = new Map<number, Set<string>>();
  for (const [key, emails] of Object.entries(data)) {
    if (!Array.isArray(emails)) continue;
    map.set(Number(key), new Set(emails));
  }
  return map;
}

async function fetchLastOnlineMap(): Promise<Record<string, number>> {
  const msg = await HttpUtil.post('/panel/api/clients/lastOnline', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch lastOnline');
  const validated = parseMsg(msg, LastOnlineMapSchema, 'clients/lastOnline');
  return (validated.obj && typeof validated.obj === 'object') ? validated.obj : {};
}

async function fetchDefaultSettings(): Promise<DefaultsPayload> {
  const msg = await HttpUtil.post('/panel/setting/defaultSettings', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch defaults');
  const validated = parseMsg(msg, DefaultsPayloadSchema, 'setting/defaultSettings');
  return validated.obj ?? {};
}

export function useInbounds() {
  const queryClient = useQueryClient();

  const slimQuery = useQuery({
    queryKey: keys.inbounds.slim(),
    queryFn: fetchSlimInbounds,
    staleTime: Infinity,
  });

  const onlinesQuery = useQuery({
    queryKey: keys.clients.onlines(),
    queryFn: fetchOnlineClients,
    staleTime: Infinity,
  });

  const onlinesByNodeQuery = useQuery({
    queryKey: keys.clients.onlinesByNode(),
    queryFn: fetchOnlineClientsByNode,
    staleTime: Infinity,
  });

  const activeInboundsQuery = useQuery({
    queryKey: keys.clients.activeInbounds(),
    queryFn: fetchActiveInboundsByNode,
    staleTime: Infinity,
  });

  const lastOnlineQuery = useQuery({
    queryKey: keys.clients.lastOnline(),
    queryFn: fetchLastOnlineMap,
    staleTime: Infinity,
  });

  const defaultsQuery = useQuery({
    queryKey: keys.settings.defaults(),
    queryFn: fetchDefaultSettings,
    staleTime: Infinity,
  });

  const defaults = defaultsQuery.data ?? {};
  const expireDiff = (defaults.expireDiff ?? 0) * 86400000;
  const trafficDiff = (defaults.trafficDiff ?? 0) * 1073741824;
  const tgBotEnable = !!defaults.tgBotEnable;
  const ipLimitEnable = !!defaults.ipLimitEnable;
  const pageSize = defaults.pageSize ?? 0;
  const remarkModel = defaults.remarkModel || '-io';
  const datepicker = (defaults.datepicker as 'gregorian' | 'jalalian') || 'gregorian';

  const subSettings: SubSettings = useMemo(() => ({
    enable: !!defaults.subEnable,
    subTitle: defaults.subTitle || '',
    subURI: defaults.subURI || '',
    subJsonURI: defaults.subJsonURI || '',
    subJsonEnable: !!defaults.subJsonEnable,
    publicHost: defaults.subDomain || defaults.webDomain || '',
  }), [defaults.subEnable, defaults.subTitle, defaults.subURI, defaults.subJsonURI, defaults.subJsonEnable, defaults.subDomain, defaults.webDomain]);

  useEffect(() => {
    if (defaults.datepicker) setDatepicker(datepicker);
  }, [datepicker, defaults.datepicker]);

  const expireDiffRef = useRef(expireDiff);
  expireDiffRef.current = expireDiff;
  const trafficDiffRef = useRef(trafficDiff);
  trafficDiffRef.current = trafficDiff;

  const [dbInbounds, setDbInbounds] = useState<DBInboundInstance[]>([]);
  const dbInboundsRef = useRef<DBInboundInstance[]>([]);
  dbInboundsRef.current = dbInbounds;

  const [clientCount, setClientCount] = useState<Record<number, ClientRollup>>({});
  const [statsVersion, setStatsVersion] = useState(0);

  const [onlineClients, setOnlineClients] = useState<string[]>([]);
  const onlineClientsRef = useRef<string[]>([]);
  onlineClientsRef.current = onlineClients;

  const onlineByNodeRef = useRef<Map<number, Set<string>>>(new Map());

  const activeByNodeRef = useRef<Map<number, Set<string>>>(new Map());

  const [lastOnlineMap, setLastOnlineMap] = useState<Record<string, number>>({});

  const rollupClients = useCallback(
    (dbInbound: DBInboundInstance, inbound: { clients?: { email?: string; enable?: boolean; comment?: string }[] }): ClientRollup => {
      const clientStats = Array.isArray((dbInbound as { clientStats?: unknown }).clientStats)
        ? (dbInbound as unknown as { clientStats: { email: string; total: number; up: number; down: number; expiryTime: number }[] }).clientStats
        : [];
      const clients = (inbound?.clients?.length ? inbound.clients : clientStats.map((s) => ({
        email: s.email,
        enable: (s as unknown as { enable?: boolean }).enable ?? true,
        comment: undefined as string | undefined,
      })));
      const active: string[] = [];
      const deactive: string[] = [];
      const depleted: string[] = [];
      const expiring: string[] = [];
      const online: string[] = [];
      const comments = new Map<string, string>();
      const now = Date.now();

      const nodeId = dbInbound.nodeId ?? 0;
      const nodeOnline = onlineByNodeRef.current.get(nodeId);
      const activeForNode = activeByNodeRef.current.get(nodeId);
      const inboundActive = activeForNode === undefined || !dbInbound.tag || activeForNode.has(dbInbound.tag);

      if (dbInbound.enable) {
        for (const client of clients) {
          if (client.comment && client.email) comments.set(client.email, client.comment);
          if (client.enable) {
            if (client.email) active.push(client.email);
            if (client.email && inboundActive && nodeOnline?.has(client.email)) online.push(client.email);
          } else if (client.email) {
            deactive.push(client.email);
          }
        }
        for (const stats of clientStats) {
          const exhausted = stats.total > 0 && stats.up + stats.down >= stats.total;
          const expired = stats.expiryTime > 0 && stats.expiryTime <= now;
          if (expired || exhausted) {
            depleted.push(stats.email);
          } else {
            const expiringSoon =
              (stats.expiryTime > 0 && stats.expiryTime - now < expireDiffRef.current) ||
              (stats.total > 0 && stats.total - (stats.up + stats.down) < trafficDiffRef.current);
            if (expiringSoon) expiring.push(stats.email);
          }
        }
      } else {
        for (const client of clients) {
          if (client.email) deactive.push(client.email);
        }
      }

      return {
        clients: clients.length,
        active,
        deactive,
        depleted,
        expiring,
        online,
        comments,
      };
    },
    [],
  );

  const rebuildClientCount = useCallback(() => {
    const counts: Record<number, ClientRollup> = {};
    for (const dbInbound of dbInboundsRef.current) {
      const protocol = dbInbound.protocol;
      if (!TRACKED_PROTOCOLS.includes(protocol)) continue;
      const settings = coerceInboundJsonField(dbInbound.settings) as {
        method?: string;
        clients?: Array<{ email?: string; enable?: boolean; comment?: string }>;
      };
      if (protocol === Protocols.SHADOWSOCKS && !isSSMultiUser({ protocol, settings })) continue;
      counts[dbInbound.id] = rollupClients(dbInbound, { clients: settings.clients });
    }
    setClientCount(counts);
  }, [rollupClients]);

  useEffect(() => {
    if (!slimQuery.data) return;
    const next: DBInboundInstance[] = [];
    const counts: Record<number, ClientRollup> = {};
    for (const row of slimQuery.data as { protocol: string; id: number }[]) {
      const dbInbound = new DBInbound(row) as DBInboundInstance;
      next.push(dbInbound);
      if (TRACKED_PROTOCOLS.includes(row.protocol)) {
        const settings = coerceInboundJsonField(dbInbound.settings) as {
          method?: string;
          clients?: Array<{ email?: string; enable?: boolean; comment?: string }>;
        };
        if (row.protocol === Protocols.SHADOWSOCKS && !isSSMultiUser({ protocol: row.protocol, settings })) continue;
        counts[row.id] = rollupClients(dbInbound, { clients: settings.clients });
      }
    }
    dbInboundsRef.current = next;
    setDbInbounds(next);
    setClientCount(counts);
  }, [slimQuery.data, rollupClients]);

  useEffect(() => {
    if (onlinesQuery.data) {
      onlineClientsRef.current = onlinesQuery.data;
      setOnlineClients(onlinesQuery.data);
    }
  }, [onlinesQuery.data]);

  useEffect(() => {
    if (onlinesByNodeQuery.data) {
      onlineByNodeRef.current = toNodeOnlineMap(onlinesByNodeQuery.data);
      rebuildClientCount();
    }
  }, [onlinesByNodeQuery.data, rebuildClientCount]);

  useEffect(() => {
    if (activeInboundsQuery.data) {
      activeByNodeRef.current = toNodeOnlineMap(activeInboundsQuery.data);
      rebuildClientCount();
    }
  }, [activeInboundsQuery.data, rebuildClientCount]);

  useEffect(() => {
    if (lastOnlineQuery.data) setLastOnlineMap(lastOnlineQuery.data);
  }, [lastOnlineQuery.data]);

  const fetched = (slimQuery.data !== undefined || slimQuery.isError) && (defaultsQuery.data !== undefined || defaultsQuery.isError);
  const fetchErrorSource = slimQuery.error || defaultsQuery.error;
  const fetchError = fetchErrorSource ? (fetchErrorSource as Error).message : '';

  const refresh = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: keys.inbounds.root() }),
      queryClient.invalidateQueries({ queryKey: keys.clients.onlines() }),
      queryClient.invalidateQueries({ queryKey: keys.clients.onlinesByNode() }),
      queryClient.invalidateQueries({ queryKey: keys.clients.activeInbounds() }),
      queryClient.invalidateQueries({ queryKey: keys.clients.lastOnline() }),
      queryClient.invalidateQueries({ queryKey: keys.xray.config() }),
    ]);
  }, [queryClient]);

  const hydrateInbound = useCallback(async (id: number) => {
    const msg = await HttpUtil.get(`/panel/api/inbounds/get/${id}`);
    if (!msg?.success || !msg.obj) return null;
    const validated = parseMsg(msg, InboundDetailSchema, `inbounds/get/${id}`);
    if (!validated.obj) return null;
    const dbInbound = new DBInbound(validated.obj) as DBInboundInstance;
    setDbInbounds((prev) => {
      const next = prev.map((row) => (
        (row as unknown as { id: number }).id === id ? dbInbound : row
      ));
      dbInboundsRef.current = next;
      return next;
    });
    rebuildClientCount();
    return dbInbound;
  }, [rebuildClientCount]);

  const applyTrafficEvent = useCallback(
    (payload: unknown) => {
      if (!payload || typeof payload !== 'object') return;
      const p = payload as { onlineClients?: string[]; onlineByNode?: Record<string, string[]>; activeInbounds?: Record<string, string[]>; lastOnlineMap?: Record<string, number> };
      if (Array.isArray(p.onlineClients)) {
        onlineClientsRef.current = p.onlineClients;
        setOnlineClients(p.onlineClients);
      }
      if (p.onlineByNode && typeof p.onlineByNode === 'object') {
        onlineByNodeRef.current = toNodeOnlineMap(p.onlineByNode);
      }
      if (p.activeInbounds && typeof p.activeInbounds === 'object') {
        activeByNodeRef.current = toNodeOnlineMap(p.activeInbounds);
      }
      if (p.lastOnlineMap && typeof p.lastOnlineMap === 'object') {
        setLastOnlineMap((prev) => ({ ...prev, ...p.lastOnlineMap! }));
      }
      rebuildClientCount();
    },
    [rebuildClientCount],
  );

  const applyClientStatsEvent = useCallback(
    (payload: unknown) => {
      if (!payload || typeof payload !== 'object') return;
      const p = payload as {
        inbounds?: { id: number; up?: number; down?: number; total?: number; enable?: boolean }[];
        clients?: { email: string; up?: number; down?: number; total?: number; expiryTime?: number; enable?: boolean }[];
      };
      let touched = false;

      if (Array.isArray(p.inbounds) && p.inbounds.length > 0) {
        const byId = new Map<number, { id: number; up?: number; down?: number; total?: number; enable?: boolean }>();
        for (const row of p.inbounds) {
          if (row && row.id != null) byId.set(row.id, row);
        }
        for (const ib of dbInboundsRef.current) {
          const upd = byId.get((ib as unknown as { id: number }).id);
          if (!upd) continue;
          const ibRec = ib as unknown as { up: number; down: number; total: number; enable: boolean };
          if (typeof upd.up === 'number') ibRec.up = upd.up;
          if (typeof upd.down === 'number') ibRec.down = upd.down;
          if (typeof upd.total === 'number') ibRec.total = upd.total;
          if (typeof upd.enable === 'boolean') ibRec.enable = upd.enable;
          touched = true;
        }
      }

      if (Array.isArray(p.clients) && p.clients.length > 0) {
        const byEmail = new Map<string, { email: string; up?: number; down?: number; total?: number; expiryTime?: number; enable?: boolean }>();
        for (const row of p.clients) {
          if (row && row.email) byEmail.set(row.email, row);
        }
        for (const ib of dbInboundsRef.current) {
          const stats = (ib as unknown as { clientStats: { email: string; up: number; down: number; total: number; expiryTime: number; enable: boolean }[] }).clientStats;
          if (!Array.isArray(stats)) continue;
          for (let i = 0; i < stats.length; i++) {
            const stat = stats[i];
            const upd = byEmail.get(stat.email);
            if (!upd) continue;
            if (typeof upd.expiryTime === 'number') stat.expiryTime = upd.expiryTime;
            if (typeof upd.enable === 'boolean') stat.enable = upd.enable;
            touched = true;
          }
        }
      }

      if (touched) {
        setStatsVersion((v) => v + 1);
        setDbInbounds((prev) => {
          const next = [...prev];
          dbInboundsRef.current = next;
          return next;
        });
        rebuildClientCount();
      }
    },
    [rebuildClientCount],
  );

  const totals = useMemo(() => {
    const seen = new Map<string, { up: number; down: number }>();
    let up = 0;
    let down = 0;

    for (const ib of dbInbounds) {
      const rec = ib as unknown as {
        nodeId?: number;
        up?: number;
        down?: number;
        clientStats?: { email: string; up?: number; down?: number }[];
      };
      const stats = Array.isArray(rec.clientStats) ? rec.clientStats : [];

      if (stats.length === 0) {
        up += rec.up || 0;
        down += rec.down || 0;
        continue;
      }

      for (const c of stats) {
        if (!c.email) continue;
        const key = `${rec.nodeId ?? 0}:${c.email}`;
        if (seen.has(key)) continue;
        seen.set(key, { up: c.up || 0, down: c.down || 0 });
      }
    }

    for (const c of seen.values()) {
      up += c.up;
      down += c.down;
    }
    return { up, down };
  }, [dbInbounds]);

  return {
    fetched,
    fetchError,
    dbInbounds,
    clientCount,
    onlineClients,
    lastOnlineMap,
    statsVersion,
    totals,
    expireDiff,
    trafficDiff,
    subSettings,
    remarkModel,
    datepicker,
    tgBotEnable,
    ipLimitEnable,
    pageSize,
    refresh,
    hydrateInbound,
    applyTrafficEvent,
    applyClientStatsEvent,
  };
}
