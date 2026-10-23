import { useEffect } from 'react';
import { holdSocket } from '@/api/websocket';

type Handler = (payload: unknown) => void;

export function useWebSocket(handlers: Record<string, Handler>) {
  useEffect(() => holdSocket(handlers),
    []);
}
