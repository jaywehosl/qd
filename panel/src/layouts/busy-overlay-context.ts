import { createContext, useContext } from 'react';

export interface BusyOverlayOpts {
  title: string;
  subtitle?: string;
}

export interface BusyOverlayValue {
  show: (opts: BusyOverlayOpts) => void;
  hide: () => void;
}

// localStorage key: a panel restart reloads the whole frontend, so the busy
// takeover can't survive in-memory. We persist a "show the overlay on the next
// boot" request (the already-translated title/subtitle) so the freshly-loaded
// app keeps the frost up while it pre-renders, instead of visibly building the
// UI on the fly.
export const BOOT_BUSY_KEY = 'uup.bootBusy';

// Context kept in a component-free module so its identity is stable across HMR.
export const BusyOverlayContext = createContext<BusyOverlayValue | null>(null);

export function useBusyOverlay(): BusyOverlayValue {
  const ctx = useContext(BusyOverlayContext);
  if (!ctx) throw new Error('useBusyOverlay must be used within a BusyOverlayProvider');
  return ctx;
}
