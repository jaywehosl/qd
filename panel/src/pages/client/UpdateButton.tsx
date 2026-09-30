import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { toast } from '@/components/ds';
import { progressOf, useUpdate, type UpdateInfo } from '@/hooks/useUpdate';
import UpdateShell from './UpdateShell';
import '@/skin/connect.css';

export function updateWord(t: (key: string, opts?: Record<string, unknown>) => string, info: UpdateInfo | null, idle: string): string {
  const progress = progressOf(info);
  if (info?.status === 'installing') return t('client.update.restarting');
  if (info?.status === 'fetching') {
    return progress === null ? t('client.update.downloading') : t('client.update.downloadingPct', { pct: Math.round(progress * 100) });
  }
  return idle;
}

export default function UpdateButton() {
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

  const idle = checking
    ? t('client.update.checking')
    : latest
      ? t('client.update.latest', { version: info?.version ?? '' })
      : info?.offer
        ? t('client.update.to', { version: info.offer.version })
        : t('client.update.check');

  return (
    <UpdateShell progress={progressOf(info) ?? 0} busy={checking || !!info?.status} className="up-head">
      <div className={`cx-power${info?.status ? ' is-on' : ''}`}>
        <button
          type="button"
          className="cx-power__main"
          title={info?.version}
          disabled={checking || !!info?.status}
          onClick={() => void click()}
        >
          {updateWord(t, info, idle)}
        </button>
      </div>
    </UpdateShell>
  );
}
