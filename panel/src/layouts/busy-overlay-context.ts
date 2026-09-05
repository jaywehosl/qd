import { createContext, useContext } from 'react';

export interface BusyOverlayOpts {
  title: string;
  subtitle?: string;
}

export interface BusyOverlayValue {
  show: (opts: BusyOverlayOpts) => void;
  hide: () => void;
}

export const BOOT_BUSY_KEY = 'uup.bootBusy';

export const BusyOverlayContext = createContext<BusyOverlayValue | null>(null);

export function useBusyOverlay(): BusyOverlayValue {
  const ctx = useContext(BusyOverlayContext);
  if (!ctx) throw new Error('useBusyOverlay must be used within a BusyOverlayProvider');
  return ctx;
}
