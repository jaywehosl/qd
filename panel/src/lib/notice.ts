import type { TFunction } from 'i18next';

const said: [RegExp, string, (m: RegExpMatchArray) => Record<string, string>][] = [
  [/^Couldn't reach any server to update the subscription\./, 'unreachable', () => ({})],
  [/^Subscription added: (\d+) of (\d+) servers reachable\.$/, 'added', (m) => ({ n: m[1], total: m[2] })],
  [/^Subscription updated: (\d+) of (\d+) servers reachable\.$/, 'updated', (m) => ({ n: m[1], total: m[2] })],
  [/^The list of servers changed\. (\d+) available now\.$/, 'serversChanged', (m) => ({ n: m[1] })],
  [/^Your subscription no longer includes the exit server\.$/, 'exitGone', () => ({})],
  [/^There's no server to connect to on this network\.$/, 'noServer', () => ({})],
  [/^Couldn't connect: (.*)$/s, 'connectFailed', (m) => ({ why: m[1] })],
  [/^Connected via (.+)\.$/, 'connected', (m) => ({ node: m[1] })],
  [/^Disconnected\.$/, 'disconnected', () => ({})],
  [/^The exit server isn't included in your subscription\.$/, 'exitRefused', () => ({})],
  [/^This subscription is no longer valid\./, 'invalid', () => ({})],
  [/disabled by the administrator/i, 'disabled', () => ({})],
  [/^This subscription has expired\.$/, 'expired', () => ({})],
  [/too old for the network: update qd to (.+?)\.?$/i, 'tooOld', (m) => ({ version: m[1] })],
  [/blocked by the administrator/i, 'blocked', () => ({})],
  [/allowance of devices/i, 'devices', () => ({})],
  [/^The update to (\S+) failed: (.*)$/s, 'updateFailed', (m) => ({ version: m[1], why: m[2] })],
  [/^The update didn't start, so the previous version is back\.$/, 'rolledBack', () => ({})],
];

export function noticeText(text: string, t: TFunction): string {
  for (const [pattern, key, args] of said) {
    const m = text.match(pattern);
    if (m) return t(`client.notice.${key}`, args(m));
  }
  return text;
}
