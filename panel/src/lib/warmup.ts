type Load = () => Promise<unknown>;

const CLIENT: Load[] = [
  () => import('@/pages/client/ClientPage'),
  () => import('@/pages/client/RoutingPage'),
  () => import('@/pages/client/ClientSettingsPage'),
  () => import('@/pages/client/RoutingSection'),
  () => import('@/pages/client/ClientHistoryPanel'),
];

const ADMIN: Load[] = [
  () => import('@/pages/index/ClientsSection'),
  () => import('@/pages/index/GroupsSection'),
  () => import('@/pages/index/NodesSection'),
  () => import('@/pages/settings/SettingsPage'),
  () => import('@/pages/clients/ClientInfoModal'),
  () => import('@/pages/clients/ClientFormModal'),
  () => import('@/pages/groups/GroupEditModal'),
  () => import('@/pages/index/SystemHistoryModal'),
  () => import('@/pages/index/BackupModal'),
];

const queue: Load[] = [];
const queued = new Set<Load[]>();
let running = false;

function whenIdle(fn: () => void): void {
  const idle = window.requestIdleCallback;
  if (idle) idle(fn, { timeout: 2000 });
  else window.setTimeout(fn, 200);
}

function step(): void {
  const load = queue.shift();
  if (!load) {
    running = false;
    return;
  }
  void load().catch(() => undefined).then(() => whenIdle(step));
}

export function warmModules(part: 'client' | 'admin'): void {
  const list = part === 'admin' ? ADMIN : CLIENT;
  if (queued.has(list)) return;
  queued.add(list);
  queue.push(...list);
  if (running) return;
  running = true;
  whenIdle(step);
}
