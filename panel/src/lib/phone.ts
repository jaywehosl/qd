import { useSyncExternalStore } from 'react';

import { toast } from '@/components/ds/Toast';

export type HostCheck = { id: string; state: 'yes' | 'no' | 'unsure' };

type Host = {
  clipboard?: () => string;
  paint?: (colour: string) => void;
  insets?: () => string;
  ready?: () => void;
  pending?: () => string;
  dark?: () => boolean;
  checks?: () => string;
  open?: (id: string) => void;
  journal?: () => string;
  save?: (name: string, text: string) => string;
  saveData?: (name: string, base64: string) => string;
};

export const HOST_EVENT = 'qd-host';

export const phone = () => (window as unknown as { qdHost?: Host }).qdHost;

export const worded = () => ({ context: phone() ? 'phone' : undefined });

export function systemDark() {
  const host = phone();
  if (host?.dark) return host.dark();
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export function hostChecks(): HostCheck[] {
  try {
    return JSON.parse(phone()?.checks?.() ?? '[]') as HostCheck[];
  } catch {
    return [];
  }
}

export function saveOnPhone(name: string, blob: Blob): boolean {
  const host = phone();
  if (!host?.saveData) return false;
  void blob.arrayBuffer().then((raw) => {
    const bytes = new Uint8Array(raw);
    let text = '';
    for (let i = 0; i < bytes.length; i += 0x8000) text += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    const said = host.saveData?.(name, btoa(text)) ?? '';
    if (said.startsWith('Downloads/')) toast.success(said);
    else toast.error(said);
  });
  return true;
}

const NAV_LOW = 'qd-nav-low';
const COLS_SWAP = 'qd-cols-swap';
const upright = window.matchMedia('(orientation: portrait)');

export const navLow = () => localStorage.getItem(NAV_LOW) === 'true';
export const colsSwap = () => localStorage.getItem(COLS_SWAP) === 'true';

const placeNav = () => document.documentElement.classList.toggle('nav-low', navLow() && upright.matches);

export function setNavLow(on: boolean) {
  localStorage.setItem(NAV_LOW, String(on));
  placeNav();
}

export function setColsSwap(on: boolean) {
  localStorage.setItem(COLS_SWAP, String(on));
  document.documentElement.classList.toggle('cols-swap', on);
}

export function usePortrait() {
  return useSyncExternalStore(
    (tell) => {
      upright.addEventListener('change', tell);
      return () => upright.removeEventListener('change', tell);
    },
    () => upright.matches,
  );
}

export function mountPhone() {
  const host = phone();
  if (!host?.paint) return;

  const root = document.documentElement;
  root.classList.add('in-phone');
  root.classList.toggle('cols-swap', colsSwap());
  placeNav();
  let turned = 0;
  upright.addEventListener('change', () => {
    placeNav();
    root.classList.add('turning');
    clearTimeout(turned);
    turned = window.setTimeout(() => root.classList.remove('turning'), 400);
  });

  let painted = '';
  const tell = () => {
    const shade = getComputedStyle(root).backgroundColor;
    if (shade !== painted) host.paint?.((painted = shade));
  };
  new MutationObserver(tell).observe(root, { attributes: true, attributeFilter: ['class'] });
  tell();

  let below = '';
  const seat = () => {
    const [top, bottom, left = '0', right = '0', bars = bottom] = (host.insets?.() ?? '0,0').split(',');
    root.style.setProperty('--inset-top', `${top}px`);
    root.style.setProperty('--inset-bottom', `${bottom}px`);
    root.style.setProperty('--inset-bars', `${bars}px`);
    root.style.setProperty('--inset-left', `${left}px`);
    root.style.setProperty('--inset-right', `${right}px`);
    if (bottom === below) return;
    below = bottom;
    window.setTimeout(() => {
      const held = document.activeElement;
      if (!(held instanceof HTMLElement) || !held.matches('input, textarea')) return;
      const frame = held.closest<HTMLElement>('.ds-dialog__body');
      const box = held.getBoundingClientRect();
      const view = frame?.getBoundingClientRect() ?? { top: Number(top), bottom: window.innerHeight - Number(bottom) };
      if (box.top >= view.top && box.bottom <= view.bottom) return;
      (frame ?? window).scrollBy({ top: box.top + box.height / 2 - (view.top + view.bottom) / 2, behavior: 'smooth' });
    }, 260);
  };
  seat();
  window.addEventListener(HOST_EVENT, seat);
  window.addEventListener('resize', seat);

  (window as unknown as { qdBack: () => boolean }).qdBack = () => {
    if (!document.querySelector('[role="dialog"], [role="menu"], [role="listbox"]')) return false;
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    return true;
  };
}
