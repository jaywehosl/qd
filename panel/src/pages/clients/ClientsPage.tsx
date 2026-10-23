import { lazy, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  InfoCircleOutlined,
  LockOutlined,
  PlusOutlined,
  RetweetOutlined,
  SearchOutlined,
  StopOutlined,
  TeamOutlined,
  WarningOutlined,
  WifiOutlined,
} from '@ant-design/icons';

import {
  Button,
  Card,
  DataTable,
  Dialog,
  Input,
  Pagination,
  Popover,
  Select,
  Stat,
  Switch,
  Tag,
  Tooltip,
  TooltipProvider,
  type ColumnDef,
  type SortingState,
  type TagTone,
} from '@/components/ds';
import { Spin } from '@/components/ui';
import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useHold } from '@/hooks/useHold';
import { useWebSocket } from '@/hooks/useWebSocket';
import { cellOf } from '@/components/ui';
import { useClients, type ClientRecord, type ClientStatus } from '@/hooks/useClients';
import { IntlUtil, SizeFormatter } from '@/utils';
import { getMessage } from '@/utils/messageBus';
import { LazyMount } from '@/components/utility';
const ClientFormModal = lazy(() => import('./ClientFormModal'));
const ClientInfoModal = lazy(() => import('./ClientInfoModal'));
const BlockedDevicesModal = lazy(() => import('./BlockedDevicesModal'));
const FILTER_STATE_KEY = 'clientsFilterState';
const DISABLED_PAGE_SIZE = 200;

type Presence = 'online' | 'offline' | 'expiring' | 'expired' | 'disabled';

interface ViewState {
  search: string;
  group: string;
  status: ClientStatus;
  sort: string;
}

function readViewState(): ViewState {
  try {
    const raw = JSON.parse(localStorage.getItem(FILTER_STATE_KEY) || '{}');
    return {
      search: typeof raw.search === 'string' ? raw.search : '',
      group: typeof raw.group === 'string' ? raw.group : '',
      status: typeof raw.status === 'string' ? raw.status : '',
      sort: typeof raw.sort === 'string' ? raw.sort : '',
    };
  } catch {
    return { search: '', group: '', status: '', sort: '' };
  }
}

const SORT_OPTIONS: { value: string; column: string; order: 'ascend' | 'descend'; labelKey: string }[] = [
  { value: 'createdAt:ascend', column: 'createdAt', order: 'ascend', labelKey: 'pages.clients.sortOldest' },
  { value: 'createdAt:descend', column: 'createdAt', order: 'descend', labelKey: 'pages.clients.sortNewest' },
  { value: 'updatedAt:descend', column: 'updatedAt', order: 'descend', labelKey: 'pages.clients.sortRecentlyUpdated' },
  { value: 'lastOnline:descend', column: 'lastOnline', order: 'descend', labelKey: 'pages.clients.sortRecentlyOnline' },
  { value: 'tag:ascend', column: 'tag', order: 'ascend', labelKey: 'pages.clients.sortEmailAZ' },
  { value: 'tag:descend', column: 'tag', order: 'descend', labelKey: 'pages.clients.sortEmailZA' },
  { value: 'traffic:descend', column: 'traffic', order: 'descend', labelKey: 'pages.clients.sortMostTraffic' },
  { value: 'expiryAt:ascend', column: 'expiryAt', order: 'ascend', labelKey: 'pages.clients.sortExpiringSoonest' },
];
const DEFAULT_SORT = SORT_OPTIONS[0];

interface ConfirmState {
  title: string;
  content: string;
  okText: string;
  onOk: () => void | Promise<void>;
}

