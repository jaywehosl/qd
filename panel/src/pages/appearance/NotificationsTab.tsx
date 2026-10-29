import { useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';

import { Button, Card, Input, Switch } from '@/components/ds';
import {
  subscribe,
  getSnapshot,
  clearHistory,
  restoreAlert,
  setAlertPref,
  setSensorEnabled,
  setSensorThreshold,
  setBackupReminderEnabled,
  setBackupReminderInterval,
  type AlertCategory,
  type SensorKey,
  type Severity,
} from '@/stores/notificationStore';
const SENSORS: { key: SensorKey; unit: string }[] = [
  { key: 'cpu', unit: '%' },
  { key: 'mem', unit: '%' },
  { key: 'disk', unit: '%' },
  { key: 'sockets', unit: '' },
  { key: 'udpSockets', unit: '' },
  { key: 'clientOffline', unit: 'h' },
];

const SEVERITY_ICON: Record<Severity, ReactNode> = {
  danger: <ExclamationCircleOutlined />,
  warning: <WarningOutlined />,
  info: <CheckCircleOutlined />,
};

const CATEGORIES: AlertCategory[] = ['security', 'nodes'];

function formatTime(ts: number): string {
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return '';
  }
}

export default function NotificationsTab() {
  const { t } = useTranslation();
  const { history, dismissed, prefs, sensors, maintenance } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  return (
    <div className="notif-tab">
      <Card title={t('pages.notifications.sources')}>
        <p className="notif-tab__lead">{t('pages.notifications.sourcesLead')}</p>
        {CATEGORIES.map((c) => (
          <div className="notif-tab__source" key={c}>
            <div className="notif-tab__source-label">
              <span>{t(`pages.notifications.category.${c}`)}</span>
              <span className="notif-tab__source-hint">{t(`pages.notifications.categoryHint.${c}`)}</span>
            </div>
            <Switch checked={prefs[c]} onChange={(v) => setAlertPref(c, v)} />
          </div>
        ))}
      </Card>

      <Card title={t('pages.notifications.sensors')}>
        <p className="notif-tab__lead">{t('pages.notifications.sensorsLead')}</p>
        {SENSORS.map((s) => {
          const cfg = sensors[s.key];
          return (
            <div className="notif-tab__source" key={s.key}>
              <div className="notif-tab__source-label">
                <span>{t(`pages.notifications.sensor.${s.key}`)}</span>
                <span className="notif-tab__source-hint">{t(`pages.notifications.sensorHint.${s.key}`)}</span>
              </div>
              <div className="notif-tab__sensor-ctl">
                <span className="notif-tab__sensor-thresh">
                  <Input
                    type="number"
                    value={String(cfg.threshold)}
                    disabled={!cfg.enabled}
                    onChange={(e) => setSensorThreshold(s.key, Number((e.target as HTMLInputElement).value))}
                  />
                  <code className="notif-tab__unit">{s.unit || ' '}</code>
                </span>
                <Switch checked={cfg.enabled} onChange={(v) => setSensorEnabled(s.key, v)} />
              </div>
            </div>
          );
        })}

      </Card>

      <Card title={t('pages.notifications.maintenance')}>
        <div className="notif-tab__source">
          <div className="notif-tab__source-label">
            <span>{t('pages.notifications.backupReminder')}</span>
            <span className="notif-tab__source-hint">{t('pages.notifications.backupReminderHint')}</span>
          </div>
          <div className="notif-tab__sensor-ctl">
            <span className="notif-tab__sensor-thresh">
              <Input
                type="number"
                value={String(maintenance.backupIntervalDays)}
                disabled={!maintenance.backupReminder}
                onChange={(e) => setBackupReminderInterval(Number((e.target as HTMLInputElement).value))}
              />
              <code className="notif-tab__unit">d</code>
            </span>
            <Switch checked={maintenance.backupReminder} onChange={setBackupReminderEnabled} />
          </div>
        </div>
      </Card>

      <Card
        title={t('pages.notifications.history')}
        extra={
          <Button
            size="sm"
            variant="text"
            danger
            disabled={!history.some((r) => !(r.source === 'alert' && r.key && dismissed.includes(r.key)))}
            onClick={clearHistory}
          >
            {t('clear')}
          </Button>
        }
      >
        {history.length === 0 ? (
          <div className="notif-tab__empty">{t('pages.notifications.empty')}</div>
        ) : (
          <ul className="notif-tab__log">
            {history.map((r) => (
              <li className={`notif-tab__item notif-tab__item--${r.severity}`} key={r.id}>
                <span className="notif-tab__item-icon">{SEVERITY_ICON[r.severity]}</span>
                <span className="notif-tab__item-text">{r.text}</span>
                <span className="notif-tab__item-meta">
                  <span className="notif-tab__item-time">{formatTime(r.ts)}</span>
                  {r.source === 'alert' && r.key && dismissed.includes(r.key) && (
                    <Button size="sm" variant="text" onClick={() => restoreAlert(r.key!)}>
                      {t('pages.notifications.restore')}
                    </Button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
