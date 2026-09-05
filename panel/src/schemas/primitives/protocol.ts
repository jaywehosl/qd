import { z } from 'zod';

export const ProtocolSchema = z.enum([
  'vmess',
  'vless',
  'trojan',
  'shadowsocks',
  'wireguard',
  'hysteria',
  'http',
  'mixed',
  'tunnel',
  'tun',
]);
export type Protocol = z.infer<typeof ProtocolSchema>;

export const Protocols = Object.freeze({
  VMESS: 'vmess',
  VLESS: 'vless',
  TROJAN: 'trojan',
  SHADOWSOCKS: 'shadowsocks',
  WIREGUARD: 'wireguard',
  HYSTERIA: 'hysteria',
  HTTP: 'http',
  MIXED: 'mixed',
  TUNNEL: 'tunnel',
  TUN: 'tun',
});
