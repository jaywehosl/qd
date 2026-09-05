import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DataTable, Dialog, Input, Tag, type ColumnDef } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { clientsApi } from '@/generated/client';
import { coerceInboundJsonField, type DBInbound } from '@/models/dbinbound';

interface DetachClientsModalProps {
  open: boolean;
  source: DBInbound | null;
  onClose: () => void;
  onDetached?: () => void;
}

interface BulkDetachResult {
  detached?: string[];
  skipped?: string[];
  errors?: string[];
}

interface ClientRow {
  email: string;
  comment: string;
  enable: boolean;
}

function readClientRows(settings: unknown): ClientRow[] {
  const parsed = coerceInboundJsonField(settings) as { clients?: Array<{ email?: string; comment?: string; enable?: boolean }> };
  const clients = Array.isArray(parsed?.clients) ? parsed.clients : [];
  return clients
    .map((c) => ({ email: (c?.email || '').trim(), comment: (c?.comment || '').trim(), enable: c?.enable !== false }))
    .filter((r) => r.email);
}

export default function DetachClientsModal({ open, source, onClose, onDetached }: DetachClientsModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const [saving, setSaving] = useState(false);
  const [clientRows, setClientRows] = useState<ClientRow[]>([]);
  const [selectedEmails, setSelectedEmails] = useState<string[]>([]);
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (!open) return;
    setClientRows(source ? readClientRows(source.settings) : []);
    setSelectedEmails([]);
    setSearch('');
  }, [open, source]);

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

  async function submit() {
    if (!source || selectedEmails.length === 0) return;
    setSaving(true);
    try {
      const msg = await clientsApi.bulkDetach<BulkDetachResult>({ emails: selectedEmails, inboundIds: [source.id] }, { silent: true });
      if (!msg?.success) { message.error(msg?.msg || t('somethingWentWrong')); return; }
      const result = msg.obj || {};
      const detached = result.detached?.length ?? 0;
      const skipped = result.skipped?.length ?? 0;
      const errors = result.errors?.length ?? 0;
      if (errors > 0) message.warning(t('pages.inbounds.detachClientsResultMixed', { detached, skipped, errors }));
      else message.success(t('pages.inbounds.detachClientsResult', { detached, skipped }));
      onDetached?.();
      onClose();
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('pages.inbounds.detachClientsTitle', { remark: source?.tag ?? '' })}
      okText={t('pages.inbounds.detachClients')}
      okDanger
      okDisabled={selectedEmails.length === 0}
      confirmLoading={saving}
      width={680}
      onOk={submit}
    >
      <p className="ds-muted" style={{ marginTop: 0 }}>{t('pages.inbounds.detachClientsDesc', { count: clientRows.length })}</p>
      <div className="ds-toolbar" style={{ marginBottom: 12 }}>
        <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('pages.inbounds.attachClientsSearchPlaceholder')} style={{ maxWidth: 320 }} />
        <span className="ds-muted">{t('pages.inbounds.attachClientsSelectedCount', { selected: selectedEmails.length, total: clientRows.length })}</span>
      </div>
      <div style={{ maxHeight: 320, overflowY: 'auto' }}>
        <DataTable
          data={filteredRows}
          columns={columns}
          getRowId={(r) => r.email}
          sortable={false}
          rowSelection={{ selectedIds: selectedEmails, onChange: setSelectedEmails }}
          empty={t('noData')}
        />
      </div>
    </Dialog>
  );
}
