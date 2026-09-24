export interface RouteService {
  id: string;
  name: string;
  site: string;
  note?: string;
  entries: string[];
}

const icons = import.meta.glob('../../assets/services/*.png', { eager: true, import: 'default' }) as Record<string, string>;

export function iconOf(id: string): string | undefined {
  return icons[`../../assets/services/${id}.png`];
}

export function pickedOf(text?: string): string[] {
  return (text ?? '').split(/[\s,]+/).filter(Boolean);
}

function clean(entry: string): string {
  return entry.trim().toLowerCase().replace(/^\*\./, '').replace(/\.$/, '');
}

function entriesOf(text: string): Set<string> {
  const out = new Set<string>();
  for (const line of text.split('\n')) {
    for (const entry of line.split('#')[0].split(/[\s,]+/)) {
      const e = clean(entry);
      if (e) out.add(e);
    }
  }
  return out;
}

export function countEntries(text: string): number {
  return entriesOf(text).size;
}

export function adoptPreset(list: string, services: RouteService[]): { list: string; ids: string[] } | null {
  const have = entriesOf(list);
  const found = services.filter((s) => s.entries.some((e) => have.has(e)));
  if (found.length === 0) return null;

  const drop = new Set(found.flatMap((s) => s.entries));
  const kept: string[] = [];
  for (const line of list.split('\n')) {
    const [bare, ...note] = line.split('#');
    const entries = bare.split(/[\s,]+/).filter(Boolean);
    const left = entries.filter((e) => !drop.has(clean(e)));
    if (entries.length > 0 && left.length === 0) continue;
    kept.push(left.length === entries.length ? line : [left.join(' '), ...note].join(' #'));
  }

  const lines: string[] = [];
  kept.forEach((line, i) => {
    if (line.trim().startsWith('#')) {
      const next = kept.slice(i + 1).find((l) => l.trim() !== '');
      if (next === undefined || next.trim().startsWith('#')) return;
    }
    lines.push(line);
  });

  const text = lines.join('\n').replace(/\n{3,}/g, '\n\n').trim();
  return { list: text ? `${text}\n` : '', ids: found.map((s) => s.id) };
}
