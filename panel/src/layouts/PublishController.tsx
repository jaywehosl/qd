import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { useRegisterEditor } from '@/layouts/header-actions-context';
import { LazyMount } from '@/components/utility';
import { lazy } from 'react';

const PublishModal = lazy(() => import('@/pages/publish/PublishModal'));

export interface DraftChange {
  kind: string;
  name: string;
  action: string;
}

export interface DraftState {
  revision: number;
  publishedRevision: number;
  changes: DraftChange[];
}

interface PublishContextValue {
  draft: DraftState | null;
  dirty: boolean;
  refreshDraft: () => void;
}

const PublishContext = createContext<PublishContextValue | null>(null);

export function PublishControllerProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const { data: draft = null, refetch } = useQuery<DraftState | null>({
    queryKey: ['publish', 'draft'],
    queryFn: async () => {
      const msg = await HttpUtil.get<DraftState>('/panel/api/publish/draft', undefined, { silent: true });
      if (!msg?.success || !msg.obj) return null;
      return { ...msg.obj, changes: msg.obj.changes ?? [] };
    },
    refetchInterval: 5000,
  });

  const dirty = !!draft && draft.changes.length > 0;

  const refreshDraft = useCallback(() => { void refetch(); }, [refetch]);

  const discard = useCallback(async () => {
    setBusy(true);
    try {
      await HttpUtil.post('/panel/api/publish/discard');
      await queryClient.invalidateQueries();
    } finally {
      setBusy(false);
    }
  }, [queryClient]);

  useRegisterEditor({
    id: 'publish',
    dirty,
    restartNeeded: false,
    busy,
    saveLabel: t('publish.publish'),
    restartLabel: '',
    restartKind: 'panel',
    discardLabel: t('publish.discard'),
    save: () => setOpen(true),
    restart: () => {},
    discard,
  });

  const value = useMemo<PublishContextValue>(
    () => ({ draft, dirty, refreshDraft }),
    [draft, dirty, refreshDraft],
  );

  return (
    <PublishContext.Provider value={value}>
      {children}
      <LazyMount when={open}>
        <PublishModal
          open={open}
          draft={draft}
          onClose={() => { setOpen(false); refreshDraft(); }}
        />
      </LazyMount>
    </PublishContext.Provider>
  );
}

export function usePublish(): PublishContextValue {
  const ctx = useContext(PublishContext);
  if (!ctx) throw new Error('usePublish must be used within a PublishControllerProvider');
  return ctx;
}
