import { useCallback, useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';

export interface ClientNotif {
  id: number;
  severity: 'info' | 'warning' | 'danger' | 'error';
  text: string;
  ts: number;
  read: boolean;
}

interface NotifPayload {
  unread: number;
  items: ClientNotif[];
}

const KEY = ['client', 'notifications'];
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export function useClientNotifications() {
  const queryClient = useQueryClient();

  const { data } = useQuery<NotifPayload>({
    queryKey: KEY,
    queryFn: async () => {
      const msg = await HttpUtil.get<NotifPayload>('/client/api/notifications', undefined, { silent: true });
      const obj = msg?.success ? msg.obj : null;
      return { unread: obj?.unread ?? 0, items: obj?.items ?? [] };
    },
    refetchInterval: 10000,
  });

  const post = useCallback(async (path: string, body?: unknown) => {
    await (body === undefined
      ? HttpUtil.post(path)
      : HttpUtil.post(path, body, JSON_HEADERS));
    await queryClient.invalidateQueries({ queryKey: KEY });
  }, [queryClient]);

  const markRead = useMutation({ mutationFn: () => post('/client/api/notifications/read', { id: 0 }) });
  const dismiss = useMutation({ mutationFn: (id: number) => post('/client/api/notifications/dismiss', { id }) });
  const clear = useMutation({ mutationFn: () => post('/client/api/notifications/clear') });

  const items = useMemo(() => data?.items ?? [], [data]);

  return { items, unread: data?.unread ?? 0, markRead, dismiss, clear };
}
