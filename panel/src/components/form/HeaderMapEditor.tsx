import { useEffect, useRef, useState } from 'react';
import { MinusOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Input } from '@/components/ds';


export type HeaderMapMode = 'v1' | 'v2';

export type HeaderMapValue =
  | Record<string, string>
  | Record<string, string[]>
  | undefined;

interface HeaderRow {
  name: string;
  value: string;
}

interface HeaderMapEditorProps {
  mode: HeaderMapMode;
  value?: HeaderMapValue;
  onChange?: (next: Record<string, string> | Record<string, string[]>) => void;
}

function mapToRows(value: HeaderMapValue): HeaderRow[] {
  if (!value || typeof value !== 'object') return [];
  const out: HeaderRow[] = [];
  for (const [name, raw] of Object.entries(value)) {
    if (Array.isArray(raw)) {
      for (const v of raw) {
        out.push({ name, value: typeof v === 'string' ? v : String(v) });
      }
    } else if (typeof raw === 'string') {
      out.push({ name, value: raw });
    }
  }
  return out;
}

function rowsToMap(rows: HeaderRow[], mode: HeaderMapMode): Record<string, string> | Record<string, string[]> {
  if (mode === 'v1') {
    const map: Record<string, string> = {};
    for (const r of rows) {
      if (!r.name) continue;
      map[r.name] = r.value ?? '';
    }
    return map;
  }
  const map: Record<string, string[]> = {};
  for (const r of rows) {
    if (!r.name) continue;
    const list = map[r.name] ?? [];
    list.push(r.value ?? '');
    map[r.name] = list;
  }
  return map;
}

export default function HeaderMapEditor({ mode, value, onChange }: HeaderMapEditorProps) {
  const [rows, setRows] = useState<HeaderRow[]>(() => mapToRows(value));
  const lastEmittedRef = useRef<string>(JSON.stringify(rowsToMap(rows, mode)));

  useEffect(() => {
    const incoming = JSON.stringify(value ?? {});
    if (incoming === lastEmittedRef.current) return;
    setRows(mapToRows(value));
    lastEmittedRef.current = incoming;
  }, [value]);

  function commit(next: HeaderRow[]) {
    setRows(next);
    const map = rowsToMap(next, mode);
    lastEmittedRef.current = JSON.stringify(map);
    onChange?.(map);
  }

  function setRow(index: number, patch: Partial<HeaderRow>) {
    const next = rows.slice();
    next[index] = { ...next[index], ...patch };
    commit(next);
  }

  function addRow() {
    commit([...rows, { name: '', value: '' }]);
  }

  function removeRow(index: number) {
    const next = rows.slice();
    next.splice(index, 1);
    commit(next);
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {rows.map((row, idx) => (
        <div key={idx} style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
          <span className="ds-muted" style={{ width: 18 }}>{idx + 1}</span>
          <Input value={row.name} placeholder="Name" onChange={(e) => setRow(idx, { name: e.target.value })} />
          <Input value={row.value} placeholder="Value" onChange={(e) => setRow(idx, { value: e.target.value })} />
          <Button size="sm" icon={<MinusOutlined />} onClick={() => removeRow(idx)} />
        </div>
      ))}
      <Button size="sm" variant="primary" icon={<PlusOutlined />} onClick={addRow} style={{ alignSelf: 'flex-start' }}>Add</Button>
    </div>
  );
}
