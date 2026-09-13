import { useTranslation } from 'react-i18next';
import type { ReactNode } from 'react';
import type { TFunction } from 'i18next';
import {
  CheckCircleOutlined,
  CloseOutlined,
  ExclamationCircleOutlined,
  WarningOutlined,
} from '@ant-design/icons';

import { Button } from '@/components/ds';
import { useMetricsPanel } from '@/layouts/MetricsPanelContext';
import { useClientNotifications, type ClientNotif } from '@/layouts/useClientNotifications';
const SEVERITY_ICON: Record<ClientNotif['severity'], ReactNode> = {
  danger: <ExclamationCircleOutlined />,
  error: <ExclamationCircleOutlined />,
  warning: <WarningOutlined />,
  info: <CheckCircleOutlined />,
};

function ago(ts: number, t: TFunction): string {
  const s = Math.max(0, Math.floor((Date.now() - ts) / 1000));
  if (s < 60) return t('client.notify.justNow');
  if (s < 3600) return t('client.notify.minutesAgo', { n: Math.floor(s / 60) });
  if (s < 86400) return t('client.notify.hoursAgo', { n: Math.floor(s / 3600) });
  return new Date(ts).toLocaleDateString();
}

export default function ClientNotificationsBar() {
  const { t } = useTranslation();
  const { notifyOpen } = useMetricsPanel();
  const { items, dismiss, clear } = useClientNotifications();

  const dismissLabel = t('common.delete', { defaultValue: 'Dismiss' });

  return (
    <div className={`notif-bar is-client ${notifyOpen ? 'is-open' : ''}`} aria-hidden={!notifyOpen}>
      <div className="notif-container">
        {items.length === 0 ? (
          <div className="notif-empty">{t('client.notify.empty')}</div>
        ) : (
          <>
            <div className="notif-actions">
              <Button size="sm" loading={clear.isPending} onClick={() => clear.mutate()}>
                {t('client.notify.clear')}
              </Button>
            </div>
            <ul className="notif-list">
              {items.map((n) => (
                <li key={n.id} className={`notif-row notif-row--${n.severity}`}>
                  <span className="notif-row__icon">{SEVERITY_ICON[n.severity]}</span>
                  <span className="notif-row__text">{n.text}</span>
                  <span className="notif-row__ts">{ago(n.ts, t)}</span>
                  <button
                    type="button"
                    className="notif-row__dismiss"
                    aria-label={dismissLabel}
                    title={dismissLabel}
                    onClick={() => dismiss.mutate(n.id)}
                  >
                    <CloseOutlined />
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </div>
  );
}
