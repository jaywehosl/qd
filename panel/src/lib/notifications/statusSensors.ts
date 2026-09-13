import type { Status } from '@/models/status';
import type { SensorKey, SensorPrefs } from '@/stores/notificationStore';

export interface StatusSensorEval {
  key: SensorKey;
  over: boolean;
  value: number;
  text: string;
}

const pct = (cur: number, total: number) => (total > 0 ? Math.round((cur / total) * 100) : 0);

export function evalStatusSensors(status: Status, sensors: SensorPrefs): StatusSensorEval[] {
  const defs: { key: SensorKey; value: number; ready: boolean; unit: string; label: string }[] = [
    { key: 'cpu', value: Math.round(status.cpu.current), ready: true, unit: '%', label: 'CPU usage' },
    { key: 'mem', value: pct(status.mem.current, status.mem.total), ready: status.mem.total > 0, unit: '%', label: 'Memory usage' },
    { key: 'disk', value: pct(status.disk.current, status.disk.total), ready: status.disk.total > 0, unit: '%', label: 'Disk usage' },
    { key: 'sockets', value: status.tcpCount, ready: true, unit: '', label: 'TCP sockets' },
    { key: 'udpSockets', value: status.udpCount, ready: true, unit: '', label: 'UDP sockets' },
    { key: 'uptimeDays', value: Math.floor(status.uptime / 86400), ready: status.uptime > 0, unit: 'd', label: 'System uptime' },
  ];

  const out: StatusSensorEval[] = [];
  for (const d of defs) {
    const cfg = sensors[d.key];
    if (!cfg?.enabled || !d.ready) continue;
    const th = cfg.threshold;
    out.push({
      key: d.key,
      over: d.value >= th,
      value: d.value,
      text: `${d.label} ${d.value}${d.unit} (threshold ${th}${d.unit})`,
    });
  }
  return out;
}
