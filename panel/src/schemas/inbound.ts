import { z } from 'zod';

const SlimInboundSchema = z.object({
  id: z.number(),
  protocol: z.string(),
}).loose();

export const SlimInboundListSchema = z.array(SlimInboundSchema);

export const InboundDetailSchema = z.object({
  id: z.number(),
  protocol: z.string(),
}).loose();

export const LastOnlineMapSchema = z.record(z.string(), z.number());

export type InboundDetail = z.infer<typeof InboundDetailSchema>;
export type LastOnlineMap = z.infer<typeof LastOnlineMapSchema>;
