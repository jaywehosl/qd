import type { ReactNode } from 'react';

import { readLocalToken } from '@/api/localToken';
import { pushEvent, type Severity } from '@/stores/notificationStore';

export type ToastType = 'success' | 'error' | 'warning' | 'info';

const PANEL_SEVERITY: Record<ToastType, Severity> = {
  success: 'info',
  info: 'info',
  warning: 'warning',
  error: 'danger',
};

const CLIENT_SEVERITY: Record<ToastType, string> = {
  success: 'info',
  info: 'info',
  warning: 'warning',
  error: 'error',
};

export const NOTIFIED = 'qd:notified';

let seq = 0;

function toClient(type: ToastType, text: string) {
  void fetch('/client/api/notifications/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-QD-Token': readLocalToken() || '' },
    body: JSON.stringify({ severity: CLIENT_SEVERITY[type], text }),
  })
    .catch(() => undefined)
    .finally(() => window.dispatchEvent(new Event(NOTIFIED)));
}

function push(type: ToastType, content: ReactNode) {
  const id = ++seq;
  if (typeof content !== 'string' || !content || typeof window === 'undefined') return id;
  if (window.location.pathname.startsWith('/client')) toClient(type, content);
  else pushEvent(PANEL_SEVERITY[type], content);
  return id;
}

export const toast = {
  success: (content: ReactNode, _duration?: number) => push('success', content),
  error: (content: ReactNode, _duration?: number) => push('error', content),
  warning: (content: ReactNode, _duration?: number) => push('warning', content),
  info: (content: ReactNode, _duration?: number) => push('info', content),
  useMessage: () => [toast, null] as const,
  config: (_opts?: unknown) => { void _opts; },
};

export type ToastApi = typeof toast;

export function ToastViewport() {
  return null;
}
