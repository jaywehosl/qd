import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import BusyOverlay from '@/components/ui/BusyOverlay';
import {
  BOOT_BUSY_KEY,
  BusyOverlayContext,
  type BusyOverlayOpts,
  type BusyOverlayValue,
} from '@/layouts/busy-overlay-context';

const BOOT_HOLD_MS = 1100;

function readBootBusy(): BusyOverlayOpts | null {
  try {
    const raw = localStorage.getItem(BOOT_BUSY_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as BusyOverlayOpts;
    if (parsed && typeof parsed.title === 'string') return parsed;
    return null;
  } catch {
    return null;
  }
}

export function BusyOverlayProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<BusyOverlayOpts | null>(readBootBusy);
  const bootTimer = useRef<number | undefined>(undefined);
  const mountedWithBoot = useRef(state !== null);
  const lastOpts = useRef<BusyOverlayOpts | null>(null);
  if (state) lastOpts.current = state;

  const show = useCallback((opts: BusyOverlayOpts) => setState(opts), []);
  const hide = useCallback(() => setState(null), []);

  useEffect(() => {
    try { localStorage.removeItem(BOOT_BUSY_KEY); } catch { /* ignore */ }
    document.getElementById('boot-splash')?.remove();
    if (!mountedWithBoot.current) return;
    bootTimer.current = window.setTimeout(() => setState(null), BOOT_HOLD_MS);
    return () => window.clearTimeout(bootTimer.current);
  }, []);

  const value = useMemo<BusyOverlayValue>(() => ({ show, hide }), [show, hide]);

  return (
    <BusyOverlayContext.Provider value={value}>
      {children}
      <BusyOverlay
        open={!!state}
        title={(state ?? lastOpts.current)?.title ?? ''}
        subtitle={(state ?? lastOpts.current)?.subtitle}
      />
    </BusyOverlayContext.Provider>
  );
}
