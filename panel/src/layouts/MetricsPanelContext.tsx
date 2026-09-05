import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

interface MetricsPanelValue {
  /** the telemetry status-bar (toggled by the brand logo) */
  open: boolean;
  setOpen: (v: boolean) => void;
  toggle: () => void;
  /** the notifications strip (toggled by the header bell) — a separate row that
      stacks BELOW the status-bar inside the shared .topbar-shell */
  notifyOpen: boolean;
  setNotifyOpen: (v: boolean) => void;
  toggleNotify: () => void;
}

const MetricsPanelContext = createContext<MetricsPanelValue | null>(null);

export function MetricsPanelProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [notifyOpen, setNotifyOpen] = useState(false);
  const value = useMemo<MetricsPanelValue>(
    () => ({
      open,
      setOpen,
      toggle: () => setOpen((o) => !o),
      notifyOpen,
      setNotifyOpen,
      toggleNotify: () => setNotifyOpen((o) => !o),
    }),
    [open, notifyOpen],
  );
  return <MetricsPanelContext.Provider value={value}>{children}</MetricsPanelContext.Provider>;
}

export function useMetricsPanel(): MetricsPanelValue {
  const ctx = useContext(MetricsPanelContext);
  if (!ctx) throw new Error('useMetricsPanel must be used within a MetricsPanelProvider');
  return ctx;
}
