import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined, MinusCircleOutlined } from '@ant-design/icons';

import { Button, Dialog, Tag } from '@/components/ds';
import { HttpUtil } from '@/utils';
import type { DraftState } from '@/layouts/PublishController';
type Phase = 'plan' | 'push' | 'apply' | 'done';

interface NodeRow {
  id: number;
  name: string;
  role?: string;
  state: string;
  attempts?: number;
  error?: string;
  bytes?: number;
}

interface Skipped {
  id: number;
  name: string;
  reason: string;
}

interface PublishModalProps {
  open: boolean;
  draft: DraftState | null;
  onClose: () => void;
}

const PHASES: Phase[] = ['plan', 'push', 'apply'];

const SETTLED: Record<Phase, string[]> = {
  plan: ['planned'],
  push: ['staged', 'failed'],
  apply: ['applied', 'failed'],
  done: [],
};

function stateIcon(state: string) {
  if (state === 'failed') return <CloseCircleOutlined style={{ color: 'var(--color-error)' }} />;
  if (state === 'applied' || state === 'staged' || state === 'planned') {
    return <CheckCircleOutlined style={{ color: 'var(--color-success)' }} />;
  }
  if (state === 'skipped') return <MinusCircleOutlined style={{ color: 'var(--text-3)' }} />;
  return <LoadingOutlined style={{ color: 'var(--color-primary)' }} />;
}

export default function PublishModal({ open, draft, onClose }: PublishModalProps) {
  const { t } = useTranslation();

  const [phase, setPhase] = useState<Phase>('plan');
  const [revision, setRevision] = useState(0);
  const [nodes, setNodes] = useState<NodeRow[]>([]);
  const [skipped, setSkipped] = useState<Skipped[]>([]);
  const [running, setRunning] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    (async () => {
      const msg = await HttpUtil.get<{ revision: number; phase: Phase; nodes: NodeRow[] }>(
        '/panel/api/publish/status', undefined, { silent: true },
      );
      if (cancelled) return;
      if (msg?.success && msg.obj && msg.obj.revision > 0) {
        setRevision(msg.obj.revision);
        setPhase(msg.obj.phase);
        setNodes(msg.obj.nodes || []);
      } else {
        setPhase('plan');
        setRevision(0);
        setNodes([]);
        setSkipped([]);
      }
    })();
    return () => { cancelled = true; };
  }, [open]);

  const failed = nodes.filter((n) => n.state === 'failed');
  const settled = nodes.length > 0
    && nodes.every((n) => SETTLED[phase].includes(n.state) || n.state === 'skipped');
  const canAdvance = settled && failed.length === 0;

  const runPlan = useCallback(async () => {
    setRunning(true);
    try {
      const msg = await HttpUtil.post<{ revision: number; targets: NodeRow[]; skipped: Skipped[] }>(
        '/panel/api/publish/plan', {},
      );
      if (msg?.success && msg.obj) {
        setRevision(msg.obj.revision);
        setNodes((msg.obj.targets || []).map((n) => ({ ...n, state: 'planned' })));
        setSkipped(msg.obj.skipped || []);
      }
    } finally {
      setRunning(false);
    }
  }, []);

  const runPush = useCallback(async (only?: number[]) => {
    setRunning(true);
    setNodes((prev) => prev.map((n) => (
      !only || only.includes(n.id) ? { ...n, state: 'pushing', error: undefined } : n
    )));
    try {
      const msg = await HttpUtil.post<NodeRow[]>('/panel/api/publish/push',
        { revision, ...(only ? { nodeIds: only } : {}) },
        { headers: { 'Content-Type': 'application/json' } });
      if (msg?.success && Array.isArray(msg.obj)) {
        const byId = new Map(msg.obj.map((n) => [n.id, n]));
        setNodes((prev) => prev.map((n) => ({ ...n, ...(byId.get(n.id) ?? {}) })));
      }
    } finally {
      setRunning(false);
    }
  }, [revision]);

  const runApply = useCallback(async () => {
    setRunning(true);
    setNodes((prev) => prev.map((n) => (n.state === 'staged' ? { ...n, state: 'applying' } : n)));
    try {
      const msg = await HttpUtil.post<NodeRow[]>('/panel/api/publish/apply',
        { revision }, { headers: { 'Content-Type': 'application/json' } });
      if (msg?.success && Array.isArray(msg.obj)) {
        const byId = new Map(msg.obj.map((n) => [n.id, n]));
        setNodes((prev) => prev.map((n) => ({ ...n, ...(byId.get(n.id) ?? {}) })));
      }
    } finally {
      setRunning(false);
    }
  }, [revision]);

  const next = useCallback(async () => {
    if (phase === 'plan') { await runPush(); setPhase('push'); return; }
    if (phase === 'push') { await runApply(); setPhase('apply'); return; }
    if (phase === 'apply') { setPhase('done'); onClose(); }
  }, [phase, runPush, runApply, onClose]);

  useEffect(() => {
    if (open && phase === 'plan' && revision === 0 && !running) void runPlan();
  }, [open, phase, revision]);

  const stepIndex = PHASES.indexOf(phase === 'done' ? 'apply' : phase);

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o) onClose(); }}
      width={720}
      autoHeight
      title={t('publish.title')}
      footer={
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, width: '100%' }}>
          {failed.length > 0 && (
            <Button
              loading={running}
              onClick={() => (phase === 'apply' ? runApply() : runPush(failed.map((n) => n.id)))}
            >
              {t('publish.retry', { count: failed.length })}
            </Button>
          )}
          <span style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
            <Button onClick={onClose}>{t('cancel')}</Button>
            <Button variant="primary" disabled={!canAdvance} loading={running} onClick={next}>
              {phase === 'apply' ? t('publish.finish') : t('publish.next')}
            </Button>
          </span>
        </div>
      }
    >
      <div className="pub-steps">
        {PHASES.map((p, i) => (
          <span key={p} className={`pub-step${i === stepIndex ? ' is-active' : ''}${i < stepIndex ? ' is-done' : ''}`}>
            {t(`publish.phase.${p}`)}
          </span>
        ))}
        {revision > 0 && <Tag tone="primary">{t('publish.revision', { n: revision })}</Tag>}
      </div>

      <p className="pub-hint">{t(`publish.hint.${phase === 'done' ? 'apply' : phase}`)}</p>

      <div className="pub-rows">
        {nodes.map((n) => (
          <div key={n.id} className="pub-row">
            <span className="pub-row__icon">{stateIcon(n.state)}</span>
            <span className="pub-row__name">{n.name}</span>
            {n.role && <Tag>{n.role}</Tag>}
            <span className="pub-row__state">
              {t(`publish.state.${n.state}`, { defaultValue: n.state })}
              {n.attempts ? ` · ${t('publish.attempts', { n: n.attempts })}` : ''}
            </span>
            {n.error && <span className="pub-row__error">{n.error}</span>}
          </div>
        ))}

        {skipped.map((s) => (
          <div key={`skip-${s.id}`} className="pub-row is-skipped">
            <span className="pub-row__icon">{stateIcon('skipped')}</span>
            <span className="pub-row__name">{s.name}</span>
            <span className="pub-row__state">{s.reason}</span>
          </div>
        ))}
      </div>

      {skipped.length > 0 && <p className="pub-hint">{t('publish.skippedHint')}</p>}

      {phase === 'plan' && (draft?.changes?.length ?? 0) > 0 && (
        <div className="pub-changes">
          {(draft?.changes ?? []).map((c, i) => (
            <Tag key={i}>{c.kind}: {c.name} — {c.action}</Tag>
          ))}
        </div>
      )}
    </Dialog>
  );
}
