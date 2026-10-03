import type { ReactNode } from 'react';
import type { ColumnDef } from '@/components/ds';

export function cellOf<T>(columns: ColumnDef<T, unknown>[], id: string, record: T): ReactNode {
  const cell = columns.find((c) => c.id === id)?.cell;
  return typeof cell === 'function' ? cell({ row: { original: record } } as never) : null;
}
