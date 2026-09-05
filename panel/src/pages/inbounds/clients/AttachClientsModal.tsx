import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, DataTable, Dialog, Field, Input, Tag, type ColumnDef } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { clientsApi } from '@/generated/client';
import { coerceInboundJsonField, type DBInbound } from '@/models/dbinbound';
import { isInboundMultiUser } from '../list';

interface AttachClientsModalProps {
  open: boolean;
  source: DBInbound | null;
  dbInbounds: DBInbound[];
  onClose: () => void;
  onAttached?: () => void;
}

interface BulkAttachResult { attached?: string[]; skipped?: string[]; errors?: string[] }
interface ClientRow { email: string; comment: string; enable: boolean }

function readClientRows(settings: unknown): ClientRow[] {
  const parsed = coerceInboundJsonField(settings) as { clients?: Array<{ email?: string; comment?: string; enable?: boolean }> };
  const clients = Array.isArray(parsed?.clients) ? parsed.clients : [];
  return clients
    .map((c) => ({ email: (c?.email || '').trim(), comment: (c?.comment || '').trim(), enable: c?.enable !== false }))
    .filter((r) => r.email);
}

export default function AttachClientsModal({ open, source, dbInbounds, onClose, onAttached }: AttachClientsModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const [targetIds, setTargetIds] = useState<number[]>([]);
  const [saving, setSaving] = useState(false);
  const [clientRows, setClientRows] = useState<ClientRow[]>([]);
  const [selectedEmails, setSelectedEmails] = useState<string[]>([]);
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (!open) return;
    const rows = source ? readClientRows(source.settings) : [];
    setClientRows(rows);
    setSelectedEmails(rows.map((r) => r.email));
    setTargetIds([]);
    setSearch('');
  }, [open, source]);

  const targetOptions = useMemo(() => {
    if (!source) return [];
    return (dbInbounds || [])
      .filter((ib) => ib.id !== source.id && isInboundMultiUser(ib))
      .map((ib) => ({ value: ib.id, label: ib.remark?.trim() || ib.tag || '' }));
  }, [dbInbounds, source]);

  const filteredRows = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return clientRows;
    return clientRows.filter((r) => r.email.toLowerCase().includes(q) || r.comment.toLowerCase().includes(q));
  }, [clientRows, search]);

  const columns = useMemo<ColumnDef<ClientRow, unknown>[]>(() => [
    { id: 'email', header: () => t('pages.inbounds.email'), cell: ({ row }) => row.original.email },
    { id: 'comment', header: () => t('comment'), cell: ({ row }) => row.original.comment },
    {
      id: 'enable', size: 100, header: () => t('enable'),
      cell: ({ row }) => (row.original.enable ? <Tag tone="success">{t('enable')}</Tag> : <Tag>{t('pages.inbounds.attachClientsStatusDisabled')}</Tag>),
    },
  ], [t]);

  function toggleTarget(id: number) {
    setTargetIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  async function submit() {
    if (!source || targetIds.length === 0 || selectedEmails.length === 0) return;
    setSaving(true);
    try {
      const msg = await clientsApi.bulkAttach<BulkAttachResult>({ emails: selectedEmails, inboundIds: targetIds }, { silent: true });
      if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      const result = msg.obj || {};
      const attached = result.attached?.length ?? 0;
      const skipped = result.skipped?.length ?? 0;
      const errors = result.errors?.length ?? 0;
      if (errors > 0) message.warning(t('pages.inbounds.attachClientsResultMixed', { attached, skipped, errors }));
      else message.success(t('pages.inbounds.attachClientsResult', { attached, skipped }));
      onAttached?.();
      onClose();
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('pages.inbounds.attachClientsTitle', { remark: source?.remark?.trim() || source?.tag || '' })}
      okText={t('pages.inbounds.attachClients')}
      okDisabled={targetIds.length === 0 || selectedEmails.length === 0}
      confirmLoading={saving}
      width={680}
      onOk={submit}
    >
      <p className="ds-muted" style={{ marginTop: 0 }}>{t('pages.inbounds.attachClientsDesc', { count: clientRows.length })}</p>

      <div className="ds-toolbar" style={{ marginBottom: 12 }}>
        <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('pages.inbounds.attachClientsSearchPlaceholder')} style={{ maxWidth: 320 }} />
        <span className="ds-muted">{t('pages.inbounds.attachClientsSelectedCount', { selected: selectedEmails.length, total: clientRows.length })}</span>
      </div>
      <div style={{ maxHeight: 280, overflowY: 'auto', marginBottom: 12 }}>
        <DataTable
          data={filteredRows}
          columns={columns}
          getRowId={(r) => r.email}
          sortable={false}
          rowSelection={{ selectedIds: selectedEmails, onChange: setSelectedEmails }}
          empty={t('noData')}
        />
      </div>

      <Field label={t('pages.inbounds.attachClientsTargets')}>
        {targetOptions.length === 0 ? (
          <Alert tone="info" title={t('pages.inbounds.attachClientsNoTargets')} />
        ) : (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {targetOptions.map((o) => (
              <Tag key={o.value} tone={targetIds.includes(o.value) ? 'primary' : 'neutral'} onClick={() => toggleTarget(o.value)} style={{ cursor: 'pointer' }}>{o.label}</Tag>
            ))}
          </div>
        )}
      </Field>
    </Dialog>
  );
}
