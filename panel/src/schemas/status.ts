import { z } from 'zod';

const CurTotalInputSchema = z.object({
  current: z.number().optional(),
  total: z.number().optional(),
});

const NetIOSchema = z.object({
  up: z.number(),
  down: z.number(),
});

const NetTrafficSchema = z.object({
  sent: z.number(),
  recv: z.number(),
});

const PublicIPSchema = z.object({
  ipv4: z.union([z.string(), z.number()]),
  ipv6: z.union([z.string(), z.number()]),
});

const AppStatsSchema = z.object({
  threads: z.number(),
  mem: z.number(),
  uptime: z.number(),
});

export const StatusSchema = z.object({
  cpu: z.number().optional(),
  cpuCores: z.number().optional(),
  logicalPro: z.number().optional(),
  cpuSpeedMhz: z.number().optional(),
  disk: CurTotalInputSchema.optional(),
  loads: z.array(z.number()).optional(),
  mem: CurTotalInputSchema.optional(),
  netIO: NetIOSchema.optional(),
  netTraffic: NetTrafficSchema.optional(),
  publicIP: PublicIPSchema.optional(),
  swap: CurTotalInputSchema.optional(),
  tcpCount: z.number().optional(),
  udpCount: z.number().optional(),
  uptime: z.number().optional(),
  appUptime: z.number().optional(),
  appStats: AppStatsSchema.optional(),
  nodes: z.number().optional(),
  nodesOnline: z.number().optional(),
});

export type StatusInput = z.infer<typeof StatusSchema>;
