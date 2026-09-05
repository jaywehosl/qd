import { z } from 'zod';

export const FlowSchema = z.enum([
  '',
  'xtls-rprx-vision',
  'xtls-rprx-vision-udp443',
]);
export type Flow = z.infer<typeof FlowSchema>;

export const TLS_FLOW_CONTROL = Object.freeze({
  VISION: 'xtls-rprx-vision',
  VISION_UDP443: 'xtls-rprx-vision-udp443',
}) satisfies Record<string, Exclude<Flow, ''>>;
