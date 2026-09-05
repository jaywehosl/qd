import { lazy, useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';
import {
  DeleteOutlined,
  EditOutlined,
  InboxOutlined,
  PlusOutlined,
  TagsOutlined,
  TeamOutlined,
} from '@ant-design/icons';

import {
  Button,
  Card,
  DataTable,
  Dialog,
  Field,
  Input,
  Stat,
  Tag,
  Tooltip,
  TooltipProvider,
  type ColumnDef,
} from '@/components/ds';
import { useTheme } from '@/hooks/useTheme';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useClients } from '@/hooks/useClients';
import { clientsApi } from '@/generated/client';
import { keys } from '@/api/queryKeys';
import { getMessage } from '@/utils/messageBus';
import { LazyMount } from '@/components/utility';
import {
  ClientRecordSchema,
  GroupSummaryListSchema,
  type ClientRecord,
  type GroupSummary,
} from '@/schemas/client';
import { parseMsg } from '@/utils/zodValidate';
import { useMediaQuery } from '@/hooks/useMediaQuery';
const ClientRecordListSchema = z.array(ClientRecordSchema).nullable().transform((v) => v ?? []);

const GroupEditModal = lazy(() => import('./GroupEditModal'));

async function fetchGroups(): Promise<GroupSummary[]> {
  const msg = await clientsApi.groups(undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load groups');
  return parseMsg(msg, GroupSummaryListSchema, 'clients/groups').obj ?? [];
}

interface ConfirmState {
  title: string;
  content: string;
  okText: string;
  onOk: () => void | Promise<void>;
}

export default function GroupsPage() {
  usePageTitle();
  const { t } = useTranslation();
  const { isDark, isUltra } = useTheme();
  const queryClient = useQueryClient();

  const { inbounds } = useClients();

  const groupsQuery = useQuery({ queryKey: keys.clients.groups(), queryFn: fetchGroups });
  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data]);
  const loading = groupsQuery.isFetching;
  const fetched = groupsQuery.data !== undefined || groupsQuery.isError;
  const fetchError = groupsQuery.error ? (groupsQuery.error as Error).message : '';

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: keys.clients.root() });
  }, [queryClient]);

  const createMut = useMutation({
    mutationFn: (body: { name: string }) => clientsApi.groupsCreate(body, { silent: true }),
    onSuccess: (msg) => { if (msg?.success) invalidate(); },
  });
  const deleteMut = useMutation({
    mutationFn: (body: { name: string }) => clientsApi.groupsDelete(body, { silent: true }),
    onSuccess: (msg) => { if (msg?.success) invalidate(); },
  });

  const { isMobile } = useMediaQuery();

  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<GroupSummary | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

  const allClientsQuery = useQuery<ClientRecord[]>({
    queryKey: keys.clients.all(),
    queryFn: async () => {
      const msg = await clientsApi.list(undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load clients');
      return parseMsg(msg, ClientRecordListSchema, 'clients/list').obj ?? [];
    },
    enabled: editOpen,
    staleTime: 30_000,
  });
  const allClients = useMemo(() => allClientsQuery.data ?? [], [allClientsQuery.data]);

  const inboundLabel = useMemo(() => {
    const byId = new Map<number, string>();
    for (const ib of inbounds) byId.set(ib.id, ib.tag?.trim() || ib.remark?.trim() || String(ib.id));
    return (id: number) => byId.get(id) ?? String(id);
  }, [inbounds]);

  const totalGroups = groups.length;
  const totalClients = useMemo(() => groups.reduce((a, g) => a + (g.clientCount || 0), 0), [groups]);
  const emptyGroups = useMemo(() => groups.filter((g) => (g.clientCount || 0) === 0).length, [groups]);

  const message = getMessage();

  function runConfirm() {
    if (!confirm) return;
    setConfirmBusy(true);
    Promise.resolve(confirm.onOk())
      .finally(() => { setConfirmBusy(false); setConfirm(null); });
  }

  function openCreate() { setCreateName(''); setCreateOpen(true); }

  async function confirmCreate() {
    const name = createName.trim();
    if (!name) return;
    if (groups.some((g) => g.name.toLowerCase() === name.toLowerCase())) {
      message.error(t('pages.groups.renameCollision', { name }));
      return;
    }
    const msg = await createMut.mutateAsync({ name });
    if (msg?.success) { message.success(t('pages.groups.createSuccess', { name })); setCreateOpen(false); }
    else if (msg?.msg) message.error(msg.msg);
  }

  function openEdit(g: GroupSummary) { setEditTarget(g); setEditOpen(true); }

  function onDelete(g: GroupSummary) {
    setConfirm({
      title: t('pages.groups.deleteConfirmTitle', { name: g.name }),
      content: t('pages.groups.deleteConfirmContent', { count: g.clientCount }),
      okText: t('delete'),
      onOk: async () => {
        const msg = await deleteMut.mutateAsync({ name: g.name });
        if (msg?.success) {
          const affected = (msg.obj as { affected?: number } | undefined)?.affected ?? 0;
          message.success(t('pages.groups.deleteSuccess', { count: affected }));
        } else if (msg?.msg) message.error(msg.msg);
      },
    });
  }

  const columns = useMemo<ColumnDef<GroupSummary, unknown>[]>(() => [
    {
      id: 'actions',
      header: () => t('pages.clients.actions'),
      enableSorting: false,
      cell: ({ row }) => (
        <div className="row-actions">
          <Tooltip title={t('edit')}>
            <Button size="sm" icon={<EditOutlined />} onClick={() => openEdit(row.original)} />
          </Tooltip>
          <Tooltip title={t('pages.groups.deleteGroupOnly')}>
            <Button size="sm" className="row-delete" icon={<DeleteOutlined />} onClick={() => onDelete(row.original)} />
          </Tooltip>
        </div>
      ),
    },
    {
      accessorKey: 'name',
      header: () => t('pages.groups.groupTag'),
      cell: ({ row }) => <Tag tone="primary">{row.original.name}</Tag>,
    },
    {
      id: 'entrypoints',
      accessorFn: (g: GroupSummary) => (g.entrypointIds ?? []).length,
      header: () => t('pages.groups.inboundsInGroup'),
      cell: ({ row }) => {
        const ids = row.original.entrypointIds ?? [];
        if (ids.length === 0) return <span className="ds-muted">—</span>;
        return (
          <div className="gp-chips">
            {ids.map((id) => <Tag key={id} tone="warning">{inboundLabel(id)}</Tag>)}
          </div>
        );
      },
    },
    {
      accessorKey: 'clientCount',
      header: () => t('pages.groups.clientCount'),
      cell: ({ row }) => (
        <Tag tone={(row.original.clientCount || 0) > 0 ? 'success' : 'neutral'}>
          {row.original.clientCount || 0}
        </Tag>
      ),
    },
  ], [t, groups, inboundLabel]);

  const pageClass = ['groups-page', isDark && 'is-dark', isUltra && 'is-ultra'].filter(Boolean).join(' ');

  return (
    <TooltipProvider>
      <div className={`section-content-wrapper groups-section-wrapper ${pageClass}`}>
        {!fetched ? (
          <div className="ds-table__empty">{t('loading')}</div>
        ) : fetchError ? (
          <Card>
            <div style={{ textAlign: 'center', padding: 24 }}>
              <h3>{t('somethingWentWrong')}</h3>
              <p className="ds-muted">{fetchError}</p>
              <Button variant="primary" loading={loading} onClick={() => groupsQuery.refetch()}>{t('refresh')}</Button>
            </div>
          </Card>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: isMobile ? 8 : 12 }}>
            <Card>
              <div className="ds-stats-grid">
                <Stat title={t('pages.groups.totalGroups')} value={totalGroups} prefix={<TagsOutlined />} />
                <Stat title={t('pages.groups.totalGroupedClients')} value={totalClients} prefix={<TeamOutlined />} />
                <Stat title={t('pages.groups.emptyGroups')} value={emptyGroups} prefix={<InboxOutlined />} />
              </div>
            </Card>

            <div className="clients-add">
              <div className="vertical-tabs-container">
                <button type="button" className="vtab-btn is-active" onClick={openCreate}>
                  <span className="vtab-icon"><PlusOutlined /></span>
                  {t('pages.groups.addGroup')}
                </button>
              </div>
            </div>

            <Card flush>
              <div className="group-table" style={{ padding: '0 4px 4px' }}>
                <DataTable
                  data={groups}
                  columns={columns}
                  getRowId={(g) => g.name}
                  empty={
                    <>
                      <TagsOutlined style={{ fontSize: 28 }} />
                      <div>{t('noData')}</div>
                    </>
                  }
                />
              </div>
            </Card>
          </div>
        )}

        <Dialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          title={t('pages.groups.addGroup')}
          okText={t('create')}
          confirmLoading={createMut.isPending}
          onOk={confirmCreate}
        >
          <Field label={t('pages.groups.groupTag')}>
            <Input
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && confirmCreate()}
              placeholder={t('pages.clients.groupPlaceholder')}
              autoFocus
            />
          </Field>
        </Dialog>

        <Dialog
          open={confirm !== null}
          onOpenChange={(o) => !o && setConfirm(null)}
          title={confirm?.title ?? ''}
          okText={confirm?.okText ?? t('delete')}
          okDanger
          confirmLoading={confirmBusy}
          onOk={runConfirm}
        >
          <p style={{ margin: 0 }}>{confirm?.content}</p>
        </Dialog>

        <LazyMount when={editOpen}>
          <GroupEditModal
            open={editOpen}
            group={editTarget}
            groups={groups}
            inbounds={inbounds}
            clients={allClients}
            onClose={() => setEditOpen(false)}
            onSaved={invalidate}
          />
        </LazyMount>
      </div>
    </TooltipProvider>
  );
}
