import { lazy, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  InfoCircleOutlined,
  MoreOutlined,
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
  DropdownMenu,
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
import { useWebSocket } from '@/hooks/useWebSocket';
import { useClients } from '@/hooks/useClients';
import { useDatepicker } from '@/hooks/useDatepicker';
import type { ClientRecord, InboundOption } from '@/hooks/useClients';
import { ColorUtils, IntlUtil, SizeFormatter } from '@/utils';
import { getMessage } from '@/utils/messageBus';
import { LazyMount } from '@/components/utility';
const ClientFormModal = lazy(() => import('./ClientFormModal'));
const ClientInfoModal = lazy(() => import('./ClientInfoModal'));
import { emptyFilters, activeFilterCount } from './filters';
import type { ClientFilters } from './filters';
const FILTER_STATE_KEY = 'clientsFilterState';
const DISABLED_PAGE_SIZE = 200;

function tone(c?: string): TagTone {
  switch (c) {
    case 'green': case 'lime': return 'success';
    case 'red': case 'magenta': case 'volcano': return 'danger';
    case 'gold': case 'orange': return 'warning';
    case 'blue': case 'geekblue': case 'cyan': case 'purple': return 'primary';
    default: return 'neutral';
  }
}

type Bucket = 'active' | 'deactive' | 'depleted' | 'expiring';

interface PersistedFilterState {
  searchKey: string;
  filters: ClientFilters;
  sort: string;
}

function readFilterState(): PersistedFilterState {
  try {
    const raw = JSON.parse(localStorage.getItem(FILTER_STATE_KEY) || '{}');
    const fromRaw = (raw.filters ?? {}) as Partial<ClientFilters>;
    return {
      searchKey: typeof raw.searchKey === 'string' ? raw.searchKey : '',
      filters: {
        ...emptyFilters(),
        ...fromRaw,
        buckets: Array.isArray(fromRaw.buckets) ? fromRaw.buckets : [],
        protocols: Array.isArray(fromRaw.protocols) ? fromRaw.protocols : [],
        inboundIds: Array.isArray(fromRaw.inboundIds) ? fromRaw.inboundIds : [],
        groups: Array.isArray(fromRaw.groups) ? fromRaw.groups : [],
      },
      sort: typeof raw.sort === 'string' ? raw.sort : '',
    };
  } catch {
    return { searchKey: '', filters: emptyFilters(), sort: '' };
  }
}

function gbToBytes(gb: number | undefined): number {
  if (!gb || gb <= 0) return 0;
  return Math.round(gb * 1024 * 1024 * 1024);
}

const SORT_OPTIONS: { value: string; column: string; order: 'ascend' | 'descend'; labelKey: string }[] = [
  { value: 'createdAt:ascend', column: 'createdAt', order: 'ascend', labelKey: 'pages.clients.sortOldest' },
  { value: 'createdAt:descend', column: 'createdAt', order: 'descend', labelKey: 'pages.clients.sortNewest' },
  { value: 'updatedAt:descend', column: 'updatedAt', order: 'descend', labelKey: 'pages.clients.sortRecentlyUpdated' },
  { value: 'lastOnline:descend', column: 'lastOnline', order: 'descend', labelKey: 'pages.clients.sortRecentlyOnline' },
  { value: 'email:ascend', column: 'email', order: 'ascend', labelKey: 'pages.clients.sortEmailAZ' },
  { value: 'email:descend', column: 'email', order: 'descend', labelKey: 'pages.clients.sortEmailZA' },
  { value: 'traffic:descend', column: 'traffic', order: 'descend', labelKey: 'pages.clients.sortMostTraffic' },
  { value: 'expiryTime:ascend', column: 'expiryTime', order: 'ascend', labelKey: 'pages.clients.sortExpiringSoonest' },
];
const DEFAULT_SORT = SORT_OPTIONS[0];

function sortValueFor(column: string | null, order: 'ascend' | 'descend' | null): string {
  if (!column || !order) return DEFAULT_SORT.value;
  return `${column}:${order}`;
}

function bucketChipLabel(b: string, t: (k: string) => string): string {
  switch (b) {
    case 'active': return t('subscription.active');
    case 'expiring': return t('depletingSoon');
    case 'depleted': return t('depleted');
    case 'deactive': return t('disabled');
    case 'online': return t('online');
    default: return b;
  }
}

interface ConfirmState {
  title: string;
  content: string;
  okText: string;
  onOk: () => void | Promise<void>;
}

export default function ClientsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra } = useTheme();
  const { datepicker } = useDatepicker();
  const { isMobile } = useMediaQuery();
  const message = getMessage();

  const {
    clients, total, filtered,
    summary: serverSummary,
    allGroups,
    setQuery,
    inbounds, onlines, loading, refreshing, fetched, fetchError,
    tgBotEnable, expireDiff, trafficDiff, pageSize,
    create, update, remove, attach, detach,
    resetTraffic, setEnable,
    applyTrafficEvent, applyClientStatsEvent,
    refresh,
    hydrate,
  } = useClients();

  useWebSocket({ traffic: applyTrafficEvent, client_stats: applyClientStatsEvent });

  const [togglingEmail, setTogglingEmail] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [editingClient, setEditingClient] = useState<ClientRecord | null>(null);
  const [editingAttachedIds, setEditingAttachedIds] = useState<number[]>([]);
  const [infoOpen, setInfoOpen] = useState(false);
  const [infoClient, setInfoClient] = useState<ClientRecord | null>(null);
  const liveInfoClient = useMemo(
    () => (infoClient ? clients.find((c) => c.email === infoClient.email) ?? infoClient : null),
    [clients, infoClient],
  );

  const initial = readFilterState();
  const [searchKey, setSearchKey] = useState(initial.searchKey);
  const [filters, setFilters] = useState<ClientFilters>(initial.filters);

  const initialSort = SORT_OPTIONS.find((o) => o.value === initial.sort) ?? DEFAULT_SORT;
  const [sortColumn, setSortColumn] = useState<string | null>(initialSort.column);
  const [sortOrder, setSortOrder] = useState<'ascend' | 'descend' | null>(initialSort.order);
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
    localStorage.setItem(FILTER_STATE_KEY, JSON.stringify({ searchKey, filters, sort: sortValueFor(sortColumn, sortOrder) }));
  }, [searchKey, filters, sortColumn, sortOrder]);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebouncedSearch(searchKey), 300);
    return () => window.clearTimeout(handle);
  }, [searchKey]);

  useEffect(() => { setCurrentPage(1); }, [debouncedSearch, filters, sortColumn, sortOrder]);

  useEffect(() => {
    setQuery({
      page: currentPage,
      pageSize: tablePageSize,
      search: debouncedSearch,
      filter: filters.buckets.join(','),
      protocol: filters.protocols.join(','),
      inbound: filters.inboundIds.join(','),
      expiryFrom: filters.expiryFrom,
      expiryTo: filters.expiryTo,
      usageFrom: gbToBytes(filters.usageFromGB),
      usageTo: gbToBytes(filters.usageToGB),
      autoRenew: filters.autoRenew || undefined,
      hasTgId: filters.hasTgId || undefined,
      hasComment: filters.hasComment || undefined,
      group: filters.groups.join(',') || undefined,
      sort: sortColumn || undefined,
      order: sortOrder || undefined,
    });
  }, [setQuery, currentPage, tablePageSize, debouncedSearch, filters, sortColumn, sortOrder]);

  const activeCount = activeFilterCount(filters);

  useEffect(() => { setTablePageSize(pageSize > 0 ? pageSize : DISABLED_PAGE_SIZE); }, [pageSize]);

  const onlineSet = useMemo(() => new Set(onlines || []), [onlines]);
  const inboundsById = useMemo(() => {
    const out: Record<number, InboundOption> = {};
    for (const ib of inbounds) out[ib.id] = ib;
    return out;
  }, [inbounds]);

  const isOnline = useCallback((email: string) => !!email && onlineSet.has(email), [onlineSet]);

  function inboundLabel(id: number) {
    const ib = inboundsById[id];
    return ib?.remark?.trim() || ib?.tag || '';
  }

  const clientBucket = useCallback((row: ClientRecord | null | undefined): Bucket | null => {
    if (!row) return null;
    const now = Date.now();
    const expired = (row.expiryTime ?? 0) > 0 && (row.expiryTime ?? 0) <= now;
    if (expired) return 'depleted';
    if (!row.enable) return 'deactive';
    const nearExpiry = (row.expiryTime ?? 0) > 0 && (row.expiryTime ?? 0) - now < (expireDiff || 0);
    if (nearExpiry) return 'expiring';
    return 'active';
  }, [expireDiff, trafficDiff]);

  function bucketDotClass(bucket: Bucket | null): string {
    switch (bucket) {
      case 'depleted': return 'dot dot-red';
      case 'expiring': return 'dot dot-orange';
      case 'active': return 'dot dot-green';
      default: return 'dot dot-gray';
    }
  }

  const filteredClients = clients;
  const summary = serverSummary;
  const sortedClients = filteredClients;

  function trafficLabel(row: ClientRecord) {
    const t0 = row.traffic;
    const up = t0?.up || 0;
    const down = t0?.down || 0;
    const total0 = row.totalGB || 0;
    return (
      <Popover
        content={
          <table cellPadding={2}>
            <tbody>
              <tr>
                <td>↑ {SizeFormatter.sizeFormat(up)}</td>
                <td>↓ {SizeFormatter.sizeFormat(down)}</td>
              </tr>
            </tbody>
          </table>
        }
        trigger={
          <Tag
            tone={tone(ColorUtils.usageColor(up + down, trafficDiff, total0))}
            style={{ cursor: 'pointer', whiteSpace: 'nowrap' }}
          >
            {SizeFormatter.sizeFormat(up + down)}
            {total0 > 0 && ` / ${SizeFormatter.sizeFormat(total0)}`}
          </Tag>
        }
      />
    );
  }
  function expiryLabel(row: ClientRecord) {
    if (!row.expiryTime) return '∞';
    if (row.expiryTime < 0) { const days = Math.round(row.expiryTime / -86400000); return `${t('pages.clients.delayedStart')}: ${days}d`; }
    return IntlUtil.formatDate(row.expiryTime, datepicker);
  }
  function expiryRelative(row: ClientRecord) {
    if (!row.expiryTime) return '';
    if (row.expiryTime < 0) { const days = Math.round(row.expiryTime / -86400000); return `${days}d`; }
    return IntlUtil.formatRelativeTime(row.expiryTime);
  }
  function expiryColor(row: ClientRecord): string {
    if (!row.expiryTime) return 'purple';
    if (row.expiryTime < 0) return 'blue';
    const now = Date.now();
    if (row.expiryTime <= now) return 'red';
    if (row.expiryTime - now < 86400 * 1000 * 3) return 'orange';
    return 'green';
  }

  async function onToggleEnable(row: ClientRecord, next: boolean) {
    setTogglingEmail(row.email);
    try {
      const msg = await setEnable(row, next);
      if (!msg?.success) message.error(msg?.msg || t('somethingWentWrong'));
    } finally {
      setTogglingEmail(null);
    }
  }

  function onAdd() { setFormMode('add'); setEditingClient(null); setEditingAttachedIds([]); setFormOpen(true); }

  async function onEdit(row: ClientRecord) {
    setFormMode('edit');
    const full = await hydrate(row.email);
    const merged: ClientRecord = full ? { ...row, ...full.client } : { ...row };
    setEditingClient(merged);
    const ids = full?.inboundIds ?? (Array.isArray(row.inboundIds) ? row.inboundIds : []);
    setEditingAttachedIds([...ids]);
    setFormOpen(true);
  }

  function onDelete(row: ClientRecord) {
    setConfirm({
      title: t('pages.clients.deleteConfirmTitle', { email: row.email }),
      content: t('pages.clients.deleteConfirmContent'),
      okText: t('delete'),
      onOk: async () => { const msg = await remove(row.email); if (msg?.success) message.success(t('pages.clients.toasts.deleted')); },
    });
  }

  function onResetTraffic(row: ClientRecord) {
    if (!row?.email) { message.warning(t('pages.clients.resetNotPossible')); return; }
    setConfirm({
      title: `${t('pages.inbounds.resetTraffic')} — ${row.email}`,
      content: t('pages.inbounds.resetTrafficContent'),
      okText: t('reset'),
      onOk: async () => { const msg = await resetTraffic(row); if (msg?.success) message.success(t('pages.clients.toasts.trafficReset')); },
    });
  }

  async function onShowInfo(row: ClientRecord) {
    const full = await hydrate(row.email);
    setInfoClient(full ? { ...row, ...full.client, inboundIds: full.inboundIds } : row);
    setInfoOpen(true);
  }
  const onSave = useCallback(async (
    payload: Record<string, unknown> | { client: Record<string, unknown>; inboundIds: number[] },
    meta: { isEdit: false } | { isEdit: true; email: string; attach: number[]; detach: number[] },
  ) => {
    if (!meta.isEdit) return create(payload);
    const updateMsg = await update(meta.email, payload);
    if (!updateMsg?.success) return updateMsg;
    if (Array.isArray(meta.attach) && meta.attach.length > 0) { const r = await attach(meta.email, meta.attach); if (!r?.success) return r; }
    if (Array.isArray(meta.detach) && meta.detach.length > 0) { const r = await detach(meta.email, meta.detach); if (!r?.success) return r; }
    return updateMsg;
  }, [create, update, attach, detach]);

  const pageClass = useMemo(() => ['clients-page', isDark && 'is-dark', isUltra && 'is-ultra'].filter(Boolean).join(' '), [isDark, isUltra]);

  const columns = useMemo<ColumnDef<ClientRecord, unknown>[]>(() => {
    const cols: ColumnDef<ClientRecord, unknown>[] = [
      {
        id: 'actions', size: 200, header: () => t('pages.clients.actions'),
        cell: ({ row }) => {
          const record = row.original;
          return (
            <div className="row-actions">
              <Tooltip title={t('pages.clients.clientInfo')}><Button size="sm" icon={<InfoCircleOutlined />} onClick={() => onShowInfo(record)} /></Tooltip>
              <Tooltip title={t('pages.inbounds.resetTraffic')}><Button size="sm" icon={<RetweetOutlined />} onClick={() => onResetTraffic(record)} /></Tooltip>
              <Tooltip title={t('edit')}><Button size="sm" icon={<EditOutlined />} onClick={() => onEdit(record)} /></Tooltip>
              {/* Plain until pointed at: a row of four buttons should not have
                  one shouting red among them. */}
              <Tooltip title={t('delete')}>
                <Button size="sm" className="row-delete" icon={<DeleteOutlined />} onClick={() => onDelete(record)} />
              </Tooltip>
            </div>
          );
        },
      },
      {
        id: 'enable', size: 80, accessorFn: (c) => (c.enable ? 1 : 0), header: () => t('pages.clients.enabled'),
        cell: ({ row }) => <Switch checked={!!row.original.enable} onChange={(next) => onToggleEnable(row.original, next)} />,
      },
      {
        id: 'online', size: 100, accessorFn: (c) => c.traffic?.lastOnline ?? 0, header: () => t('pages.clients.online'),
        cell: ({ row }) => {
          const record = row.original;
          const bucket = clientBucket(record);
          const lastOnline = record.traffic?.lastOnline ?? 0;
          const title = `${t('lastOnline')}: ${lastOnline > 0 ? IntlUtil.formatDate(lastOnline, datepicker) : '-'}`;
          const dot = <span className="state-dot" />;
          if (bucket === 'depleted') return <Tooltip title={title}><Tag tone="danger">{dot}{t('depleted')}</Tag></Tooltip>;
          if (record.enable && isOnline(record.email)) return <Tag tone="success">{dot}{t('pages.clients.online')}</Tag>;
          if (!record.enable) return <Tag>{dot}{t('disabled')}</Tag>;
          if (bucket === 'expiring') return <Tag tone="warning">{dot}{t('depletingSoon')}</Tag>;
          return <Tooltip title={title}><Tag>{dot}{t('pages.clients.offline')}</Tag></Tooltip>;
        },
      },
      {
        id: 'email', accessorFn: (c) => c.email, header: () => t('pages.clients.client'),
        cell: ({ row }) => (
          <div className="email-cell">
            <span className="email">{row.original.email}</span>
            {row.original.comment && <span className="sub" title={row.original.comment}>{row.original.comment}</span>}
          </div>
        ),
      },
    ];
    if (allGroups.length > 0) {
      cols.push({
        id: 'group', size: 130, accessorFn: (c) => c.group ?? '', header: () => t('pages.clients.group'),
        cell: ({ row }) => {
          const record = row.original;
          if (!record.group) return <span className="ds-muted">—</span>;
          const isActive = filters.groups.includes(record.group);
          return (
            <Tag tone="primary" style={{ cursor: 'pointer', opacity: isActive ? 0.6 : 1 }} onClick={() => { if (!isActive) setFilters({ ...filters, groups: [...filters.groups, record.group!] }); }}>
              {record.group}
            </Tag>
          );
        },
      });
    }
    cols.push(
      {
        id: 'traffic',
        accessorFn: (c) => (c.traffic?.up ?? 0) + (c.traffic?.down ?? 0),
        header: () => t('pages.clients.traffic'),
        cell: ({ row }) => trafficLabel(row.original),
      },
      {
        id: 'expiryTime', accessorFn: (c) => c.expiryTime ?? 0, header: () => t('pages.clients.duration'),
        cell: ({ row }) => <Tooltip title={expiryLabel(row.original)}><Tag tone={tone(expiryColor(row.original))}>{row.original.expiryTime ? expiryRelative(row.original) : '∞'}</Tag></Tooltip>,
      },
    );
    return cols;
  }, [t, togglingEmail, clientBucket, isOnline, inboundsById, filters, allGroups, datepicker]);

  function clearOneFilter<K extends keyof ClientFilters>(key: K) {
    if (key === 'expiryFrom' || key === 'expiryTo') { setFilters({ ...filters, expiryFrom: undefined, expiryTo: undefined }); return; }
    if (key === 'usageFromGB' || key === 'usageToGB') { setFilters({ ...filters, usageFromGB: undefined, usageToGB: undefined }); return; }
    setFilters({ ...filters, [key]: emptyFilters()[key] });
  }

  function closableChip(key: string, t0: TagTone, label: React.ReactNode, onClose: () => void) {
    return (
      <Tag key={key} tone={t0} className="filter-chip">
        {label}
        <button type="button" className="filter-chip__x" aria-label={t('delete')} onClick={onClose}>×</button>
      </Tag>
    );
  }

  const presetSortValue = useMemo(() => {
    const value = sortValueFor(sortColumn, sortOrder);
    return SORT_OPTIONS.some((o) => o.value === value) ? value : undefined;
  }, [sortColumn, sortOrder]);

  const tableSorting = useMemo<SortingState>(
    () => (sortColumn && sortOrder ? [{ id: sortColumn, desc: sortOrder === 'descend' }] : []),
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

  function statWithList(title: string, value: number, icon: React.ReactNode, emails: string[]) {
    const stat = <Stat title={title} value={String(value)} prefix={icon} />;
    if (emails.length === 0) return stat;
    return (
      <Popover
        side="bottom"
        trigger={<button type="button" className="stat-trigger">{stat}</button>}
        content={<div className="client-email-list">{emails.map((e) => <div key={e}>{e}</div>)}</div>}
      />
    );
  }

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
          <div style={{ display: 'flex', flexDirection: 'column', gap: isMobile ? 8 : 12 }}>
            <Card>
              <div className="ds-stats-grid">
                <Stat title={t('clients')} value={String(summary.total)} prefix={<TeamOutlined />} />
                {statWithList(t('online'), summary.online.length, <WifiOutlined />, summary.online)}
                {statWithList(t('depleted'), summary.depleted.length, <CloseCircleOutlined />, summary.depleted)}
                {statWithList(t('depletingSoon'), summary.expiring.length, <WarningOutlined />, summary.expiring)}
                {statWithList(t('disabled'), summary.deactive.length, <StopOutlined />, summary.deactive)}
                <Stat title={t('subscription.active')} value={String(summary.active)} prefix={<CheckCircleOutlined />} />
              </div>
            </Card>

            <div className="clients-add">
              <div className="vertical-tabs-container">
                <button type="button" className="vtab-btn is-active" onClick={onAdd}>
                  <span className="vtab-icon"><PlusOutlined /></span>
                  {t('pages.clients.addClients')}
                </button>
              </div>
            </div>

            <Card flush>
              {/* One row: what you do, what you look for, and where you are in
                  the list — the search and sort had a strip of their own with
                  nothing else on it. */}
              <div className={`card-toolbar${isMobile ? ' is-stacked' : ''}`}>
                <div className="toolbar-search">
                  <SearchOutlined className="toolbar-search__icon" />
                  <Input value={searchKey} onChange={(e) => setSearchKey(e.target.value)} placeholder={t('pages.clients.searchPlaceholder')} />
                </div>

                <div className="toolbar-sort">
                  <Select
                    value={presetSortValue}
                    placeholder={t('pages.clients.sortCustom')}
                    onChange={(value) => { const opt = SORT_OPTIONS.find((o) => o.value === value); setSortColumn(opt?.column ?? null); setSortOrder(opt?.order ?? null); }}
                    options={SORT_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                  />
                </div>

                <div className="toolbar-pages">
                  <Pagination {...pagination} />
                </div>
              </div>

              {(activeCount > 0 || debouncedSearch.trim().length > 0) && (
                <div className="filter-chips">
                  {/* Both belong to the filtering, so they sit on the filter row
                      rather than among the controls above. */}
                  <span className="filter-count">{t('pages.clients.showingCount', { shown: filtered, total })}</span>
                  {activeCount > 0 && (
                    <Button size="sm" onClick={() => setFilters(emptyFilters())}>
                      {t('pages.clients.clearAllFilters')}
                    </Button>
                  )}
                  {filters.buckets.map((b) => closableChip(`b-${b}`, 'neutral', bucketChipLabel(b, t), () => setFilters({ ...filters, buckets: filters.buckets.filter((x) => x !== b) })))}
                  {filters.protocols.map((p) => closableChip(`p-${p}`, 'primary', p, () => setFilters({ ...filters, protocols: filters.protocols.filter((x) => x !== p) })))}
                  {filters.inboundIds.map((id) => closableChip(`i-${id}`, 'primary', inboundLabel(id), () => setFilters({ ...filters, inboundIds: filters.inboundIds.filter((x) => x !== id) })))}
                  {filters.groups.map((g) => closableChip(`g-${g}`, 'primary', `${t('pages.clients.group')}: ${g}`, () => setFilters({ ...filters, groups: filters.groups.filter((x) => x !== g) })))}
                  {(filters.expiryFrom || filters.expiryTo) && closableChip('exp', 'primary', `${t('pages.clients.expiryTime')}: ${filters.expiryFrom ? IntlUtil.formatDate(filters.expiryFrom, datepicker) : '…'} → ${filters.expiryTo ? IntlUtil.formatDate(filters.expiryTo, datepicker) : '…'}`, () => clearOneFilter('expiryFrom'))}
                  {(filters.usageFromGB || filters.usageToGB) && closableChip('usage', 'warning', `${t('pages.clients.traffic')}: ${filters.usageFromGB ?? 0}${filters.usageToGB ? `–${filters.usageToGB}` : '+'} GB`, () => clearOneFilter('usageFromGB'))}
                  {filters.autoRenew && closableChip('renew', 'warning', `${t('pages.clients.renew')}: ${filters.autoRenew === 'on' ? t('enabled') : t('disabled')}`, () => clearOneFilter('autoRenew'))}
                  {filters.hasTgId && closableChip('tg', 'neutral', `${t('pages.clients.telegramId')}: ${filters.hasTgId === 'yes' ? t('pages.clients.has') : t('pages.clients.hasNot')}`, () => clearOneFilter('hasTgId'))}
                  {filters.hasComment && closableChip('cm', 'neutral', `${t('pages.clients.comment')}: ${filters.hasComment === 'yes' ? t('pages.clients.has') : t('pages.clients.hasNot')}`, () => clearOneFilter('hasComment'))}
                </div>
              )}

              {!isMobile ? (
                <div className="client-table" style={{ padding: '0 4px 4px' }}>
                  <Spin spinning={loading}>
                    <DataTable
                      data={sortedClients}
                      columns={columns}
                      getRowId={(c) => c.email}
                      sorting={tableSorting}
                      onSortingChange={onTableSortingChange}
                      empty={<><TeamOutlined style={{ fontSize: 32, marginBottom: 8 }} /><div>{t('noData')}</div></>}
                    />
                  </Spin>
                </div>
              ) : (
                <Spin spinning={loading}>
                  <div className="client-cards" style={{ padding: '0 12px 12px' }}>
                    {filteredClients.length === 0 && (
                      <div className="card-empty"><TeamOutlined style={{ fontSize: 28, opacity: 0.5 }} /><div>{t('noData')}</div></div>
                    )}
                    {filteredClients.map((row) => {
                      const bucket = clientBucket(row);
                      return (
                        <div key={row.email} className="client-card">
                          <div className="card-head">
                            <span className={bucketDotClass(bucket)} />
                            <span className="tag-name">{row.email}</span>
                            {bucket === 'depleted' && <Tag tone="danger" className="status-tag">{t('depleted')}</Tag>}
                            {bucket === 'expiring' && <Tag tone="warning" className="status-tag">{t('depletingSoon')}</Tag>}
                            <div className="card-actions" onClick={(e) => e.stopPropagation()}>
                              <Tooltip title={t('pages.clients.clientInfo')}>
                                <button type="button" className="row-action-trigger" onClick={() => onShowInfo(row)}><InfoCircleOutlined /></button>
                              </Tooltip>
                              <Switch checked={!!row.enable} onChange={(next) => onToggleEnable(row, next)} />
                              <DropdownMenu
                                items={[
                                  { key: 'reset', icon: <RetweetOutlined />, label: t('pages.inbounds.resetTraffic'), onSelect: () => onResetTraffic(row) },
                                  { key: 'edit', icon: <EditOutlined />, label: t('edit'), onSelect: () => onEdit(row) },
                                  { key: 'delete', icon: <DeleteOutlined />, label: t('delete'), danger: true, onSelect: () => onDelete(row) },
                                ]}
                                trigger={<button type="button" className="row-action-trigger"><MoreOutlined /></button>}
                              />
                            </div>
                          </div>
                        </div>
                      );
                    })}
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
          <ClientFormModal open={formOpen} mode={formMode} client={editingClient} attachedIds={editingAttachedIds} inbounds={inbounds} tgBotEnable={tgBotEnable} groups={allGroups} save={onSave} onOpenChange={setFormOpen} />
        </LazyMount>
        <LazyMount when={infoOpen}>
          <ClientInfoModal
            open={infoOpen}
            client={liveInfoClient}
            isOnline={liveInfoClient ? isOnline(liveInfoClient.email) : false}
            onDevice={async (action, fingerprint, blocked) => {
              if (!infoClient?.email) return;
              await HttpUtil.post(`/panel/api/clients/devices/${action}`,
                { email: infoClient.email, fingerprint, blocked },
                { headers: { 'Content-Type': 'application/json' } });
              await refresh();
            }}
            onAddress={async (ip) => {
              if (!infoClient?.email) return;
              await HttpUtil.post('/panel/api/clients/addresses/forget',
                { email: infoClient.email, ip },
                { headers: { 'Content-Type': 'application/json' } });
              await refresh();
            }}
            onExit={async (nodeId) => {
              if (!infoClient?.email) return;
              await HttpUtil.post('/panel/api/clients/exits/forget',
                { email: infoClient.email, nodeId },
                { headers: { 'Content-Type': 'application/json' } });
              await refresh();
            }}
            onOpenChange={setInfoOpen}
          />
        </LazyMount>
      </div>
    </TooltipProvider>
  );
}
