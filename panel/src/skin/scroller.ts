const HIDE_AFTER = 1400;
const MIN_THUMB = 10;
const THUMB_SHARE = 0.3;
const THUMB_INSET = 2;
const RAIL_SHARE = 0.2;
const RAIL_MIN = 64;

const AREAS = '.ds-dialog__body, .notif-list, .rt-picker__list, .log-body, .log-container, .ds-table-wrap';
const AREAS_X = '.deploy-script__text';

type Axis = 'y' | 'x';

interface Area {
  view: number;
  full: number;
  at: number;
  box: { top: number; right: number; bottom: number; left: number; height: number; width: number };
}

function readWindow(): Area {
  const doc = (document.scrollingElement ?? document.documentElement) as HTMLElement;
  return {
    view: window.innerHeight,
    full: doc.scrollHeight,
    at: doc.scrollTop,
    box: {
      top: 0, right: window.innerWidth, bottom: window.innerHeight, left: 0,
      height: window.innerHeight, width: window.innerWidth,
    },
  };
}

function readElement(el: HTMLElement, axis: Axis): Area {
  const box = el.getBoundingClientRect();
  const frame = axis === 'y'
    ? el.closest('.ds-dialog__content')?.getBoundingClientRect()
    : el.closest('.deploy-script')?.getBoundingClientRect();
  return {
    view: axis === 'y' ? el.clientHeight : el.clientWidth,
    full: axis === 'y' ? el.scrollHeight : el.scrollWidth,
    at: axis === 'y' ? el.scrollTop : el.scrollLeft,
    box: {
      top: box.top,
      right: axis === 'y' && frame ? frame.right : box.right,
      bottom: axis === 'x' && frame ? frame.bottom : box.bottom,
      left: box.left,
      height: box.height,
      width: box.width,
    },
  };
}

function scroller(target: HTMLElement | null, axis: Axis = 'y') {
  const read = () => (target ? readElement(target, axis) : readWindow());
  const scrollTo = (to: number) => {
    if (!target) { window.scrollTo({ top: to }); return; }
    if (axis === 'y') target.scrollTop = to;
    else target.scrollLeft = to;
  };

  const rail = document.createElement('div');
  rail.className = axis === 'y' ? 'qd-scroll' : 'qd-scroll qd-scroll--x';

  const track = document.createElement('div');
  track.className = 'qd-scroll__track';

  const thumb = document.createElement('div');
  thumb.className = 'qd-scroll__thumb';
  track.appendChild(thumb);
  rail.appendChild(track);
  document.body.appendChild(rail);

  rail.addEventListener('pointerdown', (e) => e.stopPropagation());

  let fade = 0;
  let dragging = false;
  let grabbedAt = 0;
  let grabbedTop = 0;

  function measure() {
    const area = read();
    if (area.full <= area.view + 1 || area.view < 1) return null;

    const along = axis === 'y' ? area.box.height : area.box.width;
    const railLength = Math.max(RAIL_MIN, along * RAIL_SHARE);
    const length = Math.max(MIN_THUMB, (area.view / area.full) * railLength * THUMB_SHARE);
    const travel = Math.max(0, railLength - length - THUMB_INSET * 2);
    const progress = area.at / (area.full - area.view);
    return { area, railLength, length, travel, top: THUMB_INSET + travel * progress };
  }

  function draw() {
    const at = measure();
    if (!at) {
      rail.classList.remove('is-live');
      return;
    }
    if (axis === 'y') {
      rail.style.height = `${at.railLength + 16}px`;
      rail.style.top = `${at.area.box.top + (at.area.box.height - at.railLength) / 2 - 8}px`;
      rail.style.right = `${Math.max(0, window.innerWidth - at.area.box.right)}px`;
      track.style.height = `${at.railLength}px`;
      thumb.style.height = `${at.length}px`;
      thumb.style.transform = `translateY(${at.top}px)`;
      return;
    }
    rail.style.width = `${at.railLength + 16}px`;
    rail.style.left = `${at.area.box.left + (at.area.box.width - at.railLength) / 2 - 8}px`;
    rail.style.top = `${at.area.box.bottom + 4}px`;
    track.style.width = `${at.railLength}px`;
    thumb.style.width = `${at.length}px`;
    thumb.style.transform = `translateX(${at.top}px)`;
  }

  function show() {
    if (!measure()) {
      rail.classList.remove('is-live');
      return;
    }
    draw();
    rail.classList.add('is-live');
    window.clearTimeout(fade);
    if (dragging || axis === 'x') return;
    fade = window.setTimeout(() => rail.classList.remove('is-live'), HIDE_AFTER);
  }

  const source: HTMLElement | Window = target ?? window;
  source.addEventListener('scroll', show, { passive: true });
  window.addEventListener('resize', show, { passive: true });

  thumb.addEventListener('pointerdown', (e) => {
    const at = measure();
    if (!at) return;
    dragging = true;
    grabbedAt = axis === 'y' ? e.clientY : e.clientX;
    grabbedTop = at.area.at;
    thumb.setPointerCapture(e.pointerId);
    rail.classList.add('is-held');
    e.preventDefault();
  });

  thumb.addEventListener('pointermove', (e) => {
    if (!dragging) return;
    const at = measure();
    if (!at) return;
    if (at.travel <= 0) return;
    const moved = ((axis === 'y' ? e.clientY : e.clientX) - grabbedAt) / at.travel;
    scrollTo(grabbedTop + moved * (at.area.full - at.area.view));
  });

  const release = (e: PointerEvent) => {
    if (!dragging) return;
    dragging = false;
    rail.classList.remove('is-held');
    try { thumb.releasePointerCapture(e.pointerId); } catch { /* уже отпущен */ }
    show();
  };

  thumb.addEventListener('pointerup', release);
  thumb.addEventListener('pointercancel', release);

  show();
  return { rail, draw, alive: () => !target || target.isConnected };
}

export function mountScroller() {
  if (document.getElementById('qd-scroll')) return;

  const page = scroller(null);
  page.rail.id = 'qd-scroll';

  const held = new Map<HTMLElement, ReturnType<typeof scroller>>();

  const sweep = () => {
    for (const el of document.querySelectorAll<HTMLElement>(AREAS)) {
      if (!held.has(el)) held.set(el, scroller(el, 'y'));
    }
    for (const el of document.querySelectorAll<HTMLElement>(AREAS_X)) {
      if (!held.has(el)) held.set(el, scroller(el, 'x'));
    }
    for (const [el, own] of held) {
      if (own.alive()) {
        own.draw();
        continue;
      }
      own.rail.remove();
      held.delete(el);
    }
    if (!page.rail.isConnected) document.body.appendChild(page.rail);
    page.draw();
  };

  new MutationObserver(sweep).observe(document.body, { childList: true, subtree: true });
  sweep();
}
