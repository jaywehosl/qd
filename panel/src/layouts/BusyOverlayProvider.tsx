import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import BusyOverlay from '@/components/ui/BusyOverlay';
import {
  BOOT_BUSY_KEY,
  BusyOverlayContext,
  type BusyOverlayOpts,
  type BusyOverlayValue,
} from '@/layouts/busy-overlay-context';

// How long to keep the frost up on boot after a panel restart, so the app has
// a beat to render behind it rather than visibly assembling on the fly.
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

/**
 * Owns the single, app-wide BusyOverlay. Any controller can raise the frosted
 * "system is busy" takeover via useBusyOverlay() — it's portalled to <body> and
 * covers the WHOLE UI (header included), so panel- and core-restarts look
 * identical. Also replays a pending boot-busy request after a panel reload.
 */
export function BusyOverlayProvider({ children }: { children: ReactNode }) {
  // Seed from the persisted boot-busy request synchronously, so the React
  // overlay is present in the very first paint (no gap between the static
  // index.html splash and React taking over).
  const [state, setState] = useState<BusyOverlayOpts | null>(readBootBusy);
  const bootTimer = useRef<number | undefined>(undefined);
  // Whether we mounted because of a boot-busy request (captured once).
  const mountedWithBoot = useRef(state !== null);
  // Retain the last shown text so the exit animation fades a labelled card
  // instead of a blank one when state flips to null.
  const lastOpts = useRef<BusyOverlayOpts | null>(null);
  if (state) lastOpts.current = state;

  const show = useCallback((opts: BusyOverlayOpts) => setState(opts), []);
  const hide = useCallback(() => setState(null), []);

  // Boot pre-render hold: consume the one-shot request, drop the static
  // index.html splash now that React's overlay is painted, and keep the frost
  // up briefly while the fresh app finishes rendering.
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
