import { useEffect, useRef, useSyncExternalStore } from 'react';

import {
  subscribe,
  getSnapshot,
  pushEvent,
  markBackupDone,
} from '@/stores/notificationStore';
import { isPanelUpdateAvailable } from '@/lib/panel-version';

const REPO = 'jaywehosl/qd';
const UPDATE_CHECK_MIN_INTERVAL = 6 * 3600_000; // don't poll GitHub more than ~4×/day
const TICK_MS = 3600_000;                        // re-evaluate hourly
const LAST_CHECK_KEY = 'uup.notifications.lastUpdateCheck';

function curVersion(): string {
  return (window as unknown as { X_UI_CUR_VER?: string }).X_UI_CUR_VER || '';
}

async function checkGithubRelease(): Promise<void> {
  let last = 0;
  try { last = Number(localStorage.getItem(LAST_CHECK_KEY)) || 0; } catch { /* ignore */ }
  if (Date.now() - last < UPDATE_CHECK_MIN_INTERVAL) return;
  try {
    const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
      headers: { Accept: 'application/vnd.github+json' },
    });
    // Record the attempt regardless, so a failure/rate-limit doesn't hammer.
    try { localStorage.setItem(LAST_CHECK_KEY, String(Date.now())); } catch { /* ignore */ }
    if (!res.ok) return;
    const data = (await res.json()) as { tag_name?: string };
    const tag = data?.tag_name?.trim();
    const cur = curVersion();
    if (tag && cur && isPanelUpdateAvailable(tag, cur)) {
      pushEvent('info', `Panel update available: ${tag} (you're on v${cur.replace(/^v/, '')})`, `update:${tag}`);
    }
  } catch {
    /* offline / CORS / rate-limited — silently skip until the next interval */
  }
}

/**
 * Headless maintenance reminders: a GitHub "newer release available" check
 * (cached so we poll at most ~4×/day) and a "no backup in N days" reminder.
 * Mounted once in PanelLayout.
 */
export default function MaintenanceWatcher() {
  const { maintenance } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const m = maintenance;
  const ranBackupInit = useRef(false);

  useEffect(() => {
    function run() {
      if (m.updateCheck) void checkGithubRelease();

      if (m.backupReminder) {
        if (m.lastBackupAt === 0) {
          // First time the reminder is active → start the clock instead of
          // nagging immediately (first reminder fires `intervalDays` from now).
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
  }, [m.updateCheck, m.backupReminder, m.backupIntervalDays, m.lastBackupAt]);

  return null;
}
