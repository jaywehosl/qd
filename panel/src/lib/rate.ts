import { unit } from '@/lib/unit';
const NBSP = ' ';

const SHORT = ['b/s', 'kb/s', 'Mb/s', 'Gb/s'];

export function shortBits(v: number): string {
  let n = Math.max(0, v) * 8;
  let step = 0;
  while (step < SHORT.length - 1 && Number(n.toPrecision(3)) >= 1000) {
    n /= 1000;
    step += 1;
  }
  return `${step === 0 && n < 1 ? 0 : Number(n.toPrecision(3))}${NBSP}${unit(SHORT[step])}`;
}

export function bitsPerSec(v: number): string {
  const bits = Math.max(0, v) * 8;
  if (bits >= 1e9) return `${Number((bits / 1e9).toFixed(1))}${NBSP}${unit('Gbit/s')}`;
  if (bits >= 1e6) return `${Number((bits / 1e6).toFixed(1))}${NBSP}${unit('Mbit/s')}`;
  if (bits >= 1e3) return `${(bits / 1e3).toFixed(0)}${NBSP}${unit('kbit/s')}`;
  return `${Math.round(bits)}${NBSP}${unit('bit/s')}`;
}
