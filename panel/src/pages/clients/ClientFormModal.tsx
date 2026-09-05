import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';

import { Button, Dialog, Field, Input, Select, Switch, Tag } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { RandomUtil } from '@/utils';
import { DateTimePicker } from '@/components/form';
import type { ClientRecord, InboundOption } from '@/hooks/useClients';
import { ClientFormSchema, ClientCreateFormSchema } from '@/schemas/client';


interface ApiMsg<T = unknown> { success?: boolean; obj?: T }
type Mode = 'add' | 'edit';
interface SaveMetaEdit { isEdit: true; email: string; attach: number[]; detach: number[] }
interface SaveMetaCreate { isEdit: false }
interface SaveCreatePayload { client: Record<string, unknown>; inboundIds: number[] }

interface ClientFormModalProps {
  open: boolean;
  mode: Mode;
  client: ClientRecord | null;
  inbounds: InboundOption[];
  attachedIds?: number[];
  tgBotEnable?: boolean;
  groups?: string[];
  save: (payload: Record<string, unknown> | SaveCreatePayload, meta: SaveMetaEdit | SaveMetaCreate) => Promise<ApiMsg | null>;
  onOpenChange: (open: boolean) => void;
}

interface FormState {
  email: string; subId: string; uuid: string; password: string; auth: string;
  flow: string; security: string; reverseTag: string; totalGB: number;
  expiryDate: Dayjs | null; delayedStart: boolean; delayedDays: number;
  reset: number; tgId: number; group: string; comment: string;
  enable: boolean; admin: boolean; limitIp: number; allowExit: number; inboundIds: number[];
}

function emptyForm(): FormState {
  return {
    email: '', subId: '', uuid: '', password: '', auth: '', flow: '', security: 'auto',
    reverseTag: '', totalGB: 0, expiryDate: null, delayedStart: false, delayedDays: 0,
    reset: 0, tgId: 0, group: '', comment: '', enable: true, admin: false, limitIp: 0, allowExit: 0, inboundIds: [],
  };
}

function bytesToGB(bytes: number): number {
  if (!bytes || bytes <= 0) return 0;
  return Math.round((bytes / (1024 * 1024 * 1024)) * 100) / 100;
}
function gbToBytes(gb: number): number {
  if (!gb || gb <= 0) return 0;
  return Math.round(gb * 1024 * 1024 * 1024);
}

