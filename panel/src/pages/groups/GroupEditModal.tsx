import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button, Dialog, Divider, Field, Input, Switch, Tag } from '@/components/ds';
import { VerticalTabs } from '@/components/ui';
import { phone } from '@/lib/phone';
import { getMessage } from '@/utils/messageBus';
import { HttpUtil } from '@/utils';
import { useFormSeed } from '@/hooks/useFormSeed';
import type { ClientRecord, Entrypoint, GroupSummary } from '@/schemas/client';
interface GroupEditModalProps {
  open: boolean;
  group: GroupSummary | null;
  groups: GroupSummary[];
  entrypoints: Entrypoint[];
  clients: ClientRecord[];
  onClose: () => void;
  onSaved: () => void;
}

export default function GroupEditModal({
  open,
  group,
  groups,
  entrypoints,
  clients,
  onClose,
  onSaved,
}: GroupEditModalProps) {
  const { t } = useTranslation();
  const message = getMessage();

  const [tag, setTag] = useState('');
  const [entrypointIds, setEntrypointIds] = useState<number[]>([]);
  const [members, setMembers] = useState<number[]>([]);
  const [deviceLimit, setDeviceLimit] = useState(0);
  const [allowExit, setAllowExit] = useState(false);
  const [allowDev, setAllowDev] = useState(false);
  const [allowCore, setAllowCore] = useState(false);
  const [saving, setSaving] = useState(false);
  const [touched, setTouched] = useState(false);

  const activeEntrypoints = useMemo(() => entrypoints.filter((e) => e.enable !== false), [entrypoints]);
  const originalMembers = useMemo(
    () => clients.filter((c) => c.group === group?.tag).map((c) => c.id),
    [clients, group?.tag],
  );

  useFormSeed(open, group?.tag ?? '', () => {
    if (!group) return;
    setTag(group.tag);
    setEntrypointIds([...(group.entrypointIds ?? [])]);
    setMembers(originalMembers);
    setDeviceLimit(group.deviceLimit || 0);
    setAllowExit(!!group.allowExit);
    setAllowDev(!!group.allowDev);
    setAllowCore(!!group.allowCore);
    setTouched(false);
  });

  useEffect(() => {
    if (!open || touched) return;
    setMembers(originalMembers);
  }, [open, touched, originalMembers]);

  function toggleEntry(id: number) {
    setEntrypointIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  function toggleClient(id: number) {
    setTouched(true);
    setMembers((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  async function submit() {
    if (!group) return;
    const nextTag = tag.trim();
    if (!nextTag) {
      message.error(t('pages.groups.nameRequired', { defaultValue: 'Group tag cannot be empty' }));
      return;
    }
    if (groups.some((g) => g.id !== group.id && g.tag.toLowerCase() === nextTag.toLowerCase())) {
      message.error(t('pages.groups.renameCollision', { name: nextTag }));
      return;
    }

    setSaving(true);
    try {
      const steps: [string, unknown][] = [
        ['/panel/api/clients/groups/save', { id: group.id, tag: nextTag, entrypointIds, deviceLimit, allowExit, allowDev, allowCore }],
      ];
      const added = members.filter((id) => !originalMembers.includes(id));
      const removed = originalMembers.filter((id) => !members.includes(id));
      if (added.length > 0) steps.push(['/panel/api/clients/groups/members', { group: group.id, clients: added }]);
      if (removed.length > 0) steps.push(['/panel/api/clients/groups/members', { group: 0, clients: removed }]);
      for (const [path, body] of steps) {
        const msg = await HttpUtil.post(path, body, { silent: true });
        if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      }

      message.success(t('pages.groups.saveSuccess', { defaultValue: 'Group saved' }));
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  }

  const onPhone = !!phone();
  const [tab, setTab] = useState('general');
  const shows = (part: string) => !onPhone || tab === part;

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o && !saving) onClose(); }}
      width={720}
      autoHeight
      title={onPhone ? (
        <span className="ms-title">
          <small>{t('pages.groups.editGroup', { defaultValue: 'Edit Group' })}</small>
          <span>{group?.tag ?? '—'}</span>
        </span>
      ) : (
        <span className="ef-title">
          {t('edit', { defaultValue: 'Edit' })}
          <Tag tone="success" className="ef-title__node">{group?.tag ?? '—'}</Tag>
          {t('pages.groups.groupWord', { defaultValue: 'group' })}
        </span>
      )}
      footer={onPhone ? (
        <div className="ms-actions">
          <Button onClick={onClose}>{t('close')}</Button>
          <Button variant="primary" loading={saving} onClick={submit}>{t('save')}</Button>
        </div>
      ) : (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
          <span style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
            <Button onClick={onClose}>{t('close')}</Button>
            <Button variant="primary" loading={saving} onClick={submit}>{t('save')}</Button>
          </span>
        </div>
      )}
    >
      <div className="group-edit">
      {onPhone && (
        <div className="ge-tabs">
          <VerticalTabs
            items={[
              { key: 'general', label: t('pages.groups.tabGeneral', { defaultValue: 'General' }) },
              { key: 'members', label: t('pages.groups.tabMembers', { defaultValue: 'Members' }) },
            ]}
            activeKey={tab}
            onChange={setTab}
          />
        </div>
      )}
      {shows('general') && (
      <>
      <div className="ge-head">
        <Field label={t('pages.groups.groupTag', { defaultValue: 'Group Tag' })}>
          <Input value={tag} onChange={(e) => setTag(e.target.value)} />
        </Field>
        <Field label={t('pages.groups.deviceLimit', { defaultValue: 'Device limit' })}>
          <Input
            type="number"
            min={0}
            value={deviceLimit}
            onChange={(e) => setDeviceLimit(Math.max(0, Number(e.target.value) || 0))}
          />
        </Field>
      </div>

      <div className="ge-toggles">
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
        <div className="ge-toggle">
          <span className="ge-toggle__label">{t('pages.groups.allowDev')}</span>
          <Switch checked={allowDev} onChange={setAllowDev} aria-label={t('pages.groups.allowDev')} />
        </div>
        <div className="ge-toggle">
          <span className="ge-toggle__label">{t('pages.groups.allowCore')}</span>
          <Switch checked={allowCore} onChange={setAllowCore} aria-label={t('pages.groups.allowCore')} />
        </div>
      </div>
      </>
      )}

      {shows('members') && (
      <>
      <Divider>{t('pages.groups.inboundsInGroup', { defaultValue: 'Entrypoints in group' })}</Divider>
      {activeEntrypoints.length === 0 ? (
        <div className="ge-empty">{t('pages.groups.noInbounds', { defaultValue: 'No active entrypoints.' })}</div>
      ) : (
        <div className="ge-picks">
          {activeEntrypoints.map((e) => {
            const on = entrypointIds.includes(e.id);
            return (
              <Tag
                key={e.id}
                tone={on ? 'primary' : 'neutral'}
                className={`ge-pick${on ? ' is-on' : ''}`}
                onClick={() => toggleEntry(e.id)}
              >
                {e.tag?.trim() || String(e.id)}
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
            const on = members.includes(c.id);
            const taken = !on && !!c.group && c.group !== group?.tag;
            return (
              <Tag
                key={c.id}
                tone={on ? 'success' : 'neutral'}
                className={`ge-pick${on ? ' is-on' : ''}${taken ? ' is-taken' : ''}`}
                title={taken ? t('pages.groups.movesFrom', { defaultValue: 'Currently in {name}', name: c.group }) : undefined}
                onClick={() => toggleClient(c.id)}
              >
                {c.tag}
              </Tag>
            );
          })}
        </div>
      )}
      </>
      )}
      </div>
    </Dialog>
  );
}
