const DELAY = 350;
const GAP = 8;

export function mountTips(): void {
  if (typeof document === 'undefined') return;

  const tip = document.createElement('div');
  tip.className = 'ds-tooltip qd-tip';
  tip.setAttribute('role', 'tooltip');
  let owner: HTMLElement | null = null;
  let timer = 0;

  const hide = () => {
    window.clearTimeout(timer);
    tip.classList.remove('is-shown');
    owner = null;
  };

  const place = (el: HTMLElement) => {
    const box = el.getBoundingClientRect();
    const w = tip.offsetWidth;
    const h = tip.offsetHeight;
    const below = box.bottom + GAP + h <= window.innerHeight;
    const top = below ? box.bottom + GAP : box.top - GAP - h;
    const left = Math.min(Math.max(box.left + box.width / 2 - w / 2, GAP), window.innerWidth - w - GAP);
    tip.style.top = `${Math.max(top, GAP)}px`;
    tip.style.left = `${left}px`;
  };

  document.addEventListener('pointerover', (e) => {
    const el = (e.target as Element | null)?.closest?.<HTMLElement>('[title], [data-tip]') ?? null;
    if (el === owner) return;
    hide();
    if (!el) return;

    const native = el.getAttribute('title');
    if (native) {
      el.setAttribute('data-tip', native);
      el.removeAttribute('title');
      if (!el.getAttribute('aria-label') && !el.textContent?.trim()) el.setAttribute('aria-label', native);
    }
    const text = el.getAttribute('data-tip');
    if (!text) return;

    owner = el;
    timer = window.setTimeout(() => {
      if (!el.isConnected) return;
      if (!tip.isConnected) document.body.append(tip);
      tip.textContent = text;
      place(el);
      tip.classList.add('is-shown');
    }, DELAY);
  });

  document.addEventListener('pointerdown', hide, true);
  document.addEventListener('keydown', hide, true);
  document.addEventListener('scroll', hide, true);
  document.documentElement.addEventListener('pointerleave', hide);
  window.addEventListener('blur', hide);
}
