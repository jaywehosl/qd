// Recharts splits a tick label on whitespace into separate tspans, which drops
// the space and stacks the unit under the number.
const NBSP = ' ';

/**
 * The counters are bytes per second but the number people quote is bits, so the
 * conversion belongs here rather than in a byte formatter wearing a wig.
 */
export function bitsPerSec(v: number): string {
  const bits = Math.max(0, v) * 8;
  if (bits >= 1e9) return `${(bits / 1e9).toFixed(1)}${NBSP}Gbit/s`;
  if (bits >= 1e6) return `${(bits / 1e6).toFixed(1)}${NBSP}Mbit/s`;
  if (bits >= 1e3) return `${(bits / 1e3).toFixed(0)}${NBSP}kbit/s`;
  return `${Math.round(bits)}${NBSP}bit/s`;
}
