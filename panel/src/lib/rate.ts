const NBSP = ' ';

export function bitsPerSec(v: number): string {
  const bits = Math.max(0, v) * 8;
  if (bits >= 1e9) return `${(bits / 1e9).toFixed(1)}${NBSP}Gbit/s`;
  if (bits >= 1e6) return `${(bits / 1e6).toFixed(1)}${NBSP}Mbit/s`;
  if (bits >= 1e3) return `${(bits / 1e3).toFixed(0)}${NBSP}kbit/s`;
  return `${Math.round(bits)}${NBSP}bit/s`;
}
