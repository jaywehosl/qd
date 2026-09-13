import { coerceInboundJsonField } from '@/models/dbinbound';

export function isInboundMultiUser(record: { settings: unknown }): boolean {
  const settings = coerceInboundJsonField(record.settings) as { clients?: unknown[] };
  return Array.isArray(settings.clients) && settings.clients.length > 0;
}
