import { useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { expectRestart, restarting } from '@/lib/handover';

export interface UpdateOffer {
  version: string;
  state: 'current' | 'behind' | 'required';
}

export interface UpdateInfo {
  version: string;
  build: string;
  status: '' | 'fetching' | 'installing';
  error: string;
  done: number;
  total: number;
  offer?: UpdateOffer;
  postponedUntil?: number;
  failed?: string;
  release?: string;
  updatedFrom?: string;
}

const KEY = ['client', 'update'];

let running = '';
let stale = false;

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (stale && document.hidden) window.location.reload();
  });
}

export function progressOf(info: UpdateInfo | null): number | null {
  if (!info?.status) return null;
  if (info.status === 'installing') return 1;
  return info.total > 0 ? Math.min(1, info.done / info.total) : 0;
}

export function useUpdate() {
  const queryClient = useQueryClient();

  const { data } = useQuery<UpdateInfo | null>({
    queryKey: KEY,
    queryFn: async () => {
      const msg = await HttpUtil.get<UpdateInfo>('/client/api/update', undefined, { silent: true });
      const info = msg?.success ? (msg.obj ?? null) : null;
      if (!info && restarting()) return queryClient.getQueryData<UpdateInfo | null>(KEY) ?? null;
      if (info?.status === 'installing') expectRestart(90000);
      if (info?.version) {
        if (running && running !== info.version) stale = true;
        running = info.version;
      }
      return info;
    },
    refetchInterval: (query) => {
      const held = query.state.data;
      if (held?.status || restarting()) return 400;
      const left = held?.postponedUntil && held.postponedUntil > 0 ? held.postponedUntil - Date.now() + 250 : Infinity;
      return Math.max(400, Math.min(30000, left));
    },
  });

  const run = useCallback(async () => {
    const msg = await HttpUtil.post<UpdateInfo>('/client/api/update', undefined, { silent: true });
    if (msg?.success && msg.obj) queryClient.setQueryData(KEY, msg.obj);
    return msg;
  }, [queryClient]);

  const postpone = useCallback(async (minutes: number) => {
    const msg = await HttpUtil.post<UpdateInfo>('/client/api/update/postpone', { minutes }, {
      headers: { 'Content-Type': 'application/json' },
      silent: true,
    });
    if (msg?.success && msg.obj) queryClient.setQueryData(KEY, msg.obj);
    return msg;
  }, [queryClient]);

  return { info: data ?? null, run, postpone };
}
