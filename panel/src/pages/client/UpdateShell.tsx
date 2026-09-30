import { useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react';

interface UpdateShellProps {
  progress: number;
  busy: boolean;
  className?: string;
  children: ReactNode;
}

const STROKE = 5;
const BLUR = 3.5;

function outline(x: number, y: number, w: number, h: number, r: number): string {
  const mid = x + w / 2;
  return `M ${mid} ${y} H ${x + w - r} A ${r} ${r} 0 0 1 ${x + w} ${y + r} V ${y + h - r} `
    + `A ${r} ${r} 0 0 1 ${x + w - r} ${y + h} H ${x + r} A ${r} ${r} 0 0 1 ${x} ${y + h - r} `
    + `V ${y + r} A ${r} ${r} 0 0 1 ${x + r} ${y} H ${mid}`;
}

export default function UpdateShell({ progress, busy, className = '', children }: UpdateShellProps) {
  const shell = useRef<HTMLElement>(null);
  const glow = useId();
  const [box, setBox] = useState({ w: 0, h: 0, x: 0, y: 0, pw: 0, ph: 0, r: 0 });

  useLayoutEffect(() => {
    const el = shell.current;
    if (!el) return;
    const measure = () => {
      const pill = el.querySelector<HTMLElement>(':scope > .cx-power') ?? el;
      setBox({
        w: el.offsetWidth,
        h: el.offsetHeight,
        x: pill.offsetLeft,
        y: pill.offsetTop,
        pw: pill.offsetWidth,
        ph: pill.offsetHeight,
        r: parseFloat(getComputedStyle(pill).borderTopLeftRadius) || 0,
      });
    };
    measure();
    const watch = new ResizeObserver(measure);
    watch.observe(el);
    return () => watch.disconnect();
  }, []);

  const r = Math.max(0, Math.min(box.r, box.pw / 2, box.ph / 2));
  const perimeter = 2 * (box.pw + box.ph) - (8 - 2 * Math.PI) * r;
  const shown = busy && box.pw > 0;

  return (
    <section ref={shell} className={`cx-card cx-controls up-shell ${className}`}>
      {shown && (
        <svg className="up-ring" width={box.w} height={box.h} aria-hidden="true">
          <defs>
            <filter id={glow} x="-10%" y="-60%" width="120%" height="220%">
              <feGaussianBlur stdDeviation={BLUR} />
            </filter>
          </defs>
          <path
            d={outline(box.x, box.y, box.pw, box.ph, r)}
            filter={`url(#${glow})`}
            strokeWidth={STROKE}
            strokeDasharray={perimeter}
            strokeDashoffset={perimeter * (1 - progress)}
          />
        </svg>
      )}
      {children}
    </section>
  );
}
