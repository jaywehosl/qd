import { useSyncExternalStore } from 'react';
import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';
import {
  CheckCircleFilled,
  CloseCircleFilled,
  ExclamationCircleFilled,
  InfoCircleFilled,
} from '@ant-design/icons';
import { recordToast, type Severity } from '@/stores/notificationStore';

export type ToastType = 'success' | 'error' | 'warning' | 'info';

/** Toast kind → notification severity (success/info are both informational). */
const TOAST_SEVERITY: Record<ToastType, Severity> = {
  success: 'info',
  info: 'info',
  warning: 'warning',
  error: 'danger',
};

interface ToastItem {
  id: number;
  type: ToastType;
  content: ReactNode;
}

// ---- module store (imperative, framework-agnostic) ------------------------
let items: ToastItem[] = [];
const listeners = new Set<() => void>();
let seq = 0;

function emit() {
  listeners.forEach((l) => l());
}
function subscribe(l: () => void) {
  listeners.add(l);
  return () => { listeners.delete(l); };
}
function getSnapshot() {
  return items;
}

function dismiss(id: number) {
  items = items.filter((i) => i.id !== id);
  emit();
}

function push(type: ToastType, content: ReactNode, duration = 3000) {
  const id = ++seq;
  items = [...items, { id, type, content }];
  emit();
  // Mirror into the notification history log (string content only).
  if (typeof content === 'string') {
    recordToast(TOAST_SEVERITY[type], content);
  }
  if (duration > 0) {
    window.setTimeout(() => dismiss(id), duration);
  }
  return id;
}

/** Imperative toast API (drop-in for antd `message`'s success/error/...). */
export const toast = {
  success: (content: ReactNode, duration?: number) => push('success', content, duration),
  error: (content: ReactNode, duration?: number) => push('error', content, duration),
  warning: (content: ReactNode, duration?: number) => push('warning', content, duration),
  info: (content: ReactNode, duration?: number) => push('info', content, duration),
  /** antd-compat: returns [api, contextHolder]; the viewport is global so holder is null. */
  useMessage: () => [toast, null] as const,
  /** antd-compat no-op (DS toasts mount in their own global viewport). */
  config: (_opts?: unknown) => { void _opts; },
};

export type ToastApi = typeof toast;

const ICONS: Record<ToastType, ReactNode> = {
  success: <CheckCircleFilled />,
  error: <CloseCircleFilled />,
  warning: <ExclamationCircleFilled />,
  info: <InfoCircleFilled />,
};

/** Mount once near the app root (each entry). Renders the live toast stack. */
export function ToastViewport() {
  const list = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  if (typeof document === 'undefined') return null;
  return createPortal(
    <div className="ds-toast-viewport" role="region" aria-live="polite">
      {list.map((it) => (
        <div key={it.id} className={`ds-toast ds-toast--${it.type}`} onClick={() => dismiss(it.id)}>
          <span className="ds-toast__icon">{ICONS[it.type]}</span>
          <span className="ds-toast__text">{it.content}</span>
        </div>
      ))}
    </div>,
    document.body,
  );
}
