import i18next from 'i18next';

import { readLocalToken } from '@/api/localToken';
import { worded } from '@/lib/phone';
import { queryClient } from '@/queryClient';

const OVERLAY_ID = 'qd-lost';
const PROBE_EVERY = 1000;
const PROBE_WAIT = 2000;
const MISSES_SHOWN = 4;

let watching = false;
let misses = 0;
let timer = 0;
let reloadAfter = false;
let round = 0;

function say(key: string): string {
  return i18next.isInitialized ? i18next.t(key, worded()) : '';
}

function draw(kind: 'lost' | 'key'): void {
  if (typeof document === 'undefined' || document.getElementById(OVERLAY_ID)) return;

  const overlay = document.createElement('div');
  overlay.id = OVERLAY_ID;
  overlay.className = 'qd-lost';
  overlay.setAttribute('role', 'alert');

  const head = document.createElement('div');
  head.className = 'qd-lost__head';
  const dot = document.createElement('span');
  dot.className = kind === 'lost' ? 'qd-lost__dot is-live' : 'qd-lost__dot';
  const title = document.createElement('span');
  title.textContent = say(`client.lost.${kind}Title`) || (kind === 'lost' ? 'Reconnecting' : 'This window lost its key');
  head.append(dot, title);

  const text = document.createElement('p');
  text.className = 'qd-lost__text';
  text.textContent = say(`client.lost.${kind}Text`)
    || (kind === 'lost' ? 'The window lost touch with qd and keeps trying to reach it.' : 'Open qd again to get a fresh one.');

  const again = document.createElement('button');
  again.type = 'button';
  again.className = 'qd-lost__again';
  again.textContent = say('client.lost.again') || 'Try now';
  again.addEventListener('click', () => window.location.reload());

  overlay.append(head, text, again);
  document.body.append(overlay);
}

function hide(): void {
  document.getElementById(OVERLAY_ID)?.remove();
}

async function reachable(): Promise<boolean> {
  try {
    await fetch('/client/api/state', {
      headers: { 'X-QD-Token': readLocalToken() || '' },
      cache: 'no-store',
      signal: AbortSignal.timeout(PROBE_WAIT),
    });
    return true;
  } catch {
    return false;
  }
}

async function probe(): Promise<void> {
  window.clearTimeout(timer);
  const mine = ++round;
  const up = await reachable();
  if (mine !== round) return;
  if (up) {
    watching = false;
    misses = 0;
    if (reloadAfter) {
      window.location.reload();
      return;
    }
    hide();
    void queryClient.invalidateQueries();
    return;
  }
  misses++;
  if (misses >= MISSES_SHOWN) draw('lost');
  timer = window.setTimeout(() => void probe(), PROBE_EVERY);
}

export function clientUnreachable(reload = false): void {
  reloadAfter ||= reload;
  if (watching) return;
  watching = true;
  misses = 0;
  void probe();
}

export function showSessionGone(): void {
  hide();
  draw('key');
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.hidden || !watching) return;
    misses = 0;
    hide();
    void probe();
  });
}

export function looksLikeClientGone(error: unknown): boolean {
  if (!error) return false;
  const message = String((error as { message?: unknown })?.message ?? error);
  return (
    /failed to fetch dynamically imported module/i.test(message) ||
    /unable to preload css/i.test(message) ||
    /loading chunk .* failed/i.test(message) ||
    /importing a module script failed/i.test(message) ||
    /dynamically imported module/i.test(message)
  );
}
