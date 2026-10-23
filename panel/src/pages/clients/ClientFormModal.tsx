import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';

import { Button, Dialog, Field, Input, Select, Switch, Tag } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { RandomUtil } from '@/utils';
import { DateTimePicker } from '@/components/form';
import type { ClientRecord } from '@/hooks/useClients';
import { ClientFormSchema, ClientCreateFormSchema } from '@/schemas/client';
import { useMediaQuery } from '@/hooks/useMediaQuery';

interface ApiMsg { success?: boolean }

interface ClientFormModalProps {
  open: boolean;
  client: ClientRecord | null;
  groups?: string[];
  save: (client: Record<string, unknown>, id?: number) => Promise<ApiMsg | null>;
  onOpenChange: (open: boolean) => void;
}

interface FormState {
  tag: string;
  uuid: string;
  group: string;
  comment: string;
  expiryDate: Dayjs | null;
  admin: boolean;
  deviceLimit: number;
  allowExit: number;
}

function emptyForm(): FormState {
  return { tag: '', uuid: '', group: '', comment: '', expiryDate: null, admin: false, deviceLimit: 0, allowExit: 0 };
}

export default function ClientFormModal({ open, client, groups = [], save, onOpenChange }: ClientFormModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const isEdit = client !== null;
  const { isMobile } = useMediaQuery();

  const [form, setForm] = useState<FormState>(emptyForm);
  const [submitting, setSubmitting] = useState(false);

  function update<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  useEffect(() => {
    if (!open) return;
    if (client) {
      setForm({
        tag: client.tag,
        uuid: client.uuid || '',
        group: client.group || '',
        comment: client.comment || '',
        expiryDate: client.expiryAt ? dayjs(client.expiryAt) : null,
        admin: !!client.admin,
        deviceLimit: client.deviceLimit || 0,
        allowExit: client.allowExit || 0,
      });
    } else {
      setForm({ ...emptyForm(), tag: RandomUtil.randomLowerAndNum(10), uuid: RandomUtil.randomUUID() });
    }
  }, [open, client]);

  async function onSubmit() {
    const validated = (isEdit ? ClientFormSchema : ClientCreateFormSchema).safeParse(form);
    if (!validated.success) {
      message.error(t(validated.error.issues[0]?.message ?? 'somethingWentWrong'));
      return;
    }
    const payload: Record<string, unknown> = {
      tag: form.tag.trim(),
      uuid: form.uuid.trim(),
      group: form.group,
      comment: form.comment,
      expiryAt: form.expiryDate ? form.expiryDate.valueOf() : 0,
      admin: form.admin,
      deviceLimit: form.deviceLimit,
      allowExit: form.allowExit,
    };
    if (!isEdit) payload.enable = true;

    setSubmitting(true);
    try {
      const msg = await save(payload, client?.id);
      if (msg?.success) onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  }

  const tagField = <Input value={form.tag} onChange={(e) => update('tag', e.target.value)} />;
  const uuidField = (
    <div className="cf-regen">
      <Input value={form.uuid} onChange={(e) => update('uuid', e.target.value)} />
      <Button icon={<ReloadOutlined />} onClick={() => update('uuid', RandomUtil.randomUUID())} />
    </div>
  );
  const groupPick = (
    <Select
      value={form.group}
      onChange={(v) => update('group', v)}
      options={[
        { value: '', label: t('pages.clients.noGroup', { defaultValue: 'No group' }) },
        ...groups.map((g) => ({ value: g, label: g })),
      ]}
    />
  );
  const exitPick = (
    <Select
      value={String(form.allowExit)}
      onChange={(v) => update('allowExit', Number(v))}
      options={[
        { value: '0', label: t('pages.clients.exitInherit', { defaultValue: 'As the group allows' }) },
        { value: '1', label: t('pages.clients.exitAllow', { defaultValue: 'Always allowed' }) },
        { value: '2', label: t('pages.clients.exitDeny', { defaultValue: 'Never allowed' }) },
      ]}
    />
  );
  const expiryField = <DateTimePicker value={form.expiryDate} onChange={(d) => update('expiryDate', d || null)} />;
  const commentField = (
    <Input
      value={form.comment}
      disabled={!form.expiryDate}
      placeholder={form.expiryDate
        ? t('pages.clients.commentPlaceholder', { defaultValue: 'Why this date' })
        : t('pages.clients.commentDisabled', { defaultValue: 'Set an expiry date first' })}
      onChange={(e) => update('comment', e.target.value)}
    />
  );
  const limitField = (
    <Input
      type="number"
      min={0}
      value={form.deviceLimit}
      onChange={(e) => update('deviceLimit', Math.max(0, Number(e.target.value) || 0))}
    />
  );
  const adminLabel = t('pages.clients.admin', { defaultValue: 'Administrator' });

  if (isMobile) {
    return (
      <Dialog
        open={open}
        onOpenChange={(o) => { if (!o && !submitting) onOpenChange(false); }}
        title={isEdit ? (
          <span className="ms-title">
            <small>{t('pages.clients.editClient', { defaultValue: 'Edit client' })}</small>
            <span>{client?.tag}</span>
          </span>
        ) : t('pages.clients.addClient')}
        confirmLoading={submitting}
        autoHeight
        footer={(
          <div className="ms-actions">
            <Button onClick={() => onOpenChange(false)}>{t('cancel')}</Button>
            <Button variant="primary" loading={submitting} onClick={onSubmit}>{isEdit ? t('save') : t('create')}</Button>
          </div>
        )}
      >
        <div className="mf">
          <Field label={t('pages.clients.tag', { defaultValue: 'Tag' })}>{tagField}</Field>
          <Field label={t('pages.clients.uuid', { defaultValue: 'UUID' })}>{uuidField}</Field>
          <Field label={t('pages.clients.group')}>{groupPick}</Field>
          <Field label={t('pages.clients.allowExit', { defaultValue: 'Exit nodes' })}>{exitPick}</Field>
          <Field label={t('pages.clients.deviceLimit', { defaultValue: 'Device limit' })}>{limitField}</Field>
          <Field label={t('pages.clients.expiryTime')}>{expiryField}</Field>
          <Field label={t('pages.clients.comment')}>{commentField}</Field>
          <label className="mf-switch" htmlFor="cf-admin-m">
            <span>{adminLabel}</span>
            <Switch id="cf-admin-m" checked={form.admin} onChange={(v) => update('admin', v)} aria-label={adminLabel} />
          </label>
        </div>
      </Dialog>
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o && !submitting) onOpenChange(false); }}
      title={(
        <div className="cf-title">
          <span className="ef-title">
            {isEdit ? (
              <>
                {t('edit', { defaultValue: 'Edit' })}
                <Tag tone="success" className="ef-title__node">{form.tag || '—'}</Tag>
                {t('pages.clients.clientWord', { defaultValue: 'client' })}
              </>
            ) : t('pages.clients.addClient')}
          </span>
          <div className="cf-switch">
            <Switch id="cf-admin" checked={form.admin} onChange={(v) => update('admin', v)} aria-label={adminLabel} />
            <label htmlFor="cf-admin">{adminLabel}</label>
          </div>
        </div>
      )}
      confirmLoading={submitting}
      width={720}
      autoHeight
      footer={
        <div className="cf-foot">
          <div className="cf-foot__group">{groupPick}</div>
          <span className="cf-foot__actions">
            <Button onClick={() => onOpenChange(false)}>{t('cancel')}</Button>
            <Button variant="primary" loading={submitting} onClick={onSubmit}>
              {isEdit ? t('save') : t('create')}
            </Button>
          </span>
        </div>
      }
    >
      <div className="cf-form">
        <div className="cf-pair">
          <Field label={t('pages.clients.tag', { defaultValue: 'Tag' })}>{tagField}</Field>
          <Field label={t('pages.clients.uuid', { defaultValue: 'UUID' })}>{uuidField}</Field>
        </div>
        <div className="cf-pair">
          <Field label={t('pages.clients.expiryTime')}>{expiryField}</Field>
          <Field label={t('pages.clients.comment')}>{commentField}</Field>
        </div>
        <div className="cf-pair">
          <Field label={t('pages.clients.deviceLimit', { defaultValue: 'Device limit' })}>{limitField}</Field>
          <Field label={t('pages.clients.allowExit', { defaultValue: 'Exit nodes' })}>{exitPick}</Field>
        </div>
      </div>
    </Dialog>
  );
}
