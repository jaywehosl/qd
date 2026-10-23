import { z } from 'zod';

const nullableStringArray = z.array(z.string()).nullable().transform((v) => v ?? []);
const nullableNumberArray = z.array(z.number()).nullable().transform((v) => v ?? []);

export const ClientTrafficSchema = z.object({
  up: z.number().optional(),
  down: z.number().optional(),
  lastOnline: z.number().optional(),
});

export const ClientDeviceSchema = z.object({
  fingerprint: z.string(),
  platform: z.string().optional(),
  model: z.string().optional(),
  kind: z.string().optional(),
  version: z.string().optional(),
  blocked: z.boolean().optional(),
  ip: z.string().optional(),
  firstSeen: z.number().optional(),
  lastSeen: z.number().optional(),
  up: z.number().optional(),
  down: z.number().optional(),
  nodeId: z.number().optional(),
}).loose();

export const ClientAddressSchema = z.object({
  ip: z.string(),
  fingerprint: z.string().optional(),
  nodeId: z.number().optional(),
  firstSeen: z.number().optional(),
  lastOnline: z.number().optional(),
}).loose();

const ClientExitSchema = z.object({
  nodeId: z.number(),
  firstSeen: z.number().optional(),
  lastOnline: z.number().optional(),
  up: z.number().optional(),
  down: z.number().optional(),
}).loose();

export const ClientRecordSchema = z.object({
  id: z.number(),
  tag: z.string(),
  uuid: z.string().optional(),
  group: z.string().optional(),
  comment: z.string().optional(),
  enable: z.boolean().optional(),
  admin: z.boolean().optional(),
  expiryAt: z.number().optional(),
  deviceLimit: z.number().optional(),
  allowExit: z.number().optional(),
  entrypointIds: nullableNumberArray.optional(),
  traffic: ClientTrafficSchema.nullable().optional(),
  devices: z.array(ClientDeviceSchema).nullable().transform((v) => v ?? []).optional(),
  addresses: z.array(ClientAddressSchema).nullable().transform((v) => v ?? []).optional(),
  exits: z.array(ClientExitSchema).nullable().transform((v) => v ?? []).optional(),
  uri: z.string().optional(),
  lastNodeId: z.number().optional(),
  onlineSince: z.number().optional(),
  lastConnect: z.number().optional(),
  createdAt: z.number().optional(),
  updatedAt: z.number().optional(),
}).loose();

export const ClientRecordListSchema = z.array(ClientRecordSchema).nullable().transform((v) => v ?? []);

export const EntrypointSchema = z.object({
  id: z.number(),
  nodeId: z.number().optional(),
  tag: z.string().optional(),
  port: z.number().optional(),
  enable: z.boolean().optional(),
}).loose();

export const EntrypointListSchema = z.array(EntrypointSchema).nullable().transform((v) => v ?? []);

export const ClientsSummarySchema = z.object({
  total: z.number(),
  active: z.number(),
  online: nullableStringArray,
  expired: nullableStringArray,
  expiring: nullableStringArray,
  disabled: nullableStringArray,
});

export const ClientPageResponseSchema = z.object({
  items: ClientRecordListSchema,
  total: z.number(),
  filtered: z.number(),
  page: z.number(),
  pageSize: z.number(),
  summary: ClientsSummarySchema.nullable().optional(),
  groups: nullableStringArray.optional(),
});

export const OnlinesSchema = nullableStringArray;

export const GroupSummarySchema = z.object({
  id: z.number(),
  tag: z.string(),
  clientCount: z.number(),
  entrypointIds: nullableNumberArray.optional(),
  deviceLimit: z.number().optional(),
  allowExit: z.boolean().optional(),
  allowDev: z.boolean().optional(),
  allowCore: z.boolean().optional(),
}).loose();

export const GroupSummaryListSchema = z.array(GroupSummarySchema).nullable().transform((v) => v ?? []);

function hasForbiddenChars(value: string): boolean {
  if (value.includes('/') || value.includes('\\') || value.includes(' ')) return true;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code < 0x20 || code === 0x7f) return true;
  }
  return false;
}

export const ClientFormSchema = z.object({
  tag: z
    .string()
    .trim()
    .min(1, 'pages.clients.tagRequired')
    .refine((v) => !hasForbiddenChars(v), 'pages.clients.tagInvalidChars'),
  uuid: z
    .string()
    .trim()
    .min(1, 'pages.clients.uuidRequired')
    .refine((v) => !hasForbiddenChars(v), 'pages.clients.uuidInvalidChars'),
  group: z.string(),
});

export const ClientCreateFormSchema = ClientFormSchema.refine(
  (v) => v.group.trim() !== '',
  { message: 'pages.clients.groupRequired', path: ['group'] },
);

export type ClientRecord = z.infer<typeof ClientRecordSchema>;
export type ClientTraffic = z.infer<typeof ClientTrafficSchema>;
export type ClientDevice = z.infer<typeof ClientDeviceSchema>;
export type ClientAddress = z.infer<typeof ClientAddressSchema>;
export type Entrypoint = z.infer<typeof EntrypointSchema>;
export type ClientsSummary = z.infer<typeof ClientsSummarySchema>;
export type ClientPageResponse = z.infer<typeof ClientPageResponseSchema>;
export type GroupSummary = z.infer<typeof GroupSummarySchema>;
