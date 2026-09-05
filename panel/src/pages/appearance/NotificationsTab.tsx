import { useSyncExternalStore } from 'react';
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
  setUpdateCheckEnabled,
  setBackupReminderEnabled,
  setBackupReminderInterval,
  type AlertCategory,
  type SensorKey,
  type Severity,
} from '@/stores/notificationStore';
const SENSOR_LABELS: { key: SensorKey; label: string; hint: string; unit: string }[] = [
  { key: 'cpu', label: 'CPU usage', hint: 'Alert when CPU load exceeds the threshold', unit: '%' },
  { key: 'mem', label: 'Memory usage', hint: 'Alert when RAM usage exceeds the threshold', unit: '%' },
  { key: 'disk', label: 'Disk usage', hint: 'Alert when disk usage exceeds the threshold', unit: '%' },
  { key: 'sockets', label: 'Open TCP sockets', hint: 'Alert on an abnormal number of open TCP sockets', unit: '' },
  { key: 'udpSockets', label: 'Open UDP sockets', hint: 'Alert on an abnormal number of open UDP sockets', unit: '' },
  { key: 'uptimeDays', label: 'Uptime reminder', hint: 'Remind to check OS / panel updates after N days up', unit: 'd' },
  { key: 'clientOffline', label: 'Client offline', hint: 'Alert when a client that was online goes silent for N hours', unit: 'h' },
];

const SEVERITY_ICON: Record<Severity, ReactNode> = {
  danger: <ExclamationCircleOutlined />,
  warning: <WarningOutlined />,
  info: <CheckCircleOutlined />,
};

const CATEGORY_LABELS: { key: AlertCategory; label: string; hint: string }[] = [
  { key: 'security', label: 'Security warnings', hint: 'Default port / base path / HTTP exposure alerts' },
  { key: 'xray', label: 'Node reachability', hint: 'A node stopped answering the panel' },
  { key: 'restart', label: 'Restart reminders', hint: 'Panel / core restart pending after a save' },
];

function formatTime(ts: number): string {
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return '';
  }
}

export default function NotificationsTab() {
  const { history, dismissed, prefs, sensors, maintenance } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  return (
    <div className="notif-tab">
      <Card title="Alert sources">
        <p className="notif-tab__lead">
          Toggle which live alerts may appear in the header strip. Dismissed alerts
          and all pop-up toasts are kept in the history log below.
        </p>
        {CATEGORY_LABELS.map((c) => (
          <div className="notif-tab__source" key={c.key}>
            <div className="notif-tab__source-label">
              <span>{c.label}</span>
              <span className="notif-tab__source-hint">{c.hint}</span>
            </div>
            <Switch checked={prefs[c.key]} onChange={(v) => setAlertPref(c.key, v)} />
          </div>
        ))}
      </Card>

      <Card title="Threshold sensors">
        <p className="notif-tab__lead">
          Watch the live server status and notify when a value crosses its
          threshold. Each fires once per crossing and re-arms after it drops back.
        </p>
        {SENSOR_LABELS.map((s) => {
          const cfg = sensors[s.key];
          return (
            <div className="notif-tab__source" key={s.key}>
              <div className="notif-tab__source-label">
                <span>{s.label}</span>
                <span className="notif-tab__source-hint">{s.hint}</span>
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

      <Card title="Maintenance">
        <div className="notif-tab__source">
          <div className="notif-tab__source-label">
            <span>Panel update check</span>
            <span className="notif-tab__source-hint">Notify when a newer Community Panel release is published on GitHub</span>
          </div>
          <Switch checked={maintenance.updateCheck} onChange={setUpdateCheckEnabled} />
        </div>
        <div className="notif-tab__source">
          <div className="notif-tab__source-label">
            <span>Backup reminder</span>
            <span className="notif-tab__source-hint">Remind to export the database if none has been done in N days</span>
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
        title="History"
        extra={
          <Button
            size="sm"
            variant="text"
            danger
            disabled={!history.some((r) => !(r.source === 'alert' && r.key && dismissed.includes(r.key)))}
            onClick={clearHistory}
          >
            Clear
          </Button>
        }
      >
        {history.length === 0 ? (
          <div className="notif-tab__empty">No notifications yet.</div>
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
                      Restore
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
