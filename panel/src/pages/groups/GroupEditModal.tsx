import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PlusOutlined } from '@ant-design/icons';

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

type RelayRow = { nodeId: number; weblink: string };

const RELAYS_PER_NODE = 4;

function docOf(weblink: string): string {
  const at = weblink.indexOf('/public/');
  return (at >= 0 ? weblink.slice(at + '/public/'.length) : weblink).trim().replace(/\/+$/, '');
}

function relayKey(rows: RelayRow[]): string {
  return rows.map((r) => `${r.nodeId}|${docOf(r.weblink)}`).sort().join('\n');
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
  const [relayEnable, setRelayEnable] = useState(false);
  const [relays, setRelays] = useState<Record<number, string[]>>({});
  const [saving, setSaving] = useState(false);
  const [touched, setTouched] = useState(false);

  const activeInbounds = useMemo(() => inbounds.filter((i) => i.enable !== false), [inbounds]);

  const nodeOf = useMemo(() => {
    const m = new Map<number, number>();
    inbounds.forEach((ib) => { if (ib.nodeId != null) m.set(ib.id, ib.nodeId); });
    return m;
  }, [inbounds]);
  const relayNodes = useMemo(() => {
    const set = new Set<number>();
    entrypointIds.forEach((id) => { const n = nodeOf.get(id); if (n != null) set.add(n); });
    return [...set];
  }, [entrypointIds, nodeOf]);
  const nodeLabel = (nid: number) => {
    const ib = inbounds.find((i) => i.nodeId === nid);
    return ib ? entryLabel(ib) : `#${nid}`;
  };
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
    setRelayEnable(!!(group as { relayEnable?: boolean }).relayEnable);
    const seed: Record<number, string[]> = {};
    (group.relays ?? []).forEach((r) => { (seed[r.nodeId] ??= []).push(r.weblink); });
    setRelays(seed);
    setTouched(false);
  });

  useEffect(() => {
    if (!open || touched) return;
    setEmails(originalEmails);
  }, [open, touched, originalEmails]);

  function toggleEntry(id: number) {
    setEntrypointIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  const rowsOf = (nid: number) => relays[nid] ?? [''];

  function setRow(nid: number, at: number, value: string) {
    setRelays((prev) => ({ ...prev, [nid]: (prev[nid] ?? ['']).map((w, i) => (i === at ? value : w)) }));
  }

  function addRow(nid: number) {
    setRelays((prev) => ({ ...prev, [nid]: [...(prev[nid] ?? ['']), ''] }));
  }

  function dropRow(nid: number, at: number) {
    setRelays((prev) => ({ ...prev, [nid]: (prev[nid] ?? ['']).filter((_, i) => i !== at) }));
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

    const relayList: RelayRow[] = relayNodes.flatMap((nid) => (relays[nid] ?? [])
      .map((w) => w.trim())
      .filter((w) => w !== '')
      .map((weblink) => ({ nodeId: nid, weblink })));

    const seen = new Set<string>();
    for (const r of relayList) {
      const doc = docOf(r.weblink);
      if (seen.has(doc)) {
        message.error(t('pages.groups.relayTwice', { defaultValue: 'The same relay link is entered twice: {link}', link: r.weblink }));
        return;
      }
      seen.add(doc);
      const other = groups.find((g) => g.name !== group.name
        && (g.relays ?? []).some((o) => docOf(o.weblink) === doc && o.nodeId !== r.nodeId));
      if (other) {
        message.error(t('pages.groups.relayTaken', { defaultValue: 'This relay link already serves another node in group {name}', name: other.name }));
        return;
      }
    }

    setSaving(true);
    try {
      if (nextName !== group.name) {
        const msg = await clientsApi.groupsRename({ oldName: group.name, newName: nextName }, { silent: true });
        if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      }

      const relaysDiffer = relayKey(relayList) !== relayKey(group.relays ?? []);

      if (!sameSet(entrypointIds, group.entrypointIds ?? [])
          || deviceLimit !== (Number((group as { deviceLimit?: number }).deviceLimit) || 0)
          || allowExit !== !!(group as { allowExit?: boolean }).allowExit
          || relayEnable !== !!(group as { relayEnable?: boolean }).relayEnable
          || relaysDiffer) {
        const msg = await clientsApi.groupsEntrypoints(
          { name: nextName, entrypointIds, deviceLimit, allowExit, relayEnable, relays: relayList }, { silent: true });
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
          <span className="ge-toggle__label">
            {t('pages.groups.relayEnable', { defaultValue: 'Relay fallback' })}
          </span>
          <Switch
            checked={relayEnable}
            onChange={setRelayEnable}
            aria-label={t('pages.groups.relayEnable', { defaultValue: 'Relay fallback' })}
          />
        </div>
      </div>

      {relayEnable && (
        relayNodes.length === 0 ? (
          <div className="ge-empty">{t('pages.groups.relayNoNodes', { defaultValue: 'Pick inbounds first — relay links are set per ingress.' })}</div>
        ) : (
          <div className="ge-relays">
            {relayNodes.map((nid) => {
              const rows = rowsOf(nid);
              return (
                <div key={nid} className="ge-relay-node">
                  <div className="ge-relay-node__head">
                    <span className="ds-field__label">{nodeLabel(nid)}</span>
                    {rows.length < RELAYS_PER_NODE && (
                      <Button size="sm" icon={<PlusOutlined />} onClick={() => addRow(nid)}>
                        {t('pages.groups.addRelay', { defaultValue: 'Add Relay' })}
                      </Button>
                    )}
                  </div>
                  {rows.map((w, at) => (
                    <div key={at} className="ge-relay-row">
                      <Input
                        value={w}
                        onChange={(e) => setRow(nid, at, e.target.value)}
                        placeholder={t('pages.groups.relayLink', { defaultValue: 'public document link id' })}
                      />
                      <Button danger onClick={() => dropRow(nid, at)}>{t('delete')}</Button>
                    </div>
                  ))}
                </div>
              );
            })}
          </div>
        )
      )}

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
