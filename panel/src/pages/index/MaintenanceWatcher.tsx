import { useEffect, useRef, useSyncExternalStore } from 'react';

import {
  subscribe,
  getSnapshot,
  pushEvent,
  markBackupDone,
} from '@/stores/notificationStore';

const TICK_MS = 3600_000;

export default function MaintenanceWatcher() {
  const { maintenance } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const m = maintenance;
  const ranBackupInit = useRef(false);

  useEffect(() => {
    function run() {
      if (m.backupReminder) {
        if (m.lastBackupAt === 0) {
          if (!ranBackupInit.current) { ranBackupInit.current = true; markBackupDone(); }
        } else {
          const overdueMs = m.backupIntervalDays * 86400_000;
          if (Date.now() - m.lastBackupAt >= overdueMs) {
            pushEvent('warning', `No database backup in ${m.backupIntervalDays}+ days — export one from the Backup dialog.`, 'backup-overdue');
          }
        }
      }
    }
    run();
    const id = window.setInterval(run, TICK_MS);
    return () => window.clearInterval(id);
  }, [m.backupReminder, m.backupIntervalDays, m.lastBackupAt]);

  return null;
}
