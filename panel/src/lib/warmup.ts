type Load = () => Promise<unknown>;

const HEAVY: Load[] = [
  () => import('@/pages/index/SystemHistoryModal'),
  () => import('@/pages/client/ClientHistoryPanel'),
  () => import('@/pages/client/RoutingSection'),
  () => import('@/pages/index/BackupModal'),
  () => import('@/pages/clients/ClientFormModal'),
  () => import('@/pages/clients/ClientInfoModal'),
  () => import('@/pages/groups/GroupEditModal'),
  () => import('@/pages/inbounds/EntryFormModal'),
  () => import('@/pages/inbounds/info/InboundInfoModal'),
  () => import('@/pages/publish/PublishModal'),
];

function whenIdle(fn: () => void): void {
  const idle = window.requestIdleCallback;
  if (idle) idle(fn, { timeout: 2000 });
  else window.setTimeout(fn, 200);
}

export function warmModules(): void {
  let i = 0;
  const step = () => {
    if (i >= HEAVY.length) return;
    const load = HEAVY[i++];
    void load().catch(() => undefined).then(() => whenIdle(step));
  };
  whenIdle(step);
}
