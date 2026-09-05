import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button, Dialog, Field, Select, Tag, toast } from '@/components/ds';
import { HttpUtil } from '@/utils';
import { useFormSeed } from '@/hooks/useFormSeed';
import type { DBInbound } from '@/models/dbinbound';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' }, silent: true } as const;

export interface EntryFormValues {
  remark: string;
  port: number;
  enable: boolean;
  nodeId: number;
}

interface EntryFormModalProps {
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
  mode: 'add' | 'edit';
  dbInbound: DBInbound | null;
  dbInbounds: DBInbound[];
  availableNodes?: NodeRecord[];
}

const EMPTY: EntryFormValues = {
  remark: '',
  port: 443,
  enable: true,
  nodeId: 0,
};

export default function EntryFormModal({
  open,
  onClose,
  onSaved,
  mode,
  dbInbound,
  dbInbounds,
  availableNodes = [],
}: EntryFormModalProps) {
  const { t } = useTranslation();
  const [form, setForm] = useState<EntryFormValues>(EMPTY);
  const [saving, setSaving] = useState(false);

  const isEdit = mode === 'edit';
  const nodeId = form.nodeId;
  const node = availableNodes.find((n) => n.id === nodeId);

  useFormSeed(open, `${mode}:${dbInbound?.id ?? 0}`, () => {
    if (isEdit && dbInbound) {
      setForm({
        remark: dbInbound.remark ?? '',
        port: dbInbound.port ?? 443,
        enable: dbInbound.enable ?? true,
        nodeId: dbInbound.nodeId ?? 0,
      });
    } else {
      setForm({ ...EMPTY, nodeId: availableNodes[0]?.id ?? 0 });
    }
  });

  useEffect(() => {
    if (!open || isEdit) return;
    setForm((prev) => (prev.nodeId ? prev : { ...prev, nodeId: availableNodes[0]?.id ?? 0 }));
  }, [open, isEdit, availableNodes]);

  useEffect(() => {
    if (!open || !node?.port) return;
    setForm((prev) => (prev.port === node.port ? prev : { ...prev, port: node.port as number }));
  }, [open, node?.port]);

  function update<K extends keyof EntryFormValues>(key: K, value: EntryFormValues[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  const portTaken = dbInbounds.some(
    (i) => i.port === form.port
      && i.id !== dbInbound?.id
      && (i.nodeId ?? 0) === nodeId,
  );

  const submit = async () => {
    if (portTaken) {
      toast.error(t('pages.inbounds.portTaken', { defaultValue: 'Port already used on this node' }));
      return;
    }
    if (!form.nodeId) {
      toast.error(t('pages.inbounds.pickNode', { defaultValue: 'Pick the node this entrypoint lives on' }));
      return;
    }

    setSaving(true);
    try {
      const owner = availableNodes.find((n) => n.id === form.nodeId);
      const body = {
        nodeId: form.nodeId,
        remark: owner?.name || owner?.address || '',
        port: form.port,
        enable: form.enable,
      };
      const msg = isEdit && dbInbound
        ? await HttpUtil.post(`/panel/api/inbounds/update/${dbInbound.id}`, body, JSON_HEADERS)
        : await HttpUtil.post('/panel/api/inbounds/add', body, JSON_HEADERS);
      if (!msg?.success) {
        toast.error(msg?.msg || t('somethingWentWrong'));
        return;
      }
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  };

  const name = node?.name || node?.address || dbInbound?.remark || '—';

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => { if (!v) onClose(); }}
      // Two fields do not need the width of a form: this is a narrow dialog.
      width={520}
      autoHeight
      title={
        <span className="ef-title">
          {isEdit ? (
            <>
              {t('edit', { defaultValue: 'Edit' })}
              <Tag tone={node?.status === 'online' ? 'success' : 'neutral'} className="ef-title__node">
                {name}
              </Tag>
              {t('pages.inbounds.entrypointWord', { defaultValue: 'entrypoint' })}
            </>
          ) : t('pages.inbounds.addEntry', { defaultValue: 'Add entrypoint' })}
        </span>
      }
      footer={
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, width: '100%' }}>
          <span style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
            <Button onClick={onClose}>{t('close')}</Button>
            <Button variant="primary" loading={saving} onClick={submit}>{t('save')}</Button>
          </span>
        </div>
      }
    >
      {!isEdit && (
        <div style={{ marginBottom: 12 }}>
          <Field label={t('pages.inbounds.node', { defaultValue: 'Node' })}>
            <Select
              value={String(form.nodeId || '')}
              placeholder={t('pages.inbounds.pickNode', { defaultValue: 'Pick the node this entrypoint lives on' })}
              onChange={(v) => update('nodeId', Number(v) || 0)}
              options={availableNodes.map((n) => ({
                value: String(n.id),
                label: `${n.name || n.address} · ${n.role ?? 'ingress'}`,
              }))}
            />
          </Field>
        </div>
      )}

      <span style={{ display: 'block', opacity: 0.7, fontSize: 13 }}>
        {t('pages.inbounds.portFollowsNode', {
          defaultValue: 'The entrypoint answers on the port of its node — set it in the node card.',
        })}
      </span>

      {portTaken && (
        <div style={{ marginTop: 10 }}>
          <Tag tone="danger">
            {t('pages.inbounds.portTaken', { defaultValue: 'This node already has an entrypoint' })}
          </Tag>
        </div>
      )}
    </Dialog>
  );
}
