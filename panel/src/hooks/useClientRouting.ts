import { useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import {
  RoutingStateSchema,
  ProcessListSchema,
  type DomainRule,
  type RoutingRole,
  type RoutingRule,
  type RoutingState,
  type RunningProcess,
} from '@/schemas/client-routing';

export type { DomainRule, RoutingRole, RoutingRule, RoutingState, RunningProcess };

const KEY = ['client', 'routing'];
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export function useClientRouting() {
  const queryClient = useQueryClient();

  const query = useQuery<RoutingState | null>({
    queryKey: KEY,
    queryFn: async () => {
      const msg = await HttpUtil.get('/client/api/routing', undefined, { silent: true });
      if (!msg?.success) return null;
      return parseMsg(msg, RoutingStateSchema, 'client/routing').obj ?? null;
    },
    refetchInterval: 15000,
  });

  const write = useCallback(async (rules: RoutingRule[], defaultRole: RoutingRole) => {
    const msg = await HttpUtil.post(
      '/client/api/routing',
      { defaultRole, rules: rules.map(({ process, path, role }) => ({ process, path, role })) },
      JSON_HEADERS,
    );
    if (!msg?.success) return null;
    const next = parseMsg(msg, RoutingStateSchema, 'client/routing').obj ?? null;
    if (next) queryClient.setQueryData(KEY, next);
    return next;
  }, [queryClient]);

  const writeDomains = useCallback(async (domains: Array<{ domain: string; role: RoutingRole }>) => {
    const msg = await HttpUtil.post(
      '/client/api/routing',
      { domains: domains.map(({ domain, role }) => ({ domain, role })) },
      JSON_HEADERS,
    );
    if (!msg?.success) return null;
    const next = parseMsg(msg, RoutingStateSchema, 'client/routing').obj ?? null;
    if (next) queryClient.setQueryData(KEY, next);
    return next;
  }, [queryClient]);

  const writeBundles = useCallback(async (bundles: Array<{ id: string; role: string }>) => {
    const msg = await HttpUtil.post(
      '/client/api/routing',
      { bundles: bundles.map(({ id, role }) => ({ id, role })) },
      JSON_HEADERS,
    );
    if (!msg?.success) return null;
    const next = parseMsg(msg, RoutingStateSchema, 'client/routing').obj ?? null;
    if (next) queryClient.setQueryData(KEY, next);
    return next;
  }, [queryClient]);

  const state = query.data ?? null;

  const setRole = useCallback((id: number, role: RoutingRole) => {
    if (!state) return Promise.resolve(null);
    return write(state.rules.map((r) => (r.id === id ? { ...r, role } : r)), state.defaultRole);
  }, [state, write]);

  const remove = useCallback((id: number) => {
    if (!state) return Promise.resolve(null);
    return write(state.rules.filter((r) => r.id !== id), state.defaultRole);
  }, [state, write]);

  const setDefaultRole = useCallback((role: RoutingRole) => {
    if (!state) return Promise.resolve(null);
    return write(state.rules, role);
  }, [state, write]);

  const add = useCallback((rule: { process: string; path?: string; role: RoutingRole }) => {
    if (!state) return Promise.resolve(null);
    const target = (r: { process: string; path?: string }) => (r.path || r.process).toLowerCase();
    const kept = state.rules.filter((r) => target(r) !== target(rule));
    return write([...kept, { id: -1, ...rule }], state.defaultRole);
  }, [state, write]);

  const reset = useCallback(() => write([], 'tunnel'), [write]);

  const setDomainRole = useCallback((id: number, role: RoutingRole) => {
    if (!state) return Promise.resolve(null);
    return writeDomains(state.domains.map((d) => (d.id === id ? { ...d, role } : d)));
  }, [state, writeDomains]);

  const removeDomain = useCallback((id: number) => {
    if (!state) return Promise.resolve(null);
    return writeDomains(state.domains.filter((d) => d.id !== id));
  }, [state, writeDomains]);

  const addDomain = useCallback((domain: string, role: RoutingRole) => {
    if (!state) return Promise.resolve(null);
    return writeDomains([...state.domains, { domain, role }]);
  }, [state, writeDomains]);

  const resetDomains = useCallback(() => writeDomains([]), [writeDomains]);

  const setBundleRole = useCallback((id: string, role: string) => {
    if (!state) return Promise.resolve(null);
    return writeBundles(state.bundles.map((b) => (b.id === id ? { ...b, role } : b)));
  }, [state, writeBundles]);

  const resetBundles = useCallback(() => writeBundles([]), [writeBundles]);

  const refresh = useCallback(() => { void query.refetch(); }, [query]);

  return {
    state,
    loading: query.isPending,
    setRole,
    setDefaultRole,
    remove,
    add,
    reset,
    setDomainRole,
    removeDomain,
    addDomain,
    resetDomains,
    setBundleRole,
    resetBundles,
    refresh,
  };
}

export async function fetchProcesses(): Promise<RunningProcess[]> {
  const msg = await HttpUtil.get('/client/api/routing/processes', undefined, { silent: true });
  if (!msg?.success) return [];
  return parseMsg(msg, ProcessListSchema, 'client/routing/processes').obj ?? [];
}
