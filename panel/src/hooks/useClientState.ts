import { useCallback } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import {
  ClientStateSchema,
  ClientNodeListSchema,
  RefreshResultSchema,
  type ClientState,
  type ClientNode,
  type RefreshResult,
} from '@/schemas/client-state';

export type { ClientState, ClientNode };

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
const STATE_KEY = ['client', 'state'];

async function fetchState(): Promise<ClientState | null> {
  const msg = await HttpUtil.get('/client/api/state', undefined, { silent: true });
  if (!msg?.success) return null;
  return parseMsg(msg, ClientStateSchema, 'client/state').obj ?? null;
}

export function useClientState() {
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: STATE_KEY,
    queryFn: fetchState,
    refetchInterval: 3000,
  });

  const put = useCallback((state: ClientState | null) => {
    if (state) queryClient.setQueryData(STATE_KEY, state);
  }, [queryClient]);

  const post = useCallback(async (path: string, body?: unknown) => {
    const msg = body === undefined
      ? await HttpUtil.post(path)
      : await HttpUtil.post(path, body, JSON_HEADERS);
    if (!msg?.success) return null;
    const parsed = parseMsg(msg, ClientStateSchema, path).obj ?? null;
    put(parsed);
    return parsed;
  }, [put]);

  const settle = useCallback(async (path: string) => {
    const next = await post(path);
    await queryClient.invalidateQueries({ queryKey: ['client', 'nodes'] });
    return next;
  }, [post, queryClient]);

  const importUri = useCallback((uri: string) => post('/client/api/import', { uri }), [post]);
  const connect = useCallback(() => settle('/client/api/connect'), [settle]);
  const disconnect = useCallback(() => settle('/client/api/disconnect'), [settle]);
  const setEgress = useCallback((egress: boolean) => post('/client/api/toggle', { egress }), [post]);
  const setAdblock = useCallback((adblock: boolean) => post('/client/api/toggle', { adblock }), [post]);
  const reset = useCallback(
    (subscription = false) => post('/client/api/reset', { subscription }),
    [post],
  );

  const refreshMut = useMutation({
    mutationFn: async (): Promise<RefreshResult | null> => {
      const msg = await HttpUtil.post('/client/api/subscription/refresh');
      if (!msg?.success) return null;
      return parseMsg(msg, RefreshResultSchema, 'client/subscription/refresh').obj ?? null;
    },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['client'] }); },
  });

  return {
    state: query.data ?? null,
    loading: query.isPending,
    refetch: query.refetch,
    importUri,
    connect,
    disconnect,
    setEgress,
    setAdblock,
    reset,
    refreshSubscription: refreshMut.mutateAsync,
    refreshing: refreshMut.isPending,
  };
}

export async function fetchClientNodes(): Promise<ClientNode[]> {
  const msg = await HttpUtil.get('/client/api/nodes', undefined, { silent: true });
  if (!msg?.success) return [];
  return parseMsg(msg, ClientNodeListSchema, 'client/nodes').obj ?? [];
}
