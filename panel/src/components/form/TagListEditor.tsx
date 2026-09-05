import { useState } from 'react';
import { Input, Tag } from '@/components/ds';

export interface TagListEditorProps {
  value?: string[];
  onChange?: (next: string[]) => void;
  /** Preset suggestions shown as togglable chips even when not selected. */
  presets?: string[];
  placeholder?: string;
  /** Characters that split a typed entry into multiple tags. */
  separators?: string[];
}

/** Free-text tag list (antd Select mode="tags" replacement): chips + add-input. */
export default function TagListEditor({
  value,
  onChange,
  presets = [],
  placeholder = 'type and press Enter',
  separators = [',', ' '],
}: TagListEditorProps) {
  const [text, setText] = useState('');
  const tags = value ?? [];

  const commit = (next: string[]) => onChange?.(next);
  const remove = (v: string) => commit(tags.filter((x) => x !== v));
  const toggle = (v: string) => commit(tags.includes(v) ? tags.filter((x) => x !== v) : [...tags, v]);

  const addFromText = () => {
    const parts = separators.reduce<string[]>(
      (acc, sep) => acc.flatMap((s) => s.split(sep)),
      [text],
    );
    const cleaned = parts.map((p) => p.trim()).filter(Boolean).filter((p) => !tags.includes(p));
    if (cleaned.length) commit([...tags, ...cleaned]);
    setText('');
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {tags.map((v) => (
          <Tag key={v} tone="primary" onClick={() => remove(v)} style={{ cursor: 'pointer' }} title="remove">
            {v} ×
          </Tag>
        ))}
        {presets.filter((p) => !tags.includes(p)).map((p) => (
          <Tag key={p} tone="neutral" onClick={() => toggle(p)} style={{ cursor: 'pointer' }}>
            {p}
          </Tag>
        ))}
      </div>
      <Input
        value={text}
        placeholder={placeholder}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            addFromText();
          }
        }}
        onBlur={() => { if (text.trim()) addFromText(); }}
      />
    </div>
  );
}
