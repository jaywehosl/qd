import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { Dialog, Input, Tag } from '@/components/ds';
import { Spin } from '@/components/ui';
import { fetchProcesses } from '@/hooks/useClientRouting';

interface ProcessPickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Lowercased path-or-name of everything that already has a rule. */
  existing: Set<string>;
  onPick: (pick: { process: string; path?: string }) => void;
}

export default function ProcessPickerDialog({
  open, onOpenChange, existing, onPick,
}: ProcessPickerDialogProps) {
  const { t } = useTranslation();
  const [term, setTerm] = useState('');

  const { data: processes = [], isPending } = useQuery({
    queryKey: ['client', 'routing', 'processes'],
    queryFn: fetchProcesses,
    enabled: open,
    // The point of the list is to find what is talking right now, so it is not
    // worth caching across openings.
    staleTime: 0,
  });

  const needle = term.trim().toLowerCase();

  const shown = useMemo(() => {
    const list = needle
      ? processes.filter((p) => p.name.toLowerCase().includes(needle)
        || (p.path ?? '').toLowerCase().includes(needle))
      : processes;

    // A rule keys on the executable, so fifteen chrome.exe rows are fifteen
    // ways to pick the same one. Fold them into the row a rule would create
    // and carry the whole family's flow count on it.
    const folded = new Map<string, typeof list[number] & { instances: number }>();
    for (const p of list) {
      const key = (p.path || p.name).toLowerCase();
      const seen = folded.get(key);
      if (!seen) {
        folded.set(key, { ...p, instances: 1 });
        continue;
      }
      seen.connections = (seen.connections ?? 0) + (p.connections ?? 0);
      seen.instances += 1;
    }

    // Busiest first: whoever came here came to route something that is
    // currently moving traffic, not to read an alphabet.
    return [...folded.values()].sort((a, b) => (b.connections ?? 0) - (a.connections ?? 0)
      || a.name.localeCompare(b.name));
  }, [processes, needle]);

  const exactListed = processes.some((p) => p.name.toLowerCase() === needle);

  const take = (pick: { process: string; path?: string }) => {
    onPick(pick);
    setTerm('');
    onOpenChange(false);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o) setTerm(''); onOpenChange(o); }}
      title={t('client.routing.addRule')}
      width={560}
      footer={null}
    >
      <div className="rt-picker">
        <Input
          autoFocus
          value={term}
          placeholder={t('client.routing.searchProcess')}
          onChange={(e) => setTerm(e.target.value)}
        />

        <div className="rt-picker__list">
          {isPending ? (
            <div className="rt-empty"><Spin spinning size="large" /></div>
          ) : shown.length === 0 && !needle ? (
            <div className="rt-empty">{t('client.routing.noProcesses')}</div>
          ) : shown.map((p) => {
            const ruled = existing.has((p.path || p.name).toLowerCase());
            return (
              <button
                key={(p.path || p.name).toLowerCase()}
                type="button"
                className="rt-proc"
                disabled={ruled}
                onClick={() => take({ process: p.name, path: p.path })}
              >
                {p.icon
                  ? <img className="rt-proc__icon" src={p.icon} alt="" aria-hidden="true" />
                  : <span className="rt-proc__icon rt-proc__icon--blank" aria-hidden="true" />}
                <span className="rt-proc__name">{p.name}</span>
                {p.path && <span className="rt-proc__path">{p.path}</span>}
                {p.instances > 1 && <span className="rt-proc__count">×{p.instances}</span>}
                {ruled
                  ? <Tag>{t('client.routing.alreadyRuled')}</Tag>
                  : p.connections
                    ? <Tag tone="primary">{t('client.routing.flows', { count: p.connections })}</Tag>
                    : null}
              </button>
            );
          })}

          {/* Something that is not running right now still deserves a rule —
              a game you are about to launch, an updater that wakes on a timer. */}
          {needle && !exactListed && (
            <button
              type="button"
              className="rt-proc rt-proc--manual"
              disabled={existing.has(needle)}
              onClick={() => take({ process: term.trim() })}
            >
              <span className="rt-proc__name">{term.trim()}</span>
              <span className="rt-proc__path">{t('client.routing.byNameHint')}</span>
              <Tag tone="warning">{t('client.routing.notRunning')}</Tag>
            </button>
          )}
        </div>
      </div>
    </Dialog>
  );
}
