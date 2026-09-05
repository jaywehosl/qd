import { useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckCircleOutlined,
  CloseOutlined,
  ExclamationCircleOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';

import { useMetricsPanel } from '@/layouts/MetricsPanelContext';
import { useNotifications, type Severity } from '@/pages/index/useNotifications';
import { dismissAlert, dismissEvent, ackSensor, subscribe, getSnapshot } from '@/stores/notificationStore';
const SEVERITY_ICON: Record<Severity, ReactNode> = {
  danger: <ExclamationCircleOutlined />,
  warning: <WarningOutlined />,
  info: <CheckCircleOutlined />,
};

export default function NotificationsBar() {
  const { t } = useTranslation();
  const { notifyOpen } = useMetricsPanel();
  const rows = useNotifications();
  const { active } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  const total = rows.length + active.length;
  const dismissLabel = t('common.delete', { defaultValue: 'Dismiss' });

  return (
    <div className={`notif-bar ${notifyOpen ? 'is-open' : ''}`} aria-hidden={!notifyOpen}>
      <div className="notif-container">
        {total === 0 ? (
          <div className="notif-empty">
            {t('pages.index.notifyEmpty', { defaultValue: 'All clear — no notifications.' })}
          </div>
        ) : (
          <ul className="notif-list">
            {/* Live-condition alerts (port/path/xray/restart) — dismiss silences the key. */}
            {rows.map((r) => (
              <li key={r.id} className={`notif-row notif-row--${r.severity}`}>
                <span className="notif-row__icon">{SEVERITY_ICON[r.severity]}</span>
                <span className="notif-row__text">{r.text}</span>
                <button
                  type="button"
                  className="notif-row__dismiss"
                  aria-label={dismissLabel}
                  title={dismissLabel}
                  onClick={() => (r.category === 'sensor'
                    ? ackSensor(r.id.replace(/^sensor-/, ''))
                    : dismissAlert(r.id, r.severity, r.text))}
                >
                  <CloseOutlined />
                </button>
              </li>
            ))}
            {/* Event notifications (sensors / log) — dismiss removes from the strip. */}
            {active.map((r) => (
              <li key={r.id} className={`notif-row notif-row--${r.severity}`}>
                <span className="notif-row__icon">{SEVERITY_ICON[r.severity]}</span>
                <span className="notif-row__text">{r.text}</span>
                <button
                  type="button"
                  className="notif-row__dismiss"
                  aria-label={dismissLabel}
                  title={dismissLabel}
                  onClick={() => dismissEvent(r.id)}
                >
                  <CloseOutlined />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
