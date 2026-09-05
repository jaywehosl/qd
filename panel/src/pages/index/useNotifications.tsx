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
  text: string;
}

export function useNotifications(): NotificationRow[] {
  const { t } = useTranslation();
  const { fetched: settingsFetched, restartNeeded } = useSettingsController();
  const xrayRestartNeeded = false;
  const { status, fetched: statusFetched } = useStatusQuery();

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


    if (prefs.restart && restartNeeded) {
      rows.push({ id: 'restart-panel', category: 'restart', severity: 'warning', text: t('pages.index.notifyRestartPanel') });
    }
    if (prefs.restart && xrayRestartNeeded) {
      rows.push({ id: 'restart-xray', category: 'restart', severity: 'warning', text: t('pages.index.notifyRestartXray') });
    }

    if (statusFetched) {
      for (const s of evalStatusSensors(status, sensors)) {
        if (s.over && !sensorAcked.includes(s.key)) {
          rows.push({ id: `sensor-${s.key}`, category: 'sensor', severity: 'warning', text: s.text });
        }
      }
    }

    return rows.filter((r) => !dismissed.includes(r.id));
  }, [t, settingsFetched, statusFetched, status, restartNeeded, xrayRestartNeeded, dismissed, prefs, sensors, sensorAcked]);
}