export default function ClientsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra } = useTheme();
  const { isMobile } = useMediaQuery();
  const message = getMessage();

  const {
    clients, total, filtered, summary, allGroups,
    setQuery, onlines, loading, refreshing, fetched, fetchError,
    expireDiff, pageSize,
    create, update, remove, resetTraffic, setEnable,
    applyTrafficEvent, applyClientStatsEvent,
    refresh,
  } = useClients();

  useWebSocket({ traffic: applyTrafficEvent, client_stats: applyClientStatsEvent });

  const [formOpen, setFormOpen] = useState(false);
  const [editingClient, setEditingClient] = useState<ClientRecord | null>(null);
  const [infoOpen, setInfoOpen] = useState(false);
  const hold = useHold();
  const [statsOpen, setStatsOpen] = useState(false);
  const [blockedOpen, setBlockedOpen] = useState(false);
  const [infoId, setInfoId] = useState<number | null>(null);
  const infoClient = useMemo(() => clients.find((c) => c.id === infoId) ?? null, [clients, infoId]);

  const initial = readViewState();
  const [searchKey, setSearchKey] = useState(initial.search);
  const [group, setGroup] = useState(initial.group);
  const [status, setStatus] = useState<ClientStatus>(initial.status);
  const initialSort = SORT_OPTIONS.find((o) => o.value === initial.sort) ?? DEFAULT_SORT;
  const [sortColumn, setSortColumn] = useState(initialSort.column);
  const [sortOrder, setSortOrder] = useState<'ascend' | 'descend'>(initialSort.order);
  const [currentPage, setCurrentPage] = useState(1);
  const [tablePageSize, setTablePageSize] = useState(25);
  const [debouncedSearch, setDebouncedSearch] = useState(searchKey);

  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);
  function runConfirm() {
    if (!confirm) return;
    setConfirmBusy(true);
    Promise.resolve(confirm.onOk()).finally(() => { setConfirmBusy(false); setConfirm(null); });
  }

  useEffect(() => {
    localStorage.setItem(FILTER_STATE_KEY, JSON.stringify({
      search: searchKey, group, status, sort: `${sortColumn}:${sortOrder}`,
    }));
  }, [searchKey, group, status, sortColumn, sortOrder]);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebouncedSearch(searchKey), 300);
    return () => window.clearTimeout(handle);
  }, [searchKey]);

  useEffect(() => { setCurrentPage(1); }, [debouncedSearch, group, status, sortColumn, sortOrder]);

  useEffect(() => {
    setQuery({
      page: currentPage,
      pageSize: tablePageSize,
      search: debouncedSearch,
      group,
      status,
      sort: sortColumn,
      order: sortOrder,
    });
  }, [setQuery, currentPage, tablePageSize, debouncedSearch, group, status, sortColumn, sortOrder]);

  useEffect(() => { setTablePageSize(pageSize > 0 ? pageSize : DISABLED_PAGE_SIZE); }, [pageSize]);

  const onlineSet = useMemo(() => new Set(onlines), [onlines]);

  const presence = useCallback((row: ClientRecord): Presence => {
    const now = Date.now();
    const expiry = row.expiryAt ?? 0;
    if (expiry > 0 && expiry <= now) return 'expired';
    if (!row.enable) return 'disabled';
    if (onlineSet.has(row.tag)) return 'online';
    if (expiry > 0 && expiry - now < expireDiff) return 'expiring';
    return 'offline';
  }, [onlineSet, expireDiff]);

  function expiryTone(row: ClientRecord): TagTone {
    const expiry = row.expiryAt ?? 0;
    if (!expiry) return 'primary';
    const now = Date.now();
    if (expiry <= now) return 'danger';
    if (expiry - now < expireDiff) return 'warning';
    return 'success';
  }

  function trafficLabel(row: ClientRecord) {
    const up = row.traffic?.up || 0;
    const down = row.traffic?.down || 0;
    return (
      <Popover
        content={<span>↑ {SizeFormatter.sizeFormat(up)} ↓ {SizeFormatter.sizeFormat(down)}</span>}
        trigger={<Tag style={{ cursor: 'pointer', whiteSpace: 'nowrap' }}>{SizeFormatter.sizeFormat(up + down)}</Tag>}
      />
    );
  }

  async function onToggleEnable(row: ClientRecord, next: boolean) {
    const msg = await setEnable(row, next);
    if (!msg?.success) message.error(msg?.msg || t('somethingWentWrong'));
  }

  function onAdd() { setEditingClient(null); setFormOpen(true); }
  function onEdit(row: ClientRecord) { setEditingClient(row); setFormOpen(true); }

  function onDelete(row: ClientRecord) {
    setConfirm({
      title: t('pages.clients.deleteConfirmTitle', { email: row.tag }),
      content: t('pages.clients.deleteConfirmContent'),
      okText: t('delete'),
      onOk: async () => { const msg = await remove(row.id); if (msg?.success) message.success(t('pages.clients.toasts.deleted')); },
    });
  }

  function onResetTraffic(row: ClientRecord) {
    setConfirm({
      title: `${t('pages.inbounds.resetTraffic')} — ${row.tag}`,
      content: t('pages.inbounds.resetTrafficContent'),
      okText: t('reset'),
      onOk: async () => { const msg = await resetTraffic(row.id); if (msg?.success) message.success(t('pages.clients.toasts.trafficReset')); },
    });
  }

  function onShowInfo(row: ClientRecord) {
    setInfoId(row.id);
    setInfoOpen(true);
  }

  const onSave = useCallback((client: Record<string, unknown>, id?: number) => (
    id ? update(id, client) : create(client)
  ), [create, update]);

  const pageClass = useMemo(() => ['clients-page', isDark && 'is-dark', isUltra && 'is-ultra'].filter(Boolean).join(' '), [isDark, isUltra]);

  const columns = useMemo<ColumnDef<ClientRecord, unknown>[]>(() => {
    const cols: ColumnDef<ClientRecord, unknown>[] = [
      {
        id: 'actions', size: 200, enableSorting: false, header: () => t('pages.clients.actions'),
        cell: ({ row }) => {
          const record = row.original;
          return (
            <div className="row-actions">
              <Tooltip title={t('pages.clients.clientInfo')}><Button size="sm" icon={<InfoCircleOutlined />} onClick={() => onShowInfo(record)} /></Tooltip>
              <Tooltip title={t('pages.inbounds.resetTraffic')}><Button size="sm" icon={<RetweetOutlined />} onClick={() => onResetTraffic(record)} /></Tooltip>
              <Tooltip title={t('edit')}><Button size="sm" icon={<EditOutlined />} onClick={() => onEdit(record)} /></Tooltip>
              <Tooltip title={t('delete')}>
                <Button size="sm" className="row-delete" icon={<DeleteOutlined />} onClick={() => onDelete(record)} />
              </Tooltip>
            </div>
          );
        },
      },
      {
        id: 'enable', size: 80, enableSorting: false, header: () => t('pages.clients.enabled'),
        cell: ({ row }) => <Switch checked={!!row.original.enable} onChange={(next) => onToggleEnable(row.original, next)} />,
      },
      {
        id: 'lastOnline', size: 100, header: () => t('pages.clients.online'),
        cell: ({ row }) => {
          const record = row.original;
          const lastOnline = record.traffic?.lastOnline ?? 0;
          const title = `${t('lastOnline')}: ${lastOnline > 0 ? IntlUtil.formatDate(lastOnline) : '-'}`;
          const dot = <span className="state-dot" />;
          switch (presence(record)) {
            case 'expired': return <Tooltip title={title}><Tag tone="danger">{dot}{t('depleted')}</Tag></Tooltip>;
            case 'online': return <Tag tone="success">{dot}{t('pages.clients.online')}</Tag>;
            case 'disabled': return <Tag>{dot}{t('disabled')}</Tag>;
            case 'expiring': return <Tooltip title={title}><Tag tone="warning">{dot}{t('depletingSoon')}</Tag></Tooltip>;
          }
          return <Tooltip title={title}><Tag>{dot}{t('pages.clients.offline')}</Tag></Tooltip>;
        },
      },
      {
        id: 'tag', header: () => t('pages.clients.client'),
        cell: ({ row }) => (
          <div className="email-cell">
            <span className="email">{row.original.tag}</span>
            {row.original.comment && <span className="sub" title={row.original.comment}>{row.original.comment}</span>}
          </div>
        ),
      },
    ];
    if (allGroups.length > 0) {
      cols.push({
        id: 'group', size: 130, enableSorting: false, header: () => t('pages.clients.group'),
        cell: ({ row }) => {
          const tag = row.original.group;
          if (!tag) return <span className="ds-muted">—</span>;
          return (
            <Tag tone="primary" style={{ cursor: 'pointer', opacity: group === tag ? 0.6 : 1 }} onClick={() => setGroup(tag)}>
              {tag}
            </Tag>
          );
        },
      });
    }
    cols.push(
      {
        id: 'traffic', header: () => t('pages.clients.traffic'),
        cell: ({ row }) => trafficLabel(row.original),
      },
      {
        id: 'expiryAt', header: () => t('pages.clients.duration'),
        cell: ({ row }) => {
          const expiry = row.original.expiryAt ?? 0;
          return (
            <Tooltip title={expiry ? IntlUtil.formatDate(expiry) : '∞'}>
              <Tag tone={expiryTone(row.original)}>{expiry ? IntlUtil.formatRelativeTime(expiry) : '∞'}</Tag>
            </Tooltip>
          );
        },
      },
    );
    return cols;
  }, [t, presence, allGroups, group, expireDiff]);

  const sortValue = `${sortColumn}:${sortOrder}`;
  const presetSortValue = SORT_OPTIONS.some((o) => o.value === sortValue) ? sortValue : undefined;

  const tableSorting = useMemo<SortingState>(
    () => [{ id: sortColumn, desc: sortOrder === 'descend' }],
    [sortColumn, sortOrder],
  );

  function onTableSortingChange(next: SortingState) {
    const first = next[0];
    if (!first) { setSortColumn(DEFAULT_SORT.column); setSortOrder(DEFAULT_SORT.order); return; }
    setSortColumn(first.id);
    setSortOrder(first.desc ? 'descend' : 'ascend');
  }

  const pagination = {
    page: currentPage,
    pageSize: tablePageSize,
    total: filtered,
    pageSizeOptions: [10, 25, 50, 100, 200],
    onChange: (p: number, s: number) => { setCurrentPage(p); if (s !== tablePageSize) setTablePageSize(s); },
  };

  const statusOptions: { value: ClientStatus; label: string }[] = [
    { value: '', label: t('pages.clients.statusAll', { defaultValue: 'All' }) },
    { value: 'online', label: t('online') },
    { value: 'active', label: t('subscription.active') },
    { value: 'expiring', label: t('depletingSoon') },
    { value: 'expired', label: t('depleted') },
    { value: 'disabled', label: t('disabled') },
  ];

  function statWithList(title: string, value: number, icon: React.ReactNode, tags: string[]) {
    const stat = <Stat title={title} value={String(value)} prefix={icon} />;
    if (tags.length === 0) return stat;
    return (
      <Popover
        side="bottom"
        trigger={<button type="button" className="stat-trigger">{stat}</button>}
        content={<div className="client-email-list">{tags.map((e) => <div key={e}>{e}</div>)}</div>}
      />
    );
  }

  const narrowed = group !== '' || status !== '' || debouncedSearch.trim() !== '';

  return (
    <TooltipProvider>
      <div className={`section-content-wrapper clients-section-wrapper ${pageClass}`}>
        {!fetched ? (
          <div className="ds-table__empty">{t('loading')}</div>
        ) : fetchError ? (
          <Card>
            <div style={{ textAlign: 'center', padding: 24 }}>
              <h3>{t('somethingWentWrong')}</h3>
              <p className="ds-muted">{fetchError}</p>
              <Button variant="primary" loading={refreshing} onClick={refresh}>{t('refresh')}</Button>
            </div>
          </Card>
        ) : (
          <div className="page-stack">
            <Card>
              {isMobile ? (
                <button type="button" className={`ds-stats-grid is-fold${statsOpen ? ' is-open' : ''}`} aria-expanded={statsOpen} onClick={() => setStatsOpen(!statsOpen)}>
                  <Stat title={t('online')} value={String(summary.online.length)} prefix={<WifiOutlined />} />
                  <Stat title={t('clients')} value={String(summary.total)} prefix={<TeamOutlined />} />
                  <Stat title={t('disabled')} value={String(summary.disabled.length)} prefix={<StopOutlined />} />
                  <Stat title={t('depleted')} value={String(summary.expired.length)} prefix={<CloseCircleOutlined />} />
                  <Stat title={t('depletingSoon')} value={String(summary.expiring.length)} prefix={<WarningOutlined />} />
                  <Stat title={t('subscription.active')} value={String(summary.active)} prefix={<CheckCircleOutlined />} />
                </button>
              ) : (
                <div className="ds-stats-grid">
                  <Stat title={t('clients')} value={String(summary.total)} prefix={<TeamOutlined />} />
                  {statWithList(t('online'), summary.online.length, <WifiOutlined />, summary.online)}
                  {statWithList(t('depleted'), summary.expired.length, <CloseCircleOutlined />, summary.expired)}
                  {statWithList(t('depletingSoon'), summary.expiring.length, <WarningOutlined />, summary.expiring)}
                  {statWithList(t('disabled'), summary.disabled.length, <StopOutlined />, summary.disabled)}
                  <Stat title={t('subscription.active')} value={String(summary.active)} prefix={<CheckCircleOutlined />} />
                </div>
              )}
            </Card>

            {!isMobile && (
              <div className="clients-add">
                <div className="vertical-tabs-container">
                  <button type="button" className="vtab-btn is-active" onClick={onAdd}>
                    <span className="vtab-icon"><PlusOutlined /></span>
                    {t('pages.clients.addClients')}
                  </button>
                  <button type="button" className="vtab-btn" onClick={() => setBlockedOpen(true)}>
                    <span className="vtab-icon"><LockOutlined /></span>
                    {t('pages.clients.blockedDevices')}
                  </button>
                </div>
              </div>
            )}

            <Card flush>
              <div className={isMobile ? 'mc-tools' : 'card-toolbar'}>
                <div className="toolbar-search">
                  <SearchOutlined className="toolbar-search__icon" />
                  <Input value={searchKey} onChange={(e) => setSearchKey(e.target.value)} placeholder={t('pages.clients.searchPlaceholder')} />
                </div>

                <div className="toolbar-sort">
                  <Select
                    value={status}
                    onChange={(value) => setStatus(value as ClientStatus)}
                    options={statusOptions}
                  />
                </div>

                <div className="toolbar-sort">
                  <Select
                    value={presetSortValue}
                    placeholder={t('pages.clients.sortCustom')}
                    onChange={(value) => { const opt = SORT_OPTIONS.find((o) => o.value === value) ?? DEFAULT_SORT; setSortColumn(opt.column); setSortOrder(opt.order); }}
                    options={SORT_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                  />
                </div>

                {isMobile && (
                  <button type="button" className="mc-icon" aria-label={t('pages.clients.blockedDevices')} onClick={() => setBlockedOpen(true)}>
                    <LockOutlined />
                  </button>
                )}
                {isMobile ? (
                  <button type="button" className="mc-add" aria-label={t('pages.clients.addClients')} onClick={onAdd}>
                    <PlusOutlined />
                  </button>
                ) : (
                  <div className="toolbar-pages">
                    <Pagination {...pagination} />
                  </div>
                )}
              </div>

              {narrowed && (
                <div className="filter-chips">
                  <span className="filter-count">{t('pages.clients.showingCount', { shown: filtered, total })}</span>
                  {group && (
                    <Tag tone="primary" className="filter-chip">
                      {`${t('pages.clients.group')}: ${group}`}
                      <button type="button" className="filter-chip__x" aria-label={t('delete')} onClick={() => setGroup('')}>×</button>
                    </Tag>
                  )}
                </div>
              )}

              {!isMobile ? (
                <div className="client-table" style={{ padding: '0 4px 4px' }}>
                  <Spin spinning={loading}>
                    <DataTable
                      data={clients}
                      columns={columns}
                      getRowId={(c) => String(c.id)}
                      sorting={tableSorting}
                      onSortingChange={onTableSortingChange}
                      empty={<><TeamOutlined style={{ fontSize: 32, marginBottom: 8 }} /><div>{t('noData')}</div></>}
                    />
                  </Spin>
                </div>
              ) : (
                <Spin spinning={loading}>
                  <div className="mc-list">
                    {clients.length === 0 && (
                      <div className="card-empty"><TeamOutlined style={{ fontSize: 28, opacity: 0.5 }} /><div>{t('noData')}</div></div>
                    )}
                    {clients.map((row) => (
                      <div key={row.id} className="mc is-tap" {...hold(() => onShowInfo(row), () => onEdit(row))}>
                        <span className={`mc__dot is-${presence(row) === 'expired' ? 'depleted' : presence(row)}`} />
                        <div className="mc__body">
                          <span className="mc__name">{row.tag}</span>
                          <span className="mc__sub">
                            {row.group && <span className="is-group">{row.group}</span>}
                            <span>{SizeFormatter.sizeFormat((row.traffic?.up || 0) + (row.traffic?.down || 0))}</span>
                            <span className={`is-${expiryTone(row)}`}>{row.expiryAt ? IntlUtil.formatRelativeTime(row.expiryAt) : '∞'}</span>
                          </span>
                          {row.comment && <span className="mc__note">{row.comment}</span>}
                        </div>
                        <span className="mc__end" onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>{cellOf(columns, 'enable', row)}</span>
                      </div>
                    ))}
                  </div>
                  <div className="cards-pages">
                    <Pagination {...pagination} />
                  </div>
                </Spin>
              )}
            </Card>
          </div>
        )}

        <Dialog
          open={confirm !== null}
          onOpenChange={(o) => !o && setConfirm(null)}
          title={confirm?.title ?? ''}
          okText={confirm?.okText ?? t('confirm')}
          okDanger
          confirmLoading={confirmBusy}
          onOk={runConfirm}
        >
          <p className="ds-confirm__text">{confirm?.content}</p>
        </Dialog>

        <LazyMount when={formOpen}>
          <ClientFormModal open={formOpen} client={editingClient} groups={allGroups} save={onSave} onOpenChange={setFormOpen} />
        </LazyMount>
        <LazyMount when={blockedOpen}>
          <BlockedDevicesModal open={blockedOpen} onOpenChange={setBlockedOpen} onChanged={refresh} />
        </LazyMount>
        <LazyMount when={infoOpen}>
          <ClientInfoModal
            open={infoOpen}
            client={infoClient}
            isOnline={infoClient ? presence(infoClient) === 'online' : false}
            onChanged={refresh}
            onOpenChange={setInfoOpen}
            actions={isMobile && infoClient ? {
              edit: () => { setInfoOpen(false); onEdit(infoClient); },
              reset: () => { setInfoOpen(false); onResetTraffic(infoClient); },
              remove: () => { setInfoOpen(false); onDelete(infoClient); },
            } : undefined}
          />
        </LazyMount>
      </div>
    </TooltipProvider>
  );
}
