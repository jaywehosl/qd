/**
 * Notification subsystem store (framework-agnostic module store, same pattern as
 * the DS Toast store). It is the single home for:
 *
 *   • history  — every notification ever recorded (toasts + dismissed live
 *                alerts), newest first, capped, persisted to localStorage.
 *   • dismissed — keys of LIVE alerts the user has X-ed away; a dismissed live
 *                alert never reappears in the header strip (it lives in history).
 *   • prefs    — per-category enable flags for the live alerts (the Notifications
 *                tab in Appearance toggles these).
 *
 * Live alerts (port/path/xray/restart) are derived state computed in
 * useNotifications; this store only tracks which the user has silenced + logs
 * them into history when dismissed. Transient toasts are mirrored here via
 * recordToast() so the user has a scrollback log (the header only shows ACTIVE
 * live alerts; the full log lives in Appearance → Notifications).
 */

export type Severity = 'danger' | 'warning' | 'info';
export type NotifSource = 'toast' | 'alert' | 'event';

/** Live-alert categories that can be toggled on/off in the Notifications tab. */
export type AlertCategory = 'security' | 'xray' | 'restart';

export interface NotifRecord {
  id: string;
  /** the live-alert key (only for source==='alert') — ties a history row back
   *  to the dismissed set so it can be restored. */
  key?: string;
  severity: Severity;
  text: string;
  source: NotifSource;
  ts: number;
}

export interface AlertPrefs {
  security: boolean;
  xray: boolean;
  restart: boolean;
}

/** Threshold sensors evaluated against the polled server `status` (Phase 2).
 *  Edge-triggered: each fires once on the false→true crossing and re-arms when
 *  the value drops back below the threshold. */
export type SensorKey = 'cpu' | 'mem' | 'disk' | 'sockets' | 'udpSockets' | 'uptimeDays' | 'clientOffline';
export interface SensorConfig { enabled: boolean; threshold: number }
export type SensorPrefs = Record<SensorKey, SensorConfig>;

/** System log watcher: surface NEW panel log lines at/above `level` as
 *  notifications. Separate from numeric sensors because its "threshold" is a log
 *  level string. Levels match the log viewer: debug|info|notice|warning|err. */
export interface LogWatchPrefs { enabled: boolean; level: string }

/** Periodic maintenance reminders (checked by MaintenanceWatcher). */
export interface MaintenancePrefs {
  updateCheck: boolean;        // poll GitHub Releases for a newer panel build
  backupReminder: boolean;     // remind when no DB backup in `backupIntervalDays`
  backupIntervalDays: number;
  lastBackupAt: number;        // epoch ms of the last DB export (0 = clock unset)
}

interface NotifState {
  history: NotifRecord[];
  /** Event notifications currently shown in the status-bar strip + bell badge
   *  (sensors / log). Persisted so they survive a reload until dismissed. */
  active: NotifRecord[];
  dismissed: string[];
  /** Status-sensor keys the user has dismissed for the CURRENT over-threshold
   *  episode; auto-cleared when the value drops back (re-arms the live row). */
  sensorAcked: string[];
  prefs: AlertPrefs;
  sensors: SensorPrefs;
  logWatch: LogWatchPrefs;
  maintenance: MaintenancePrefs;
}

const HISTORY_KEY = 'uup.notifications.history';
const DISMISSED_KEY = 'uup.notifications.dismissed';
const PREFS_KEY = 'uup.notifications.prefs';
const SENSORS_KEY = 'uup.notifications.sensors';
const LOGWATCH_KEY = 'uup.notifications.logwatch';
const ACTIVE_KEY = 'uup.notifications.active';
const SENSOR_ACKED_KEY = 'uup.notifications.sensorAcked';
const MAINTENANCE_KEY = 'uup.notifications.maintenance';
const HISTORY_CAP = 200;
const ACTIVE_CAP = 50;

