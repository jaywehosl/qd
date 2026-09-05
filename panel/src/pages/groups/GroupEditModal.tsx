import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button, Dialog, Divider, Field, Input, Switch, Tag } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { clientsApi } from '@/generated/client';
import { useFormSeed } from '@/hooks/useFormSeed';
import type { ClientRecord, GroupSummary, InboundOption } from '@/schemas/client';
interface GroupEditModalProps {
  open: boolean;
  group: GroupSummary | null;
  groups: GroupSummary[];
  inbounds: InboundOption[];
  clients: ClientRecord[];
  onClose: () => void;
  onSaved: () => void;
}

function entryLabel(ib: InboundOption): string {
  return ib.tag?.trim() || ib.remark?.trim() || String(ib.id);
}

function sameSet(a: number[], b: number[]): boolean {
  if (a.length !== b.length) return false;
  const s = new Set(a);
  return b.every((x) => s.has(x));
}

export default function GroupEditModal({
  open,
  group,
  groups,
  inbounds,
  clients,
  onClose,
  onSaved,
}: GroupEditModalProps) {
  const { t } = useTranslation();
  const message = getMessage();

  const [name, setName] = useState('');
  const [entrypointIds, setEntrypointIds] = useState<number[]>([]);
  const [emails, setEmails] = useState<string[]>([]);
  const [deviceLimit, setDeviceLimit] = useState(0);
  const [allowExit, setAllowExit] = useState(false);
  const [saving, setSaving] = useState(false);
  const [touched, setTouched] = useState(false);

  const activeInbounds = useMemo(() => inbounds.filter((i) => i.enable !== false), [inbounds]);
  const originalEmails = useMemo(
    () => clients.filter((c) => c.group === group?.name).map((c) => c.email),
    [clients, group?.name],
  );

  useFormSeed(open, group?.name ?? '', () => {
    if (!group) return;
    setName(group.name);
    setEntrypointIds([...(group.entrypointIds ?? [])]);
    setEmails(clients.filter((c) => c.group === group.name).map((c) => c.email));
    setDeviceLimit(Number((group as { deviceLimit?: number }).deviceLimit) || 0);
    setAllowExit(!!(group as { allowExit?: boolean }).allowExit);
    setTouched(false);
  });

  useEffect(() => {
    if (!open || touched) return;
    setEmails(originalEmails);
  }, [open, touched, originalEmails]);

  function toggleEntry(id: number) {
    setEntrypointIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  function toggleClient(email: string) {
    setTouched(true);
    setEmails((prev) => (prev.includes(email) ? prev.filter((x) => x !== email) : [...prev, email]));
  }

  async function submit() {
    if (!group) return;
    const nextName = name.trim();
    if (!nextName) {
      message.error(t('pages.groups.nameRequired', { defaultValue: 'Group tag cannot be empty' }));
      return;
    }
    if (nextName !== group.name && groups.some((g) => g.name.toLowerCase() === nextName.toLowerCase())) {
      message.error(t('pages.groups.renameCollision', { name: nextName }));
      return;
    }

    setSaving(true);
    try {
      if (nextName !== group.name) {
        const msg = await clientsApi.groupsRename({ oldName: group.name, newName: nextName }, { silent: true });
        if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      }

      if (!sameSet(entrypointIds, group.entrypointIds ?? [])
          || deviceLimit !== (Number((group as { deviceLimit?: number }).deviceLimit) || 0)
          || allowExit !== !!(group as { allowExit?: boolean }).allowExit) {
        const msg = await clientsApi.groupsEntrypoints(
          { name: nextName, entrypointIds, deviceLimit, allowExit }, { silent: true });
        if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      }

      const added = emails.filter((e) => !originalEmails.includes(e));
      const removed = originalEmails.filter((e) => !emails.includes(e));
      if (added.length > 0) {
        const msg = await clientsApi.groupsBulkAdd({ emails: added, group: nextName }, { silent: true });
        if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      }
      if (removed.length > 0) {
        const msg = await clientsApi.groupsBulkRemove({ emails: removed }, { silent: true });
        if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      }

      message.success(t('pages.groups.saveSuccess', { defaultValue: 'Group saved' }));
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o && !saving) onClose(); }}
      width={720}
      autoHeight
      title={(
        <span className="ef-title">
          {t('edit', { defaultValue: 'Edit' })}
          <Tag tone="success" className="ef-title__node">{group?.name ?? '—'}</Tag>
          {t('pages.groups.groupWord', { defaultValue: 'group' })}
        </span>
      )}
      footer={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
          <span style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
            <Button onClick={onClose}>{t('close')}</Button>
            <Button variant="primary" loading={saving} onClick={submit}>{t('save')}</Button>
          </span>
        </div>
      }
    >
      <div className="group-edit">
      <div className="ge-head">
        <Field label={t('pages.groups.groupTag', { defaultValue: 'Group Tag' })}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={t('pages.groups.deviceLimit', { defaultValue: 'Device limit (0 = unlimited)' })}>
          <Input
            type="number"
            min={0}
            value={deviceLimit}
            onChange={(e) => setDeviceLimit(Math.max(0, Number(e.target.value) || 0))}
          />
        </Field>
      </div>

      <div className="ge-toggle">
        <span className="ge-toggle__label">
          {t('pages.groups.allowExit', { defaultValue: 'Allow exit nodes' })}
        </span>
        <Switch
          checked={allowExit}
          onChange={setAllowExit}
          aria-label={t('pages.groups.allowExit', { defaultValue: 'Allow exit nodes' })}
        />
      </div>

      <Divider>{t('pages.groups.inboundsInGroup', { defaultValue: 'Inbounds in group' })}</Divider>
      {activeInbounds.length === 0 ? (
        <div className="ge-empty">{t('pages.groups.noInbounds', { defaultValue: 'No active entrypoints.' })}</div>
      ) : (
        <div className="ge-picks">
          {activeInbounds.map((ib) => {
            const on = entrypointIds.includes(ib.id);
            return (
              <Tag
                key={ib.id}
                tone={on ? 'primary' : 'neutral'}
                className={`ge-pick${on ? ' is-on' : ''}`}
                onClick={() => toggleEntry(ib.id)}
              >
                {entryLabel(ib)}
              </Tag>
            );
          })}
        </div>
      )}

      <Divider>{t('pages.groups.clientsInGroup', { defaultValue: 'Clients in group' })}</Divider>
      {clients.length === 0 ? (
        <div className="ge-empty">{t('pages.groups.noClients', { defaultValue: 'No clients yet.' })}</div>
      ) : (
        <div className="ge-picks">
          {clients.map((c) => {
            const on = emails.includes(c.email);
            const taken = !on && !!c.group && c.group !== group?.name;
            return (
              <Tag
                key={c.email}
                tone={on ? 'success' : 'neutral'}
                className={`ge-pick${on ? ' is-on' : ''}${taken ? ' is-taken' : ''}`}
                title={taken ? t('pages.groups.movesFrom', { defaultValue: 'Currently in {name}', name: c.group }) : undefined}
                onClick={() => toggleClient(c.email)}
              >
                {c.email}
              </Tag>
            );
          })}
        </div>
      )}
      </div>
    </Dialog>
  );
}
