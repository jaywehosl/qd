import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SyncOutlined } from '@ant-design/icons';

import { toast } from '@/components/ds';
import { progressOf, useUpdate, type UpdateInfo } from '@/hooks/useUpdate';

export function updateWord(t: (key: string, opts?: Record<string, unknown>) => string, info: UpdateInfo | null, idle: string): string {
  const progress = progressOf(info);
  if (info?.status === 'installing') return t('client.update.restarting');
  if (info?.status === 'fetching') {
    return progress === null ? t('client.update.downloading') : t('client.update.downloadingPct', { pct: Math.round(progress * 100) });
  }
  return idle;
}

export function VersionPill({ version }: { version?: string }) {
  const { t } = useTranslation();
  const { info, run } = useUpdate();
  const [checking, setChecking] = useState(false);
  const [latest, setLatest] = useState(false);

  const click = async () => {
    if (checking || info?.status) return;
    setChecking(true);
    setLatest(false);
    const msg = await run();
    setChecking(false);
    if (!msg?.success) {
      toast.error(msg?.msg || t('client.update.failed'));
      return;
    }
    if (!msg.obj?.status) {
      setLatest(true);
      window.setTimeout(() => setLatest(false), 3000);
    }
  };

  const busy = checking || !!info?.status;
  const idle = checking
    ? t('client.update.checking')
    : latest
      ? t('client.update.latest', { version: info?.version ?? version ?? '' })
      : info?.offer
        ? t('client.update.to', { version: info.offer.version })
        : `qd ${version ?? info?.version ?? ''}`;

  return (
    <button
      type="button"
      className={`cset-version${info?.offer && !latest ? ' is-offer' : ''}`}
      title={t('client.update.check')}
      disabled={busy}
      onClick={() => void click()}
    >
      <SyncOutlined spin={busy} />
      <span>{updateWord(t, info, idle)}</span>
    </button>
  );
}
