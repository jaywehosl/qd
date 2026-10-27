import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { holdSocket } from '@/api/websocket';
import { keys } from '@/api/queryKeys';

type Handler = (payload: unknown) => void;

let seenRevision: number | null = null;

export function useWebSocketBridge() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const onRevision: Handler = (payload) => {
      if (typeof payload !== 'number') return;
      if (seenRevision !== null && payload !== seenRevision) {
        queryClient.invalidateQueries({ queryKey: keys.clients.root() });
        queryClient.invalidateQueries({ queryKey: keys.entrypoints() });
      }
      seenRevision = payload;
    };

    const onNodes: Handler = (payload) => {
      if (!Array.isArray(payload)) return;
      queryClient.setQueryData(keys.nodes.list(), payload);
    };

    return holdSocket({ revision: onRevision, nodes: onNodes });
  }, [queryClient]);
}
