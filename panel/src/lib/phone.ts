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
const DESIGN_WIDTH = 407;
const UI_SCALE = 'qd-ui-scale';
const SCALE_EVENT = 'qd-scale';
let pageFit = 1;

export const pageScale = () => pageFit;
const COLS_SWAP = 'qd-cols-swap';
const upright = window.matchMedia('(orientation: portrait)');

export const navLow = () => localStorage.getItem(NAV_LOW) === 'true';
export const colsSwap = () => localStorage.getItem(COLS_SWAP) === 'true';

export const uiScale = () => {
  const held = Number(localStorage.getItem(UI_SCALE));
  return held >= 0.8 ? Math.min(held, 1.05) : 1;
};

export function setUiScale(scale: number, keep = true) {
  if (keep) localStorage.setItem(UI_SCALE, String(scale));
  window.dispatchEvent(new CustomEvent(SCALE_EVENT, { detail: scale }));
}

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
  const near = Math.min(screen.width, screen.height) / DESIGN_WIDTH;
  const device = Math.abs(near - 1) < 0.02 ? 1 : Math.min(1.3, Math.max(0.75, near));
  const viewport = document.querySelector('meta[name="viewport"]');
  const plain = viewport?.getAttribute('content') ?? '';
  const mark = (ui: number) => root.classList.toggle('is-big', ui > 1);
  mark(uiScale());
  let want = device * uiScale();
  const sizeViewport = () => {
    let content = plain;
    pageFit = 1;
    if (want !== 1) {
      const across = upright.matches ? Math.min(screen.width, screen.height) : Math.max(screen.width, screen.height);
      const width = Math.round(across / want);
      pageFit = across / width;
      content = `width=${width}, initial-scale=${pageFit}, minimum-scale=${pageFit}, maximum-scale=${pageFit}, user-scalable=no`;
    }
    if (viewport && viewport.getAttribute('content') !== content) viewport.setAttribute('content', content);
  };
  sizeViewport();
  const settle = () => root.classList.toggle('is-fit', root.getBoundingClientRect().height <= (window.visualViewport?.height ?? innerHeight));
  new ResizeObserver(settle).observe(root);
  window.visualViewport?.addEventListener('resize', settle);
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

  let below = -1;
  const seat = () => {
    const [top, bottom, left = 0, right = 0, bars = bottom] = (host.insets?.() ?? '0,0').split(',').map((v) => Number(v) / pageFit);
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
      const view = frame?.getBoundingClientRect() ?? { top, bottom: window.innerHeight - bottom };
      if (box.top >= view.top && box.bottom <= view.bottom) return;
      (frame ?? window).scrollBy({ top: box.top + box.height / 2 - (view.top + view.bottom) / 2, behavior: 'smooth' });
    }, 260);
  };
  seat();
  window.addEventListener(HOST_EVENT, seat);
  window.addEventListener('resize', () => {
    sizeViewport();
    seat();
  });
  window.addEventListener(SCALE_EVENT, (e) => {
    const ui = (e as CustomEvent<number>).detail ?? uiScale();
    mark(ui);
    want = device * ui;
    sizeViewport();
    seat();
  });

  (window as unknown as { qdBack: () => boolean }).qdBack = () => {
    if (!document.querySelector('[role="dialog"], [role="menu"], [role="listbox"]')) return false;
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    return true;
  };
}
