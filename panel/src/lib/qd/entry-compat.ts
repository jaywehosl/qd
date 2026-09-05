import type { EntryLike } from '@/lib/qd/entry-link';

export function isSSMultiUser(_protocol?: unknown, _settings?: unknown): boolean {
  return false;
}

export function isPostQuantumLink(_link?: string): boolean {
  return false;
}

export const LinkTags: Record<string, string> = {};

export interface InboundTagInput {
  remark?: string;
  port?: number;
  listen?: string;
}

export function composeInboundTag(input: InboundTagInput): string {
  const host = input.listen && input.listen !== '0.0.0.0' ? input.listen : 'entry';
  return `${host}-${input.port ?? 443}`;
}

export function isAutoInboundTag(tag: string, input: InboundTagInput): boolean {
  return tag === composeInboundTag(input);
}

export function createDefaultInboundSettings(_protocol?: unknown): EntryLike {
  return {
    remark: '',
    listen: '0.0.0.0',
    port: 443,
    enable: true,
  };
}

export function canEnableTlsFlow(_a?: unknown, _b?: unknown): boolean {
  return false;
}

export function isSS2022(_a?: unknown): boolean {
  return false;
}
