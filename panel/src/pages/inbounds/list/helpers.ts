import { isSSMultiUser } from '@/lib/qd/entry-compat';
import { coerceInboundJsonField } from '@/models/dbinbound';

import type { StreamHints } from './types';

export function readStreamHints(streamSettings: unknown): StreamHints {
  const stream = coerceInboundJsonField(streamSettings) as { network?: string; security?: string };
  return {
    network: stream.network ?? '',
    isTls: stream.security === 'tls',
    isReality: stream.security === 'reality',
  };
}

export function networkLabel(network: string): string {
  const n = (network || '').toLowerCase();
  if (!n) return 'TCP';
  switch (n) {
    case 'httpupgrade': return 'HTTPUpgrade';
    case 'splithttp': return 'SplitHTTP';
    case 'xhttp': return 'XHTTP';
  }
  return n.toUpperCase();
}

export function networkL4(network: string): 'UDP' | '' {
  const n = (network || '').toLowerCase();
  if (n === 'kcp' || n === 'quic') return 'UDP';
  return '';
}

export function commaNetworkLabel(raw: string): string {
  const parts = (raw || 'tcp').toLowerCase().split(',').map((p) => p.trim()).filter(Boolean);
  if (parts.length === 0) return 'TCP';
  return parts.map(networkLabel).join(',');
}

export function shadowsocksNetworkLabel(settings: unknown): string {
  return commaNetworkLabel(readSettings(settings).network || '');
}

export function tunnelNetworkLabel(settings: unknown): string {
  return commaNetworkLabel(readSettings(settings).allowedNetwork || '');
}

export function mixedNetworkLabel(settings: unknown): string {
  const st = coerceInboundJsonField(settings) as { udp?: boolean };
  return st.udp ? 'TCP,UDP' : 'TCP';
}

export function readSettings(settings: unknown): { method?: string; network?: string; allowedNetwork?: string } {
  return coerceInboundJsonField(settings) as { method?: string; network?: string; allowedNetwork?: string };
}

export function isInboundMultiUser(record: { protocol: string; settings: unknown }): boolean {
  switch (record.protocol) {
    case 'vmess':
    case 'vless':
    case 'trojan':
    case 'hysteria':
      return true;
    case 'shadowsocks':
      return isSSMultiUser({ protocol: 'shadowsocks', settings: readSettings(record.settings) });
    default:
      return false;
  }
}