const DEFAULT_LOGWATCH: LogWatchPrefs = { enabled: false, level: 'warning' };
const DEFAULT_MAINTENANCE: MaintenancePrefs = {
  updateCheck: true,
  backupReminder: true,
  backupIntervalDays: 7,
  lastBackupAt: 0,
};

const DEFAULT_PREFS: AlertPrefs = { security: true, xray: true, restart: true };

// Community Panel default sensor profile (all live-condition sensors on).
const DEFAULT_SENSORS: SensorPrefs = {
  cpu: { enabled: true, threshold: 55 },         // % busy
  mem: { enabled: true, threshold: 60 },         // % used
  disk: { enabled: true, threshold: 30 },        // % used
  sockets: { enabled: true, threshold: 1000 },   // open TCP sockets
  udpSockets: { enabled: true, threshold: 1000 },// open UDP sockets
  uptimeDays: { enabled: true, threshold: 7 },   // system uptime in days
  clientOffline: { enabled: true, threshold: 12 }, // client silent for N hours
};

function loadHistory(): NotifRecord[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    const arr = raw ? (JSON.parse(raw) as unknown) : null;
    return Array.isArray(arr) ? (arr as NotifRecord[]) : [];
  } catch {
    return [];
  }
}
function loadDismissed(): string[] {
  try {
    const raw = localStorage.getItem(DISMISSED_KEY);
    const arr = raw ? (JSON.parse(raw) as unknown) : null;
    return Array.isArray(arr) ? (arr as string[]) : [];
  } catch {
    return [];
  }
}
function loadActive(): NotifRecord[] {
  try {
    const raw = localStorage.getItem(ACTIVE_KEY);
    const arr = raw ? (JSON.parse(raw) as unknown) : null;
    return Array.isArray(arr) ? (arr as NotifRecord[]) : [];
  } catch {
    return [];
  }
}
function loadStringArr(key: string): string[] {
  try {
    const raw = localStorage.getItem(key);
    const arr = raw ? (JSON.parse(raw) as unknown) : null;
    return Array.isArray(arr) ? (arr as string[]) : [];
  } catch {
    return [];
  }
}
function loadPrefs(): AlertPrefs {
  try {
    const raw = localStorage.getItem(PREFS_KEY);
    const obj = raw ? (JSON.parse(raw) as Partial<AlertPrefs>) : null;
    return obj ? { ...DEFAULT_PREFS, ...obj } : { ...DEFAULT_PREFS };
  } catch {
    return { ...DEFAULT_PREFS };
  }
}
function loadSensors(): SensorPrefs {
  try {
    const raw = localStorage.getItem(SENSORS_KEY);
    const obj = raw ? (JSON.parse(raw) as Partial<SensorPrefs>) : null;
    if (!obj) return structuredClone(DEFAULT_SENSORS);
    // Merge per-key so a new sensor added in a later build still gets a default.
    const merged = structuredClone(DEFAULT_SENSORS);
    (Object.keys(merged) as SensorKey[]).forEach((k) => {
      if (obj[k]) merged[k] = { ...merged[k], ...obj[k] };
    });
    return merged;
  } catch {
    return structuredClone(DEFAULT_SENSORS);
  }
}
function loadLogWatch(): LogWatchPrefs {
  try {
    const raw = localStorage.getItem(LOGWATCH_KEY);
    const obj = raw ? (JSON.parse(raw) as Partial<LogWatchPrefs>) : null;
    return obj ? { ...DEFAULT_LOGWATCH, ...obj } : { ...DEFAULT_LOGWATCH };
  } catch {
    return { ...DEFAULT_LOGWATCH };
  }
}
function loadMaintenance(): MaintenancePrefs {
  try {
    const raw = localStorage.getItem(MAINTENANCE_KEY);
    const obj = raw ? (JSON.parse(raw) as Partial<MaintenancePrefs>) : null;
    return obj ? { ...DEFAULT_MAINTENANCE, ...obj } : { ...DEFAULT_MAINTENANCE };
  } catch {
    return { ...DEFAULT_MAINTENANCE };
  }
}

