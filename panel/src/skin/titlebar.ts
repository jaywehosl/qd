type Bridge = {
  qdTitleBar?: (height: number, holes: number[][]) => void;
  qdWindowCommand?: (what: string) => void;
  qdWindowGrab?: (edge: string) => void;
};

const bridge = () => window as unknown as Bridge;

const EDGE = 6;
const CORNER = 14;

export function inWindow() {
  return typeof bridge().qdWindowGrab === 'function';
}

export function windowCommand(what: 'minimise' | 'maximise' | 'close') {
  void bridge().qdWindowCommand?.(what);
}

function grab(edge: string) {
  void bridge().qdWindowGrab?.(edge);
}

function tellBar() {
  const tell = bridge().qdTitleBar;
  const bar = document.querySelector('.header-container');
  if (!tell || !bar) return;
  void tell(Math.round(bar.getBoundingClientRect().bottom), []);
}

function dragArea() {
  document.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;

    const bar = document.querySelector('.header-container');
    if (!bar) return;

    const box = bar.getBoundingClientRect();
    if (e.clientY > box.bottom) return;
    if (e.clientY < EDGE) return;

    const target = e.target as HTMLElement | null;
    if (target?.closest('[data-drag="off"]')) return;
    if (target?.closest('button, a, input, select, textarea, label')) return;

    e.preventDefault();
    if (e.detail === 2) {
      windowCommand('maximise');
      return;
    }
    grab('caption');
  });

  document.addEventListener('dblclick', (e) => {
    const bar = document.querySelector('.header-container');
    if (!bar) return;
    if (e.clientY > bar.getBoundingClientRect().bottom) return;

    const target = e.target as HTMLElement | null;
    if (target?.closest('[data-drag="off"]')) return;

    windowCommand('maximise');
  });
}

function resizeEdges() {
  const skin = document.createElement('div');
  skin.className = 'qd-edges';

  const zones: Array<[string, string]> = [
    ['top', 'qd-edge--top'],
    ['bottom', 'qd-edge--bottom'],
    ['left', 'qd-edge--left'],
    ['right', 'qd-edge--right'],
    ['topleft', 'qd-edge--tl'],
    ['topright', 'qd-edge--tr'],
    ['bottomleft', 'qd-edge--bl'],
    ['bottomright', 'qd-edge--br'],
  ];

  for (const [edge, cls] of zones) {
    const strip = document.createElement('div');
    strip.className = `qd-edge ${cls}`;
    strip.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return;
      e.preventDefault();
      grab(edge);
    });
    skin.appendChild(strip);
  }

  document.body.appendChild(skin);
  document.documentElement.style.setProperty('--edge-grip', `${EDGE}px`);
  document.documentElement.style.setProperty('--edge-corner', `${CORNER}px`);

  new MutationObserver(() => {
    if (!skin.isConnected) document.body.appendChild(skin);
  }).observe(document.body, { childList: true });
}

export function watchTitleBar() {
  new MutationObserver(tabsGlide).observe(document.body, { childList: true, subtree: true });
  tabsGlide();

  if (!inWindow()) return;

  document.documentElement.classList.add('in-window');

  let pending = 0;
  const later = () => {
    window.clearTimeout(pending);
    pending = window.setTimeout(tellBar, 80);
  };

  later();
  window.addEventListener('resize', later, { passive: true });
  new MutationObserver(later).observe(document.body, { childList: true, subtree: true });

  dragArea();
  resizeEdges();
  new MutationObserver(trayGlide).observe(document.body, { childList: true, subtree: true });
  trayGlide();
  window.setTimeout(centreTabs, 300);
}

const GLIDE = '.nav-menu-item, .client-admin-btn, .sidebar-theme-cycle, .win-button';
const ONFILL = '.nav-menu-item, .client-admin-btn';
const TRAY = '.sidebar-theme-cycle, .win-button, .nav-menu-item, .client-admin-btn';

