import { z } from 'zod';

const nonNegativeInt = z.number().int().min(0);

export const AllSettingSchema = z.object({
  pageSize: z.number().int().min(0).max(1000).optional(),
  refreshMinutes: z.number().optional(),
  expireDiff: nonNegativeInt.optional(),
  trafficDiff: nonNegativeInt.max(100).optional(),
  remarkModel: z.string().optional(),
  datepicker: z.enum(['gregorian', 'jalalian']).optional(),
  timeLocation: z.string().optional(),
  dnsPrimary: z.string().optional(),
  dnsSecondary: z.string().optional(),
  dnsCache: nonNegativeInt.optional(),
  dnsMinTtl: nonNegativeInt.optional(),
  dnsMaxTtl: nonNegativeInt.optional(),
  dnsStale: nonNegativeInt.optional(),
  mtu: nonNegativeInt.optional(),
  statsSeconds: nonNegativeInt.optional(),
  pool: z.string().optional(),
  brutalMbit: nonNegativeInt.optional(),
  maxStreams: nonNegativeInt.optional(),
  streamWindowKb: nonNegativeInt.optional(),
  maxStreamWindowKb: nonNegativeInt.optional(),
  connWindowKb: nonNegativeInt.optional(),
  maxConnWindowKb: nonNegativeInt.optional(),
  idleSeconds: nonNegativeInt.optional(),
  keepAliveSeconds: nonNegativeInt.optional(),
  socketBufferKb: nonNegativeInt.optional(),
}).loose();

export type AllSettingInput = z.infer<typeof AllSettingSchema>;