let state: NotifState = {
  history: typeof localStorage !== 'undefined' ? loadHistory() : [],
  active: typeof localStorage !== 'undefined' ? loadActive() : [],
  dismissed: typeof localStorage !== 'undefined' ? loadDismissed() : [],
  sensorAcked: typeof localStorage !== 'undefined' ? loadStringArr(SENSOR_ACKED_KEY) : [],
  prefs: typeof localStorage !== 'undefined' ? loadPrefs() : { ...DEFAULT_PREFS },
  sensors: typeof localStorage !== 'undefined' ? loadSensors() : structuredClone(DEFAULT_SENSORS),
  logWatch: typeof localStorage !== 'undefined' ? loadLogWatch() : { ...DEFAULT_LOGWATCH },
  maintenance: typeof localStorage !== 'undefined' ? loadMaintenance() : { ...DEFAULT_MAINTENANCE },
};

const listeners = new Set<() => void>();
let seq = 0;

function emit() {
  listeners.forEach((l) => l());
}
function persist() {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(state.history));
    localStorage.setItem(ACTIVE_KEY, JSON.stringify(state.active));
    localStorage.setItem(DISMISSED_KEY, JSON.stringify(state.dismissed));
    localStorage.setItem(SENSOR_ACKED_KEY, JSON.stringify(state.sensorAcked));
    localStorage.setItem(PREFS_KEY, JSON.stringify(state.prefs));
    localStorage.setItem(SENSORS_KEY, JSON.stringify(state.sensors));
    localStorage.setItem(LOGWATCH_KEY, JSON.stringify(state.logWatch));
    localStorage.setItem(MAINTENANCE_KEY, JSON.stringify(state.maintenance));
  } catch {
    /* ignore quota / disabled storage */
  }
}
function commit(next: NotifState) {
  state = next;
  persist();
  emit();
}

export function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => { listeners.delete(l); };
}
export function getSnapshot(): NotifState {
  return state;
}

function newId(): string {
  return `n${Date.now().toString(36)}-${(++seq).toString(36)}`;
}

function pushHistory(rec: Omit<NotifRecord, 'id' | 'ts'> & { ts?: number }): void {
  const full: NotifRecord = { id: newId(), ts: rec.ts ?? Date.now(), ...rec };
  const history = [full, ...state.history].slice(0, HISTORY_CAP);
  commit({ ...state, history });
}

/** Mirror a transient toast into the history log. Only string content is logged
 *  (rich ReactNode toasts are skipped — they can't be replayed in a log). */
export function recordToast(severity: Severity, text: string): void {
  if (!text) return;
  pushHistory({ severity, text, source: 'toast' });
}

/** Emit an event notification (sensor / log): show it in the status-bar strip +
 *  bell badge (state.active) AND log it to history. `dedupKey` prevents a still-
 *  showing event from being added again on a re-fire (e.g. a sensor that stays
 *  over threshold across reloads) — a new one only appears after it's dismissed.
 *  Returns the record id (or '' if deduped). */
export function pushEvent(severity: Severity, text: string, dedupKey?: string): string {
  if (!text) return '';
  if (dedupKey && state.active.some((r) => r.key === dedupKey)) return '';
  const full: NotifRecord = { id: newId(), ts: Date.now(), key: dedupKey, severity, text, source: 'event' };
  const active = [full, ...state.active].slice(0, ACTIVE_CAP);
  const history = [full, ...state.history].slice(0, HISTORY_CAP);
  commit({ ...state, active, history });
  return full.id;
}

/** X an active event: remove it from the strip/bell (it stays in history). */
export function dismissEvent(id: string): void {
  if (!state.active.some((r) => r.id === id)) return;
  commit({ ...state, active: state.active.filter((r) => r.id !== id) });
}

