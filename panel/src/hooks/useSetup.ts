import { useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { expectRestart } from '@/lib/handover';

export interface SetupInfo {
  installed: boolean;
  offered: boolean;
  path: string;
  autostart: boolean;
}

const KEY = ['client', 'setup'];
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' }, silent: true } as const;

async function fetchSetup(): Promise<SetupInfo | null> {
  const msg = await HttpUtil.get<SetupInfo>('/client/api/setup', undefined, { silent: true });
  return msg?.success ? (msg.obj ?? null) : null;
}

export function useSetup() {
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery<SetupInfo | null>({
    queryKey: KEY,
    queryFn: fetchSetup,
    staleTime: Infinity,
    retry: false,
  });

  const install = useCallback(async (choice: { autostart: boolean; desktop: boolean }) => {
    expectRestart();
    const msg = await HttpUtil.post<SetupInfo>('/client/api/setup/install', choice, JSON_HEADERS);
    if (!msg?.success) return msg;
    for (let i = 0; i < 60; i++) {
      await new Promise((done) => window.setTimeout(done, 400));
      const now = await fetchSetup();
      if (now?.installed) {
        await queryClient.invalidateQueries({ queryKey: ['client'] });
        return msg;
      }
    }
    return { ...msg, success: false, msg: '' };
  }, [queryClient]);

  return { setup: data ?? null, loading: isLoading, install };
}
