import { useEffect, useRef } from 'react';

const CELL = 10;
const PACE = 5.6;
const STEP = 2.2;
const RAIL = 0.5;
const BAND = 0.09;
const DASH_Y = 0.8;
const TRACE_Y = 0.4;
const LINK_Y = 0.7;
const RAIL_H = 0.72;
const SAMPLES = 900;

interface FlowCanvasProps {
  lift: number;
  apart: number;
  fade: number;
  alive: boolean;
}

interface Walk {
  values: Float32Array;
  made: Float32Array;
  count: number;
  carried: number;
  wander: number;
}

function newWalk(): Walk {
  return {
    values: new Float32Array(SAMPLES),
    made: new Float32Array(SAMPLES),
    count: 0,
    carried: 0.5,
    wander: 0,
  };
}

function feed(walk: Walk, at: number, step: number) {
  walk.wander = walk.wander * 0.84 + (Math.random() - 0.5) * 0.22;
  const aim = 0.5 + walk.wander;
  walk.carried += (aim - walk.carried) * 0.34;
  walk.carried = Math.max(0.08, Math.min(0.92, walk.carried));

  walk.values.copyWithin(1, 0, SAMPLES - 1);
  walk.made.copyWithin(1, 0, SAMPLES - 1);
  walk.values[0] = walk.carried;
  walk.made[0] = at;
  if (walk.count < SAMPLES) walk.count += 1;
  void step;
}

function prime(walk: Walk, at: number, step: number) {
  let wander = 0;
  let held = 0.5;
  for (let i = SAMPLES - 1; i >= 0; i--) {
    wander = wander * 0.84 + (Math.random() - 0.5) * 0.22;
    held += (0.5 + wander - held) * 0.34;
    held = Math.max(0.08, Math.min(0.92, held));
    walk.values[i] = held;
    walk.made[i] = at - i * step;
  }
  walk.carried = held;
  walk.wander = wander;
  walk.count = SAMPLES;
}