export default function ClientFormModal({
  open,
  mode,
  client,
  inbounds,
  attachedIds = [],
  groups = [],
  save,
  onOpenChange,
}: ClientFormModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const isEdit = mode === 'edit';

  const [form, setForm] = useState<FormState>(emptyForm);
  const [groupDraft, setGroupDraft] = useState('');
  const [submitting, setSubmitting] = useState(false);

  function update<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  useEffect(() => {
    if (!open) return;
    if (isEdit && client) {
      const et = Number(client.expiryTime) || 0;
      const next: FormState = {
        ...emptyForm(),
        email: client.email || '', subId: client.subId || '', uuid: client.uuid || '',
        password: client.password || '', auth: client.auth || '', flow: client.flow || '',
        security: client.security || 'auto', reverseTag: client.reverse?.tag || '',
        totalGB: bytesToGB(client.totalGB || 0), reset: Number(client.reset) || 0,
        tgId: Number(client.tgId) || 0, group: client.group || '',
        comment: client.comment || '', enable: !!client.enable, admin: !!(client as { admin?: boolean }).admin,
        limitIp: Number(client.limitIp) || 0,
        allowExit: Number((client as { allowExit?: number }).allowExit) || 0,
        inboundIds: Array.isArray(attachedIds) ? [...attachedIds] : [],
      };
      if (et < 0) { next.delayedStart = true; next.delayedDays = Math.round(et / -86400000); next.expiryDate = null; }
      else { next.delayedStart = false; next.delayedDays = 0; next.expiryDate = et > 0 ? dayjs(et) : null; }
      setForm(next);
      setGroupDraft(next.group);
    } else {
      setForm({
        ...emptyForm(),
        email: RandomUtil.randomLowerAndNum(10),
        uuid: RandomUtil.randomUUID(),
        subId: RandomUtil.randomUUID(),
      });
      setGroupDraft('');
    }
  }, [open, isEdit]);

  const flowCapableIds = useMemo(() => { const s = new Set<number>(); for (const r of inbounds || []) if (r?.tlsFlowCapable) s.add(r.id); return s; }, [inbounds]);
  const vlessLikeIds = useMemo(() => { const s = new Set<number>(); for (const r of inbounds || []) if (r?.protocol === 'vless') s.add(r.id); return s; }, [inbounds]);
  const vmessIds = useMemo(() => { const s = new Set<number>(); for (const r of inbounds || []) if (r?.protocol === 'vmess') s.add(r.id); return s; }, [inbounds]);

  const showFlow = useMemo(() => (form.inboundIds || []).some((id) => flowCapableIds.has(id)), [form.inboundIds, flowCapableIds]);
  const showReverseTag = useMemo(() => (form.inboundIds || []).some((id) => vlessLikeIds.has(id)), [form.inboundIds, vlessLikeIds]);
  const showSecurity = useMemo(() => (form.inboundIds || []).some((id) => vmessIds.has(id)), [form.inboundIds, vmessIds]);

  useEffect(() => { if (!showFlow && form.flow) update('flow', ''); }, [showFlow, form.flow]);
  useEffect(() => { if (!showReverseTag && form.reverseTag) update('reverseTag', ''); }, [showReverseTag, form.reverseTag]);

  async function onSubmit() {
    const schema = isEdit ? ClientFormSchema : ClientCreateFormSchema;
    const validated = schema.safeParse({
      email: form.email, subId: form.subId, uuid: form.uuid, password: form.password, auth: form.auth,
      flow: form.flow, security: form.security, reverseTag: form.reverseTag, totalGB: form.totalGB,
      delayedStart: form.delayedStart, delayedDays: form.delayedDays, reset: form.reset,
      tgId: form.tgId, group: form.group, comment: form.comment, enable: form.enable, admin: form.admin, limitIp: form.limitIp, allowExit: form.allowExit, inboundIds: form.inboundIds,
    });
    if (!validated.success) {
      message.error(t(validated.error.issues[0]?.message ?? 'somethingWentWrong'));
      return;
    }
    const expiryTime = form.delayedStart ? -86400000 * (Number(form.delayedDays) || 0) : (form.expiryDate ? form.expiryDate.valueOf() : 0);
    const clientPayload: Record<string, unknown> = {
      email: form.email.trim(), subId: form.subId, id: form.uuid, password: form.password, auth: form.auth,
      flow: showFlow ? (form.flow || '') : '', security: showSecurity ? (form.security || 'auto') : 'auto',
      totalGB: gbToBytes(form.totalGB), expiryTime, reset: Number(form.reset) || 0,
      tgId: Number(form.tgId) || 0, group: form.group, comment: form.comment, enable: !!form.enable, admin: !!form.admin,
      limitIp: Number(form.limitIp) || 0, allowExit: Number(form.allowExit) || 0,
    };
    const reverseTag = showReverseTag ? (form.reverseTag || '').trim() : '';
    if (reverseTag) clientPayload.reverse = { tag: reverseTag };

    setSubmitting(true);
    try {
      let msg;
      if (isEdit && client) {
        const original = new Set(attachedIds || []);
        const next = new Set(form.inboundIds || []);
        const toAttach = [...next].filter((id) => !original.has(id));
        const toDetach = [...original].filter((id) => !next.has(id));
        msg = await save(clientPayload, { isEdit: true, email: client.email, attach: toAttach, detach: toDetach });
      } else {
        msg = await save({ client: clientPayload, inboundIds: form.inboundIds }, { isEdit: false });
      }
      if (msg?.success) onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  }

  const reloadInput = (value: string, onChange: (v: string) => void, regen: () => string) => (
    <div className="cf-regen">
      <Input value={value} onChange={(e) => onChange(e.target.value)} />
      <Button icon={<ReloadOutlined />} onClick={() => onChange(regen())} />
    </div>
  );

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => { if (!o && !submitting) onOpenChange(false); }}
        title={(
          <div className="cf-title">
            <span className="ef-title">
              {isEdit ? (
                <>
                  {t('edit', { defaultValue: 'Edit' })}
                  <Tag tone="success" className="ef-title__node">{form.email || '—'}</Tag>
                  {t('pages.clients.clientWord', { defaultValue: 'client' })}
                </>
              ) : t('pages.clients.addClient')}
            </span>
            <div className="cf-switch">
              <Switch
                id="cf-admin"
                checked={form.admin}
                onChange={(v) => update('admin', v)}
                aria-label={t('pages.clients.admin', { defaultValue: 'Administrator' })}
              />
              <label htmlFor="cf-admin">{t('pages.clients.admin', { defaultValue: 'Administrator' })}</label>
            </div>
          </div>
        )}
        confirmLoading={submitting}
        width={720}
        autoHeight
        footer={
          <div className="cf-foot">
            <div className="cf-foot__group">
              <Select
                value={groupDraft}
                onChange={(v) => setGroupDraft(v)}
                options={[
                  { value: '', label: t('pages.clients.noGroup', { defaultValue: 'No group' }) },
                  ...groups.map((g) => ({ value: g, label: g })),
                ]}
              />
            </div>
            {groupDraft !== form.group && (
              <Tag
                tone="primary"
                style={{ cursor: 'pointer' }}
                onClick={() => update('group', groupDraft)}
              >
                {groupDraft
                  ? t('pages.clients.confirmGroup', {
                      defaultValue: 'confirm {name} assignment',
                      name: groupDraft,
                    })
                  : t('pages.clients.confirmGroupRemoval', {
                      defaultValue: 'confirm group removal',
                    })}
              </Tag>
            )}
            <span className="cf-foot__actions">
              <Button onClick={() => onOpenChange(false)}>{t('cancel')}</Button>
              <Button variant="primary" loading={submitting} onClick={onSubmit}>
                {isEdit ? t('save') : t('create')}
              </Button>
            </span>
          </div>
        }
      >
        {/* One grid of equal halves throughout: mixed 'auto 1fr' templates made
            every row a different shape and squeezed the longer labels. */}
        <div className="cf-form">
          <div className="cf-pair">
            <Field label={t('pages.clients.tag', { defaultValue: 'Tag' })}>
              <Input value={form.email} onChange={(e) => update('email', e.target.value)} />
            </Field>
            <Field label={t('pages.clients.uuid', { defaultValue: 'UUID' })}>
              {reloadInput(form.subId, (v) => update('subId', v), () => RandomUtil.randomUUID())}
            </Field>
          </div>

          <div className="cf-pair">
            <Field label={t('pages.clients.expiryTime')}>
              <DateTimePicker value={form.expiryDate} onChange={(d) => update('expiryDate', d || null)} />
            </Field>
            <Field label={t('pages.clients.comment')}>
              <Input
                value={form.comment}
                disabled={!form.expiryDate}
                placeholder={form.expiryDate
                  ? t('pages.clients.commentPlaceholder', { defaultValue: 'Why this date' })
                  : t('pages.clients.commentDisabled', { defaultValue: 'Set an expiry date first' })}
                onChange={(e) => update('comment', e.target.value)}
              />
            </Field>
          </div>

          <div className="cf-pair">
            <Field label={t('pages.clients.deviceLimit', { defaultValue: 'Device limit' })}>
              <Input
                type="number"
                min={0}
                value={form.limitIp}
                onChange={(e) => update('limitIp', Math.max(0, Number(e.target.value) || 0))}
              />
            </Field>
            <Field label={t('pages.clients.allowExit', { defaultValue: 'Exit nodes' })}>
              <Select
                value={String(form.allowExit)}
                onChange={(v) => update('allowExit', Number(v))}
                options={[
                  { value: '0', label: t('pages.clients.exitInherit', { defaultValue: 'As the group allows' }) },
                  { value: '1', label: t('pages.clients.exitAllow', { defaultValue: 'Always allowed' }) },
                  { value: '2', label: t('pages.clients.exitDeny', { defaultValue: 'Never allowed' }) },
                ]}
              />
            </Field>
          </div>

        </div>
      </Dialog>

    </>
  );
}
