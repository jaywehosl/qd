const NBSP = ' ';

const SHORT = ['b/s', 'kb/s', 'Mb/s', 'Gb/s'];

export function shortBits(v: number): string {
  let n = Math.max(0, v) * 8;
  let unit = 0;
  while (unit < SHORT.length - 1 && Number(n.toPrecision(3)) >= 1000) {
    n /= 1000;
    unit += 1;
  }
  return `${unit === 0 && n < 1 ? 0 : Number(n.toPrecision(3))}${NBSP}${SHORT[unit]}`;
}

export function bitsPerSec(v: number): string {
  const bits = Math.max(0, v) * 8;
  if (bits >= 1e9) return `${Number((bits / 1e9).toFixed(1))}${NBSP}Gbit/s`;
  if (bits >= 1e6) return `${Number((bits / 1e6).toFixed(1))}${NBSP}Mbit/s`;
  if (bits >= 1e3) return `${(bits / 1e3).toFixed(0)}${NBSP}kbit/s`;
  return `${Math.round(bits)}${NBSP}bit/s`;
}
