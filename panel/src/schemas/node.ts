import { z } from 'zod';

export const NodeRecordSchema = z.object({
  id: z.number(),
  name: z.string().optional(),
  remark: z.string().optional(),
  address: z.string().optional(),
  port: z.number().optional(),
  basePath: z.string().optional(),
  apiToken: z.string().optional(),
  enable: z.boolean().optional(),
  status: z.string().optional(),
  latencyMs: z.number().optional(),
  cpuPct: z.number().optional(),
  memPct: z.number().optional(),
  xrayVersion: z.string().optional(),
  panelVersion: z.string().optional(),
  uptimeSecs: z.number().optional(),
  inboundCount: z.number().optional(),
  clientCount: z.number().optional(),
  onlineCount: z.number().optional(),
  depletedCount: z.number().optional(),
  lastHeartbeat: z.number().optional(),
  lastError: z.string().optional(),
  allowPrivateAddress: z.boolean().optional(),
  role: z.enum(['ingress', 'egress']).optional(),
  dnsPrimary: z.string().optional(),
  dnsSecondary: z.string().optional(),
  authority: z.string().optional(),
  certPath: z.string().optional(),
  keyPath: z.string().optional(),
  revision: z.number().optional(),
  appliedRevision: z.number().optional(),
}).loose();

export const NodeListSchema = z.array(NodeRecordSchema);

export const ProbeResultSchema = z.object({
  status: z.string(),
  latencyMs: z.number().optional(),
  xrayVersion: z.string().optional(),
  error: z.string().optional(),
}).loose();

export const NodeEditSchema = z.object({
  id: z.number().optional(),
  name: z.string().trim().min(1, 'pages.nodes.toasts.fillRequired'),
  role: z.enum(['ingress', 'egress']),
  port: z.number().int().min(1).max(65535),
  dnsPrimary: z.string().trim(),
  dnsSecondary: z.string().trim(),
  authority: z.string().trim(),
  certPath: z.string().trim(),
  keyPath: z.string().trim(),
  address: z.string().trim().min(1, 'pages.nodes.toasts.fillRequired'),
});

export const NodeFormSchema = NodeEditSchema.extend({
  uuid: z.string(),
  address: z.string().trim().min(1, 'pages.nodes.toasts.fillRequired'),
  apiToken: z.string().trim().min(1, 'pages.nodes.toasts.fillRequired'),
  enable: z.boolean(),
});

export type NodeRecord = z.infer<typeof NodeRecordSchema>;
export type ProbeResult = z.infer<typeof ProbeResultSchema>;
export type NodeFormValues = z.infer<typeof NodeFormSchema>;
export type NodeEditValues = z.infer<typeof NodeEditSchema>;