/** Dismiss a LIVE status-sensor row for the current episode (hides it until the
 *  value drops back below the threshold, then it re-arms). Status sensors are
 *  live conditions, not logged events — so this never touches history. */
export function ackSensor(key: string): void {
  if (state.sensorAcked.includes(key)) return;
  commit({ ...state, sensorAcked: [...state.sensorAcked, key] });
}
/** Re-arm a dismissed sensor once its condition clears (called by SensorWatcher). */
export function clearAckSensor(key: string): void {
  if (!state.sensorAcked.includes(key)) return;
  commit({ ...state, sensorAcked: state.sensorAcked.filter((k) => k !== key) });
}

/** X a live alert: silence it forever (per key) and drop it into history. */
export function dismissAlert(key: string, severity: Severity, text: string): void {
  const dismissed = state.dismissed.includes(key) ? state.dismissed : [...state.dismissed, key];
  const full: NotifRecord = { id: newId(), ts: Date.now(), key, severity, text, source: 'alert' };
  const history = [full, ...state.history].slice(0, HISTORY_CAP);
  commit({ ...state, dismissed, history });
}

/** Un-silence a previously dismissed live alert (it will reappear if still live). */
export function restoreAlert(key: string): void {
  if (!state.dismissed.includes(key)) return;
  commit({ ...state, dismissed: state.dismissed.filter((k) => k !== key) });
}

export function isDismissed(key: string): boolean {
  return state.dismissed.includes(key);
}

/** Clear the log, but KEEP the entries that are the only handle for restoring a
 *  currently-dismissed live alert (source==='alert' whose key is still silenced)
 *  — otherwise clearing history would strand a dismissed alert with no way to
 *  bring it back to the status bar. Toasts and already-restored entries go. */
export function clearHistory(): void {
  const history = state.history.filter(
    (r) => r.source === 'alert' && !!r.key && state.dismissed.includes(r.key),
  );
  commit({ ...state, history });
}

export function setAlertPref(category: AlertCategory, enabled: boolean): void {
  commit({ ...state, prefs: { ...state.prefs, [category]: enabled } });
}

export function setSensorEnabled(key: SensorKey, enabled: boolean): void {
  commit({ ...state, sensors: { ...state.sensors, [key]: { ...state.sensors[key], enabled } } });
}
export function setSensorThreshold(key: SensorKey, threshold: number): void {
  if (!Number.isFinite(threshold)) return;
  commit({ ...state, sensors: { ...state.sensors, [key]: { ...state.sensors[key], threshold } } });
}

export function setLogWatchEnabled(enabled: boolean): void {
  commit({ ...state, logWatch: { ...state.logWatch, enabled } });
}
export function setLogWatchLevel(level: string): void {
  commit({ ...state, logWatch: { ...state.logWatch, level } });
}

export function setUpdateCheckEnabled(enabled: boolean): void {
  commit({ ...state, maintenance: { ...state.maintenance, updateCheck: enabled } });
}
export function setBackupReminderEnabled(enabled: boolean): void {
  commit({ ...state, maintenance: { ...state.maintenance, backupReminder: enabled } });
}
export function setBackupReminderInterval(days: number): void {
  if (!Number.isFinite(days) || days < 1) return;
  commit({ ...state, maintenance: { ...state.maintenance, backupIntervalDays: Math.round(days) } });
}
/** Record that a DB export just happened: resets the backup clock and clears any
 *  standing "backup overdue" reminder. Also used to start the clock on first run. */
export function markBackupDone(): void {
  const active = state.active.filter((r) => r.key !== 'backup-overdue');
  commit({ ...state, active, maintenance: { ...state.maintenance, lastBackupAt: Date.now() } });
}

/** Remove an active event by its dedupKey (used to clear a resolved condition). */
export function dismissEventByKey(key: string): void {
  if (!state.active.some((r) => r.key === key)) return;
  commit({ ...state, active: state.active.filter((r) => r.key !== key) });
}
