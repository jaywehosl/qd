import { useEffect } from 'react';
import { holdSocket } from '@/api/websocket';

type Handler = (payload: unknown) => void;

export function useWebSocket(handlers: Record<string, Handler>) {
  useEffect(() => holdSocket(handlers),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
}
