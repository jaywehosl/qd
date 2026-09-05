import type { DBInbound } from '@/models/dbinbound';

export interface EntryLike {
  protocol?: string;
  id?: number;
  remark?: string;
  listen?: string;
  port?: number;
  enable?: boolean;
  domain?: string;
}

export interface LinkParts {
  scheme: string;
  token: string;
  host: string;
  port: number;
  label: string;
}

export const LINK_SCHEME = 'qd';

function isLoopbackHost(host: string): boolean {
  return host === 'localhost' || host === '127.0.0.1' || host === '::1' || host.startsWith('192.168.');
}

export function preferPublicHost(browserHost: string, publicHost: string): string {
  return publicHost && isLoopbackHost(browserHost) ? publicHost : browserHost;
}

export interface GenEntryLinksInput {
  client?: unknown;
  inbound: EntryLike;
  remark?: string;
  remarkModel?: string;
  hostOverride?: string;
  fallbackHostname?: string;
  token?: string;
}

export function buildEntryLink(entry: EntryLike, token: string, label?: string): string {
  const host = entry.domain ?? entry.listen ?? window.location.hostname;
  const port = entry.port ?? 443;
  const name = label ?? entry.remark ?? host;
  return `${LINK_SCHEME}://${token}@${host}:${port}#${encodeURIComponent(name)}`;
}

export function genEntryLinks(input: GenEntryLinksInput): string {
  const { inbound, remark = '', hostOverride, fallbackHostname, token = '' } = input;

  const host = hostOverride || inbound.domain || fallbackHostname || window.location.hostname;
  const port = inbound.port ?? 443;
  const label = remark || inbound.remark || host;

  return `${LINK_SCHEME}://${token}@${host}:${port}#${encodeURIComponent(label)}`;
}


export function inboundFromDb(row: DBInbound | DbInboundLike): EntryLike {
  return {
    id: row.id,
    remark: row.remark,
    listen: row.listen,
    port: row.port,
    enable: row.enable,
  };
}

export type DbInboundLike = {
  id?: number;
  remark?: string;
  listen?: string;
  port?: number;
  enable?: boolean;
};

export function genAllLinks(input: GenEntryLinksInput & { client?: unknown }): { remark?: string; link: string }[] {
  return [{ remark: input.remark, link: genEntryLinks(input) }];
}

export function genWireguardLinks(_input?: unknown): string {
  return "";
}

export function genWireguardConfigs(_input?: unknown): string {
  return "";
}

export function isPostQuantumLink(_link?: string): boolean {
  return false;
}
