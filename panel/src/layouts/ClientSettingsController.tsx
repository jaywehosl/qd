import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { toast } from '@/components/ds';
import { HttpUtil } from '@/utils';

export interface ClientSettings {
  refreshMinutes: number;
  autostart: boolean;
  autostartBehaviour: string;
  manualBehaviour: string;
}

const SETTLE_MS = 600;

interface ClientSettingsContextValue {
  settings: ClientSettings | null;
  patch: (next: Partial<ClientSettings>) => void;
  dirty: boolean;
}

const ClientSettingsContext = createContext<ClientSettingsContextValue | null>(null);

export function ClientSettingsControllerProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<ClientSettings | null>(null);
  const [pending, setPending] = useState(false);

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const known = useRef<ClientSettings | null>(null);

  const { data: saved = null, refetch } = useQuery<ClientSettings | null>({
    queryKey: ['client', 'settings'],
    queryFn: async () => {
      const msg = await HttpUtil.get<ClientSettings>('/client/api/settings', undefined, { silent: true });
      return msg?.success ? (msg.obj ?? null) : null;
    },
    refetchInterval: 3000,
  });

  useEffect(() => {
    if (!saved) return;
    known.current = saved;
    setDraft((prev) => (prev && pending ? prev : saved));
  }, [saved, pending]);

  const push = useCallback(
    async (next: ClientSettings) => {
      const msg = await HttpUtil.post('/client/api/settings', next, {
        headers: { 'Content-Type': 'application/json' },
        silent: true,
      });
      if (!msg?.success) {
        toast.error(msg?.msg || t('somethingWentWrong'));
        if (known.current) setDraft(known.current);
        setPending(false);
        return;
      }
      setPending(false);
      await refetch();
    },
    [refetch, t],
  );

  const patch = useCallback(
    (next: Partial<ClientSettings>) => {
      setDraft((prev) => {
        if (!prev) return prev;
        const merged = { ...prev, ...next };

        setPending(true);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(() => {
          const minutes = Math.min(1440, Math.max(1, Math.round(merged.refreshMinutes) || 1));
          void push({ ...merged, refreshMinutes: minutes });
        }, SETTLE_MS);

        return merged;
      });
    },
    [push],
  );

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  const value = useMemo<ClientSettingsContextValue>(
    () => ({ settings: draft, patch, dirty: pending }),
    [draft, patch, pending],
  );

  return (
    <ClientSettingsContext.Provider value={value}>
      {children}
    </ClientSettingsContext.Provider>
  );
}

export function useClientSettings(): ClientSettingsContextValue {
  const ctx = useContext(ClientSettingsContext);
  if (!ctx) throw new Error('useClientSettings must be used within a ClientSettingsControllerProvider');
  return ctx;
}
