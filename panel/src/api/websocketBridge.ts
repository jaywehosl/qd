import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { holdSocket } from '@/api/websocket';
import { keys } from '@/api/queryKeys';
import { isRecentLocalInvalidate } from '@/api/invalidationTracker';

type Handler = (payload: unknown) => void;

let invalidateTimer: number | null = null;

export function useWebSocketBridge() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const onInvalidate: Handler = (payload) => {
      const p = payload as { type?: string } | undefined;
      if (!p || p.type !== 'clients') return;
      if (invalidateTimer != null) clearTimeout(invalidateTimer);
      invalidateTimer = window.setTimeout(() => {
        invalidateTimer = null;
        if (isRecentLocalInvalidate()) return;
        queryClient.invalidateQueries({ queryKey: ['clients'] });
      }, 200);
    };

    const onNodes: Handler = (payload) => {
      if (!Array.isArray(payload)) return;
      queryClient.setQueryData(keys.nodes.list(), payload);
    };

    const release = holdSocket({ invalidate: onInvalidate, nodes: onNodes });

    return () => {
      release();
      if (invalidateTimer != null) {
        clearTimeout(invalidateTimer);
        invalidateTimer = null;
      }
    };
  }, [queryClient]);
}
