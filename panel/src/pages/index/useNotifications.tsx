import { useMemo, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';

import { useSettingsController } from '@/layouts/settings-controller-context';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import {
  subscribe as notifSubscribe,
  getSnapshot as notifSnapshot,
  type AlertCategory,
  type Severity,
} from '@/stores/notificationStore';
import { evalStatusSensors } from '@/lib/notifications/statusSensors';

export type { Severity };

export interface NotificationRow {
  id: string;
  category: AlertCategory | 'sensor';
  severity: Severity;
  /** Plain-text form, used for the history log when the row is dismissed. */
  text: string;
}

/**
 * Single source of truth for the status-bar notification strip AND the header
 * bell badge. Aggregates the live alerts that used to live as in-page "tablets"
 * on the Settings / Xray pages, now that their state is global:
 *   • Xray-core health (crashed / stopped) — from the polled server status.
 *   • Security warnings ("panel may be exposed") — mirrors SettingsPage's
 *     confAlerts, computed from the global settings draft.
 *   • Restart reminders — panel- and core-restart pending after a save.
 */
export function useNotifications(): NotificationRow[] {
  const { t } = useTranslation();
  const { fetched: settingsFetched, restartNeeded } = useSettingsController();
  const xrayRestartNeeded = false;
  const { status, fetched: statusFetched } = useStatusQuery();

  // The dismissed set + per-category prefs + sensor config live in the store.
  const { dismissed, prefs, sensors, sensorAcked } = useSyncExternalStore(notifSubscribe, notifSnapshot, notifSnapshot);

  return useMemo(() => {
    const rows: NotificationRow[] = [];

    if (prefs.xray && statusFetched && status.nodes > 0) {
      const missing = status.nodes - status.nodesOnline;
      if (status.nodesOnline === 0) {
        rows.push({
          id: 'nodes-none',
          category: 'xray',
          severity: 'danger',
          text: t('pages.index.notifyNodesNone'),
        });
      } else if (missing > 0) {
        rows.push({
          id: 'nodes-partial',
          category: 'xray',
          severity: 'warning',
          text: t('pages.index.notifyNodesPartial', { missing, total: status.nodes }),
        });
      }
    }

    // 2) Security warnings — same checks as SettingsPage confAlerts. STABLE ids
    //    (per-check, not positional) so a dismissal sticks to the right alert.

    // 3) Restart reminders.
    if (prefs.restart && restartNeeded) {
      rows.push({ id: 'restart-panel', category: 'restart', severity: 'warning', text: t('pages.index.notifyRestartPanel') });
    }
    if (prefs.restart && xrayRestartNeeded) {
      rows.push({ id: 'restart-xray', category: 'restart', severity: 'warning', text: t('pages.index.notifyRestartXray') });
    }

    // 4) Status sensors (CPU/RAM/disk/sockets/uptime) — LIVE conditions: a row
    //    shows the current value while over threshold, updates every poll, and
    //    auto-clears when it drops back. Dismissed-for-this-episode rows are
    //    hidden (re-armed by SensorWatcher once the value drops). Never logged
    //    to history — they're conditions, not events.
    if (statusFetched) {
      for (const s of evalStatusSensors(status, sensors)) {
        if (s.over && !sensorAcked.includes(s.key)) {
          rows.push({ id: `sensor-${s.key}`, category: 'sensor', severity: 'warning', text: s.text });
        }
      }
    }

    // Drop anything the user has X-ed away (it lives in history now).
    return rows.filter((r) => !dismissed.includes(r.id));
  }, [t, settingsFetched, statusFetched, status, restartNeeded, xrayRestartNeeded, dismissed, prefs, sensors, sensorAcked]);
}
