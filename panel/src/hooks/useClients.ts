import { useCallback, useMemo, useRef, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil, Msg } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { markLocalInvalidate } from '@/api/invalidationTracker';
import { useAllSettings } from '@/api/queries/useAllSettings';
import {
  ClientPageResponseSchema,
  EntrypointListSchema,
  OnlinesSchema,
  type ClientDevice,
  type ClientAddress,
  type ClientRecord,
  type ClientTraffic,
  type ClientsSummary,
  type ClientPageResponse,
  type Entrypoint,
} from '@/schemas/client';

export type { ClientRecord, ClientTraffic, ClientDevice, ClientAddress, ClientsSummary, Entrypoint };

export type ClientStatus = '' | 'online' | 'active' | 'expiring' | 'expired' | 'disabled';

export interface ClientQueryParams {
  page: number;
  pageSize: number;
  search?: string;
  group?: string;
  status?: ClientStatus;
  sort?: string;
  order?: 'ascend' | 'descend';
}

const DEFAULT_SUMMARY: ClientsSummary = {
  total: 0, active: 0, online: [], expired: [], expiring: [], disabled: [],
};

type ClientStatRow = ClientTraffic & { tag?: string; enable?: boolean; expiryAt?: number };

async function fetchClientPage(params: ClientQueryParams): Promise<ClientPageResponse> {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') query.set(key, String(value));
  }
  const msg = await HttpUtil.get(`/panel/api/clients/list/paged?${query}`, undefined, { silent: true });
  if (!msg?.success || !msg.obj) throw new Error(msg?.msg || 'Failed to fetch clients');
  const validated = parseMsg(msg, ClientPageResponseSchema, 'clients/list/paged');
  if (!validated.obj) throw new Error('Empty clients response');
  return validated.obj;
}

export async function fetchEntrypoints(): Promise<Entrypoint[]> {
  const msg = await HttpUtil.get('/panel/api/entrypoints', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch entrypoints');
  return parseMsg(msg, EntrypointListSchema, 'entrypoints').obj ?? [];
}

export function useEntrypoints() {
  const query = useQuery({ queryKey: keys.entrypoints(), queryFn: fetchEntrypoints, staleTime: Infinity });
  return query.data ?? [];
}

export function useClients() {
  const queryClient = useQueryClient();
  const { allSetting } = useAllSettings();

  const [query, setQueryState] = useState<ClientQueryParams>({ page: 1, pageSize: 25 });
  const setQuery = useCallback((next: ClientQueryParams) => {
    setQueryState((prev) => (JSON.stringify(prev) === JSON.stringify(next) ? prev : next));
  }, []);

  const listQuery = useQuery({
    queryKey: keys.clients.list(query),
    queryFn: () => fetchClientPage(query),
    staleTime: Infinity,
    placeholderData: keepPreviousData,
  });

  const onlinesQuery = useQuery({
    queryKey: keys.clients.onlines(),
    queryFn: async () => {
      const msg = await HttpUtil.get('/panel/api/clients/onlines', undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch onlines');
      return parseMsg(msg, OnlinesSchema, 'clients/onlines').obj ?? [];
    },
    staleTime: Infinity,
  });

  const onlines = useMemo(() => onlinesQuery.data ?? [], [onlinesQuery.data]);
  const summary = useMemo<ClientsSummary>(() => {
    const served = listQuery.data?.summary ?? DEFAULT_SUMMARY;
    return onlinesQuery.data ? { ...served, online: onlinesQuery.data } : served;
  }, [listQuery.data?.summary, onlinesQuery.data]);

  const invalidateAll = useCallback(() => {
    markLocalInvalidate();
    return queryClient.invalidateQueries({ queryKey: keys.clients.root() });
  }, [queryClient]);

  const afterWrite = { onSuccess: (msg: Msg<unknown> | null) => { if (msg?.success) invalidateAll(); } };

  const createMut = useMutation({
    mutationFn: (client: Record<string, unknown>) => HttpUtil.post('/panel/api/clients/add', client),
    ...afterWrite,
  });
  const updateMut = useMutation({
    mutationFn: ({ id, client }: { id: number; client: Record<string, unknown> }) =>
      HttpUtil.post(`/panel/api/clients/update/${id}`, client),
    ...afterWrite,
  });
  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/clients/del/${id}`),
    ...afterWrite,
  });
  const resetTrafficMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/clients/resetTraffic/${id}`),
    ...afterWrite,
  });

  const create = useCallback((client: Record<string, unknown>) => createMut.mutateAsync(client), [createMut]);
  const update = useCallback(
    (id: number, client: Record<string, unknown>) => updateMut.mutateAsync({ id, client }),
    [updateMut],
  );
  const remove = useCallback((id: number) => removeMut.mutateAsync(id), [removeMut]);
  const resetTraffic = useCallback((id: number) => resetTrafficMut.mutateAsync(id), [resetTrafficMut]);
  const setEnable = useCallback((client: ClientRecord, enable: boolean) => update(client.id, { enable }), [update]);

  const queryRef = useRef(query);
  queryRef.current = query;

  const applyTrafficEvent = useCallback((payload: unknown) => {
    const online = (payload as { onlineClients?: string[] } | null)?.onlineClients;
    if (Array.isArray(online)) queryClient.setQueryData(keys.clients.onlines(), online);
  }, [queryClient]);

  const applyClientStatsEvent = useCallback((payload: unknown) => {
    const rows = (payload as { clients?: ClientStatRow[] } | null)?.clients;
    if (!Array.isArray(rows) || rows.length === 0) return;
    const byTag = new Map<string, ClientStatRow>();
    for (const row of rows) if (row?.tag) byTag.set(row.tag, row);
    queryClient.setQueryData<ClientPageResponse>(keys.clients.list(queryRef.current), (prev) => {
      if (!prev) return prev;
      let touched = false;
      const items = prev.items.map((row) => {
        const fresh = byTag.get(row.tag);
        if (!fresh) return row;
        touched = true;
        return {
          ...row,
          enable: fresh.enable ?? row.enable,
          expiryAt: fresh.expiryAt ?? row.expiryAt,
          traffic: { up: fresh.up, down: fresh.down, lastOnline: fresh.lastOnline },
        };
      });
      return touched ? { ...prev, items } : prev;
    });
  }, [queryClient]);

  return {
    clients: listQuery.data?.items ?? [],
    total: listQuery.data?.total ?? 0,
    filtered: listQuery.data?.filtered ?? 0,
    summary,
    allGroups: listQuery.data?.groups ?? [],
    query,
    setQuery,
    onlines,
    loading: listQuery.isPending,
    refreshing: listQuery.isFetching,
    fetched: listQuery.data !== undefined || listQuery.isError,
    fetchError: listQuery.error ? (listQuery.error as Error).message : '',
    expireDiff: (Number(allSetting.expireDiff) || 0) * 86400000,
    pageSize: Number(allSetting.pageSize) || 0,
    refresh: invalidateAll,
    create,
    update,
    remove,
    resetTraffic,
    setEnable,
    applyTrafficEvent,
    applyClientStatsEvent,
  };
}
