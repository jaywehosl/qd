import { z } from 'zod';

export const ClientNodeSchema = z.object({
  id: z.number(),
  name: z.string(),
  role: z.string().optional(),
  latencyMs: z.number().optional(),
  selected: z.boolean().optional(),
  reachable: z.boolean().optional(),
}).loose();

export const ClientNodeListSchema = z.array(ClientNodeSchema).nullable().transform((v) => v ?? []);

export const ClientStateSchema = z.object({
  imported: z.boolean(),
  admin: z.boolean(),
  connected: z.boolean(),
  node: ClientNodeSchema.nullable().optional(),
  nodes: z.object({
    total: z.number(),
    reachable: z.number(),
  }),
  egress: z.boolean(),
  adblock: z.boolean(),
  allowExit: z.boolean().optional(),
  subscription: z.object({
    lastRefresh: z.number().optional(),
    intervalMinutes: z.number().optional(),
    expiresAt: z.number().optional(),
  }),
}).loose();

export const RefreshResultSchema = z.object({
  changed: z.boolean().optional(),
  nodes: z.number().optional(),
  reconnected: z.boolean().optional(),
}).loose();

export type ClientState = z.infer<typeof ClientStateSchema>;
export type ClientNode = z.infer<typeof ClientNodeSchema>;
export type RefreshResult = z.infer<typeof RefreshResultSchema>;
