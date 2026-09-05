import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, DataTable, Dialog, Input, Select, Tag, type ColumnDef } from '@/components/ds';
import { Spin } from '@/components/ui';
import { getMessage } from '@/utils/messageBus';
import { clientsApi } from '@/generated/client';
import type { DBInbound } from '@/models/dbinbound';

interface AttachExistingClientsModalProps {
  open: boolean;
  target: DBInbound | null;
  onClose: () => void;
  onAttached?: () => void;
}

interface BulkAttachResult { attached?: string[]; skipped?: string[]; errors?: string[] }
interface ClientRow { email: string; group: string; enable: boolean; alreadyAttached: boolean }
interface RawClient { email?: string; group?: string; enable?: boolean; inboundIds?: number[] | null }

export default function AttachExistingClientsModal({ open, target, onClose, onAttached }: AttachExistingClientsModalProps) {
  const { t } = useTranslation();
  const message = getMessage();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [clientRows, setClientRows] = useState<ClientRow[]>([]);
  const [selectedEmails, setSelectedEmails] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const [groupFilter, setGroupFilter] = useState('');

  useEffect(() => {
    if (!open || !target) return;
    let cancelled = false;
    setLoading(true);
    setSearch('');
    setGroupFilter('');
    clientsApi.list<RawClient[]>(undefined, { silent: true })
      .then((msg) => {
        if (cancelled) return;
        const list = Array.isArray(msg?.obj) ? msg.obj : [];
        const rows: ClientRow[] = list
          .map((c) => ({
            email: (c?.email || '').trim(),
            group: (c?.group || '').trim(),
            enable: c?.enable !== false,
            alreadyAttached: Array.isArray(c?.inboundIds) && c.inboundIds.includes(target.id),
          }))
          .filter((r) => r.email);
        setClientRows(rows);
        setSelectedEmails(rows.filter((r) => !r.alreadyAttached).map((r) => r.email));
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [open, target]);

  const groupOptions = useMemo(() => {
    const set = new Set<string>();
    for (const r of clientRows) if (r.group) set.add(r.group);
    return [{ value: '', label: t('pages.clients.group') }, ...[...set].sort((a, b) => a.localeCompare(b)).map((g) => ({ value: g, label: g }))];
  }, [clientRows, t]);

  const attachableCount = useMemo(() => clientRows.filter((r) => !r.alreadyAttached).length, [clientRows]);

  const filteredRows = useMemo(() => {
    const q = search.trim().toLowerCase();
    return clientRows.filter((r) => {
      if (groupFilter && r.group !== groupFilter) return false;
      if (!q) return true;
      return r.email.toLowerCase().includes(q) || r.group.toLowerCase().includes(q);
    });
  }, [clientRows, search, groupFilter]);

  const columns = useMemo<ColumnDef<ClientRow, unknown>[]>(() => [
    { id: 'email', header: () => t('pages.inbounds.email'), cell: ({ row }) => row.original.email },
    {
      id: 'group', size: 150, header: () => t('pages.clients.group'),
      cell: ({ row }) => (row.original.group ? <Tag tone="primary">{row.original.group}</Tag> : <span className="ds-muted">—</span>),
    },
    {
      id: 'status', size: 150, header: () => t('enable'),
      cell: ({ row }) => {
        if (row.original.alreadyAttached) return <Tag>{t('pages.inbounds.attachExistingStatusAttached')}</Tag>;
        return row.original.enable ? <Tag tone="success">{t('enable')}</Tag> : <Tag>{t('pages.inbounds.attachClientsStatusDisabled')}</Tag>;
      },
    },
  ], [t]);

  async function submit() {
    if (!target || selectedEmails.length === 0) return;
    setSaving(true);
    try {
      const msg = await clientsApi.bulkAttach<BulkAttachResult>({ emails: selectedEmails, inboundIds: [target.id] }, { silent: true });
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

  const noClients = !loading && clientRows.length === 0;

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('pages.inbounds.attachExistingTitle', { remark: target?.remark?.trim() || target?.tag || '' })}
      okText={t('pages.inbounds.attachClients')}
      okDisabled={selectedEmails.length === 0}
      confirmLoading={saving}
      width={680}
      onOk={submit}
    >
      <p className="ds-muted" style={{ marginTop: 0 }}>{t('pages.inbounds.attachExistingDesc', { count: attachableCount })}</p>

      {noClients ? (
        <Alert tone="info" title={t('pages.inbounds.attachExistingNoClients')} />
      ) : (
        <Spin spinning={loading}>
          <div className="ds-toolbar" style={{ marginBottom: 12 }}>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('pages.inbounds.attachClientsSearchPlaceholder')} style={{ width: 260 }} />
              {groupOptions.length > 1 && (
                <div style={{ minWidth: 160 }}>
                  <Select value={groupFilter} onChange={setGroupFilter} options={groupOptions} placeholder={t('pages.clients.group')} />
                </div>
              )}
            </div>
            <span className="ds-muted">{t('pages.inbounds.attachClientsSelectedCount', { selected: selectedEmails.length, total: attachableCount })}</span>
          </div>
          <div style={{ maxHeight: 280, overflowY: 'auto' }}>
            <DataTable
              data={filteredRows}
              columns={columns}
              getRowId={(r) => r.email}
              sortable={false}
              rowSelection={{ selectedIds: selectedEmails, onChange: setSelectedEmails, isSelectable: (r) => !r.alreadyAttached }}
              empty={t('noData')}
            />
          </div>
        </Spin>
      )}
    </Dialog>
  );
}
