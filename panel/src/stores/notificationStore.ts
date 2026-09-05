
export type Severity = 'danger' | 'warning' | 'info';
export type NotifSource = 'toast' | 'alert' | 'event';

export type AlertCategory = 'security' | 'xray' | 'restart';

export interface NotifRecord {
  id: string;
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

export type SensorKey = 'cpu' | 'mem' | 'disk' | 'sockets' | 'udpSockets' | 'uptimeDays' | 'clientOffline';
export interface SensorConfig { enabled: boolean; threshold: number }
export type SensorPrefs = Record<SensorKey, SensorConfig>;

export interface LogWatchPrefs { enabled: boolean; level: string }

export interface MaintenancePrefs {
  updateCheck: boolean;
  backupReminder: boolean;
  backupIntervalDays: number;
  lastBackupAt: number;
}

interface NotifState {
  history: NotifRecord[];
  active: NotifRecord[];
  dismissed: string[];
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

const DEFAULT_SENSORS: SensorPrefs = {
  cpu: { enabled: true, threshold: 55 },
  mem: { enabled: true, threshold: 60 },
  disk: { enabled: true, threshold: 30 },
  sockets: { enabled: true, threshold: 1000 },
  udpSockets: { enabled: true, threshold: 1000 },// open UDP sockets
  uptimeDays: { enabled: true, threshold: 7 },
  clientOffline: { enabled: true, threshold: 12 },
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

export function recordToast(severity: Severity, text: string): void {
  if (!text) return;
  pushHistory({ severity, text, source: 'toast' });
}

export function pushEvent(severity: Severity, text: string, dedupKey?: string): string {
  if (!text) return '';
  if (dedupKey && state.active.some((r) => r.key === dedupKey)) return '';
  const full: NotifRecord = { id: newId(), ts: Date.now(), key: dedupKey, severity, text, source: 'event' };
  const active = [full, ...state.active].slice(0, ACTIVE_CAP);
  const history = [full, ...state.history].slice(0, HISTORY_CAP);
  commit({ ...state, active, history });
  return full.id;
}

export function dismissEvent(id: string): void {
  if (!state.active.some((r) => r.id === id)) return;
  commit({ ...state, active: state.active.filter((r) => r.id !== id) });
}

export function ackSensor(key: string): void {
  if (state.sensorAcked.includes(key)) return;
  commit({ ...state, sensorAcked: [...state.sensorAcked, key] });
}
export function clearAckSensor(key: string): void {
  if (!state.sensorAcked.includes(key)) return;
  commit({ ...state, sensorAcked: state.sensorAcked.filter((k) => k !== key) });
}

export function dismissAlert(key: string, severity: Severity, text: string): void {
  const dismissed = state.dismissed.includes(key) ? state.dismissed : [...state.dismissed, key];
  const full: NotifRecord = { id: newId(), ts: Date.now(), key, severity, text, source: 'alert' };
  const history = [full, ...state.history].slice(0, HISTORY_CAP);
  commit({ ...state, dismissed, history });
}

export function restoreAlert(key: string): void {
  if (!state.dismissed.includes(key)) return;
  commit({ ...state, dismissed: state.dismissed.filter((k) => k !== key) });
}

export function isDismissed(key: string): boolean {
  return state.dismissed.includes(key);
}

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
export function markBackupDone(): void {
  const active = state.active.filter((r) => r.key !== 'backup-overdue');
  commit({ ...state, active, maintenance: { ...state.maintenance, lastBackupAt: Date.now() } });
}

export function dismissEventByKey(key: string): void {
  if (!state.active.some((r) => r.key === key)) return;
  commit({ ...state, active: state.active.filter((r) => r.key !== key) });
}
