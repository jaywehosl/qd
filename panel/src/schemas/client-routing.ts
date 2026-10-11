import { z } from 'zod';

export const ROUTING_ROLES = ['direct', 'tunnel', 'egress', 'noEgress'] as const;
export type RoutingRole = (typeof ROUTING_ROLES)[number];

const RoleSchema = z.enum(ROUTING_ROLES).catch('tunnel');

export const RoutingRuleSchema = z.object({
  id: z.number(),
  process: z.string(),
  title: z.string().optional(),
  path: z.string().optional(),
  icon: z.string().optional(),
  role: RoleSchema,
  running: z.boolean().optional(),
  matched: z.number().optional(),
}).loose();

export const DomainRuleSchema = z.object({
  id: z.number(),
  domain: z.string(),
  role: RoleSchema,
  matched: z.number().optional(),
}).loose();

export const BundleSchema = z.object({
  id: z.string(),
  name: z.string(),
  role: z.string().catch(''),
  matched: z.number().optional(),
  services: z.array(z.object({ id: z.string(), name: z.string() })).nullish().transform((v) => v ?? []),
}).loose();

export const RoutingStateSchema = z.object({
  defaultRole: RoleSchema,
  allowExit: z.boolean().catch(false),
  byDomain: z.boolean().optional(),
  applyMode: z.enum(['live', 'restart']).catch('live'),
  pendingRestart: z.boolean().optional(),
  rules: z.array(RoutingRuleSchema).nullable().transform((v) => v ?? []),
  domains: z.array(DomainRuleSchema).nullish().transform((v) => v ?? []),
  bundles: z.array(BundleSchema).nullish().transform((v) => v ?? []),
}).loose();

export const ProcessSchema = z.object({
  name: z.string(),
  title: z.string().optional(),
  path: z.string().optional(),
  icon: z.string().optional(),
  pid: z.number().optional(),
  connections: z.number().optional(),
}).loose();

export const ProcessListSchema = z.array(ProcessSchema).nullable().transform((v) => v ?? []);

export type RoutingRule = z.infer<typeof RoutingRuleSchema>;
export type DomainRule = z.infer<typeof DomainRuleSchema>;
export type Bundle = z.infer<typeof BundleSchema>;
export type RoutingState = z.infer<typeof RoutingStateSchema>;
export type RunningProcess = z.infer<typeof ProcessSchema>;