export function trayGlide() {
  const tray = document.querySelector<HTMLElement>('.header-container');
  if (!tray || tray.querySelector('.win-glow')) return;

  const glow = document.createElement('div');
  glow.className = 'win-glow';
  tray.prepend(glow);

  let live = false;

  const move = (btn: HTMLElement) => {
    const box = btn.getBoundingClientRect();
    const base = tray.getBoundingClientRect();

    const tight = btn.matches(TRAY);
    const padX = tight ? 3 : 0;
    const padY = tight ? 13 : 0;

    const place = () => {
      glow.style.transform =
        `translate(${box.left - base.left + padX}px, ${box.top - base.top + padY}px)`;
      glow.style.width = `${box.width - padX * 2}px`;
      glow.style.height = `${box.height - padY * 2}px`;
      glow.style.borderRadius = tight
        ? 'var(--radius-control)'
        : getComputedStyle(btn).borderRadius;
    };

    if (!live) {
      glow.style.transition = 'none';
      place();
      void glow.offsetWidth;
      glow.style.transition = '';
      live = true;
    } else {
      place();
    }

    glow.classList.toggle('is-danger', btn.classList.contains('win-button--close'));
    glow.classList.toggle('is-firm', btn.matches(ONFILL));
    glow.classList.add('is-live');
  };

  tray.addEventListener('pointerover', (e) => {
    const btn = (e.target as HTMLElement | null)?.closest<HTMLElement>(GLIDE);
    if (btn && tray.contains(btn)) move(btn);
  });

  tray.addEventListener('pointerleave', () => {
    glow.classList.remove('is-live');
    live = false;
  });
}

export function tabsGlide() {
  glideRow('.chp-tabs .ds-tabs__list', '.ds-tabs__trigger');
  glideRow('.chp-window', '.chp-win');
  glideRow('.vertical-tabs-container', '.vtab-btn');
  for (const row of document.querySelectorAll<HTMLElement>('.row-actions')) {
    glideIn(row, '.ds-btn');
  }
  for (const grid of document.querySelectorAll<HTMLElement>('.cal-grid:not(.cal-grid--head)')) {
    glideIn(grid, '.cal-day');
  }
}

function glideRow(hostSel: string, itemSel: string) {
  const list = document.querySelector<HTMLElement>(hostSel);
  if (!list) return;
  glideIn(list, itemSel);
}

function glideIn(list: HTMLElement, itemSel: string) {
  if (list.querySelector('.chp-glow')) return;

  const glow = document.createElement('div');
  glow.className = 'chp-glow';
  list.prepend(glow);

  let live = false;

  list.addEventListener('pointerover', (e) => {
    const tab = (e.target as HTMLElement | null)?.closest<HTMLElement>(itemSel);
    if (!tab || !list.contains(tab)) return;

    const box = tab.getBoundingClientRect();
    const base = list.getBoundingClientRect();
    const place = () => {
      glow.style.transform = `translate(${box.left - base.left}px, ${box.top - base.top}px)`;
      glow.style.width = `${box.width}px`;
      glow.style.height = `${box.height}px`;
    };

    if (!live) {
      glow.style.transition = 'none';
      place();
      void glow.offsetWidth;
      glow.style.transition = '';
      live = true;
    } else {
      place();
    }
    glow.classList.add('is-live');
  });

  list.addEventListener('pointerleave', () => {
    glow.classList.remove('is-live');
    live = false;
  });
}

export function centreTabs() {
  const bar = document.querySelector<HTMLElement>('.header-container');
  const nav = document.querySelector<HTMLElement>('.header-center');
  const tray = document.querySelector<HTMLElement>('.win-tray');
  if (!bar || !nav || !tray) return;

  const navWidth = nav.getBoundingClientRect().width;
  const trayWidth = tray.getBoundingClientRect().width;
  if (navWidth < 1 || trayWidth < 1) return;

  const pad = parseFloat(getComputedStyle(bar).paddingRight) || 0;
  const gap = window.innerWidth / 2 - pad - trayWidth - navWidth / 2;
  if (gap < 0) return;

  document.documentElement.style.setProperty('--tab-gap', `${Math.round(gap)}px`);
}
