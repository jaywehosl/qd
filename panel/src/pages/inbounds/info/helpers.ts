import { getMessage } from '@/utils/messageBus';
import { ColorUtils, ClipboardManager } from '@/utils';
import { coerceInboundJsonField } from '@/models/dbinbound';

import type { ClientSetting, ClientStats, DBInboundLike, InboundInfo } from './types';

export function buildInboundInfo(dbInbound: DBInboundLike): InboundInfo {
  const settings = coerceInboundJsonField(dbInbound.settings);
  const clients = Array.isArray(settings.clients) ? (settings.clients as ClientSetting[]) : [];
  return { protocol: dbInbound.protocol, clients, settings };
}

export function copyText(value: unknown, t: (k: string) => string) {
  ClipboardManager.copyText(String(value ?? '')).then((ok) => {
    if (ok) getMessage().success(t('copied'));
  });
}

export function statsColor(stats: ClientStats, trafficDiff: number) {
  return ColorUtils.usageColor(stats.up + stats.down, trafficDiff, stats.total);
}