export default function FlowCanvas({ lift, apart, fade, alive }: FlowCanvasProps) {
  const holder = useRef<HTMLCanvasElement>(null);
  const shape = useRef({ lift, apart, fade, alive });
  shape.current = { lift, apart, fade, alive };

  useEffect(() => {
    const canvas = holder.current;
    if (!canvas) return;
    const ink = canvas.getContext('2d');
    if (!ink) return;

    const main = newWalk();
    const link = newWalk();

    let clock = 0;
    let beat = 0;
    let slot = Number.NEGATIVE_INFINITY;
    let holeLo = Number.NaN;
    let holeHi = 0;
    let cutting = false;
    let primed = false;
    let frame = 0;

    const face = getComputedStyle(canvas);
    const good = face.getPropertyValue('--color-primary').trim() || '#71d888';
    const muted = face.getPropertyValue('--text-2').trim() || '#585858';

    function draw(now: number) {
      frame = requestAnimationFrame(draw);

      const box = canvas!.getBoundingClientRect();
      const ratio = window.devicePixelRatio || 1;
      const w = Math.round(box.width);
      const h = Math.round(box.height);
      if (w <= 0 || h <= 0) return;

      if (canvas!.width !== w * ratio || canvas!.height !== h * ratio) {
        canvas!.width = w * ratio;
        canvas!.height = h * ratio;
      }
      ink!.setTransform(ratio, 0, 0, ratio, 0, 0);
      ink!.clearRect(0, 0, w, h);

      const gap = beat === 0 ? 0 : now - beat;
      beat = now;
      if (gap > 0 && gap < 250) clock += (gap / 1000) * PACE;

      const { lift: rise, apart: split, fade: show, alive: live } = shape.current;

      const mid = w * RAIL;
      const half = mid * split * 0.5;
      const leftX = mid - half;
      const rightX = mid + half;
      const base = h * DASH_Y;
      const run = CELL * 0.48;
      const phase = clock % CELL;

      const cut = rise >= (1 - DASH_Y) / RAIL_H;
      if (cut && !cutting) holeLo = Number.NaN;
      cutting = cut;

      if (rise <= 0.001) {
        main.count = 0;
        link.count = 0;
        holeLo = Number.NaN;
        primed = false;
      }

      if (live) {
        const stride = STEP;
        const mark = Math.floor(clock / stride);
        if (slot === Number.NEGATIVE_INFINITY) slot = mark;
        let due = mark - slot;
        slot = mark;
        while (due > 0 && due <= 8) {
          feed(main, clock - (due - 1) * stride, stride);
          feed(link, clock - (due - 1) * stride, stride);
          due -= 1;
        }
      } else {
        slot = Number.NEGATIVE_INFINITY;
      }

      if (split > 0.001 && !primed) {
        prime(link, clock, STEP);
        primed = true;
      }

      if (cutting) {
        if (Number.isNaN(holeLo)) holeLo = leftX + clock;
        holeHi = leftX + clock;
      }

      ink!.strokeStyle = good;
      ink!.globalAlpha = 0.47;
      ink!.lineWidth = 1.6;
      ink!.lineCap = 'round';

      const lo = Number.isNaN(holeLo) ? 0 : holeLo - clock;
      const hi = Number.isNaN(holeLo) ? 0 : holeHi - clock;

      for (let a = w + CELL * 2; a > -CELL * 2; a -= CELL) {
        const from = a - phase;
        const to = from + run;

        if (to > rightX && from < w) {
          ink!.beginPath();
          ink!.moveTo(Math.max(from, rightX), base);
          ink!.lineTo(Math.min(to, w), base);
          ink!.stroke();
        }

        if (to > 0 && from < leftX) {
          const left = Math.max(from, 0);
          const right = Math.min(to, leftX);
          if (!Number.isNaN(holeLo) && right > lo && left < hi) {
            if (left < lo) {
              ink!.beginPath();
              ink!.moveTo(left, base);
              ink!.lineTo(lo, base);
              ink!.stroke();
            }
            if (right > hi) {
              ink!.beginPath();
              ink!.moveTo(hi, base);
              ink!.lineTo(right, base);
              ink!.stroke();
            }
          } else if (right - left > 1.6) {
            ink!.beginPath();
            ink!.moveTo(left, base);
            ink!.lineTo(right, base);
            ink!.stroke();
          }
        }
      }

      ink!.globalAlpha = 1;
      ink!.lineWidth = 1.8;
      ink!.lineJoin = 'round';

      const rim = 0.5;
      if (split > 0.01 && link.count > 0) {
        ink!.save();
        ink!.beginPath();
        ink!.rect(leftX + rim, 0, Math.max(0, rightX - leftX - rim * 2), h);
        ink!.clip();
        stroke(link, rightX, h * LINK_Y, leftX, 1);
        ink!.restore();
      }

      if (main.count > 0 && show > 0.004) {
        ink!.save();
        ink!.beginPath();
        ink!.rect(0, 0, Math.max(0, leftX - rim), h);
        ink!.clip();
        stroke(main, leftX, h * TRACE_Y, 0, show);
        ink!.restore();
      }

      if (rise > 0.02) {
        ink!.strokeStyle = muted;
        ink!.globalAlpha = 1;
        ink!.lineWidth = 1;
        const top = h * (1 - RAIL_H * rise);

        ink!.beginPath();
        ink!.moveTo(rightX, top);
        ink!.lineTo(rightX, h);
        ink!.stroke();

        if (split > 0.01) {
          ink!.beginPath();
          ink!.moveTo(leftX, top);
          ink!.lineTo(leftX, h);
          ink!.stroke();
        }
      }

      function stroke(walk: Walk, head: number, y: number, edge: number, alpha: number) {
        const band = h * BAND;
        const split2 = STEP * 2.5;
        ink!.strokeStyle = good;
        ink!.globalAlpha = alpha;
        ink!.beginPath();

        let began = false;
        let last = 0;
        if (shape.current.alive && walk.count > 0) {
          ink!.moveTo(head + rim, y - (walk.carried - 0.5) * band);
          began = true;
          last = head + rim;
        }

        for (let i = 0; i < walk.count; i++) {
          const px = head - (clock - walk.made[i]);
          if (px > head + rim) continue;
          if (px < edge) break;
          const py = y - (walk.values[i] - 0.5) * band;
          if (!began || last - px > split2) {
            ink!.moveTo(px, py);
            began = true;
          } else {
            ink!.lineTo(px, py);
          }
          last = px;
        }
        if (began) ink!.stroke();
        ink!.globalAlpha = 1;
      }
    }

    frame = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(frame);
  }, []);

  return <canvas ref={holder} className="cx-flow" />;
}
