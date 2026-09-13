import type { DBInbound } from '@/models/dbinbound';

export interface EntryLike {
  id?: number;
  remark?: string;
  listen?: string;
  port?: number;
  enable?: boolean;
  domain?: string;
}

const LINK_SCHEME = 'qd';

function isLoopbackHost(host: string): boolean {
  return host === 'localhost' || host === '127.0.0.1' || host === '::1' || host.startsWith('192.168.');
}

export function preferPublicHost(browserHost: string, publicHost: string): string {
  return publicHost && isLoopbackHost(browserHost) ? publicHost : browserHost;
}

export interface GenEntryLinksInput {
  inbound: EntryLike;
  remark?: string;
  hostOverride?: string;
  fallbackHostname?: string;
  token?: string;
}

export function genEntryLinks(input: GenEntryLinksInput): string {
  const { inbound, remark = '', hostOverride, fallbackHostname, token = '' } = input;

  const host = hostOverride || inbound.domain || fallbackHostname || window.location.hostname;
  const port = inbound.port ?? 443;
  const label = remark || inbound.remark || host;

  return `${LINK_SCHEME}://${token}@${host}:${port}#${encodeURIComponent(label)}`;
}

export function inboundFromDb(row: DBInbound): EntryLike {
  return {
    id: row.id,
    remark: row.remark,
    listen: row.listen,
    port: row.port,
    enable: row.enable,
  };
}
