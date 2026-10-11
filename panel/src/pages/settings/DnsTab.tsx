import { Fragment, useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';

import { Button, Card, DataTable, Dialog, Field, Input, Switch, Tag, toast, type ColumnDef } from '@/components/ds';
import { SettingListItem } from '@/components/ui';
import { HttpUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';

interface DnsTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

interface DnsRecord {
  id: number;
  suffix: string;
  v4: string;
  v6: string;
  comment: string;
  enable: boolean;
  allow: boolean;
}

interface DnsNodeStats {
  nodeId: number;
  tag: string;
  queries: number;
  hits: number;
  upstream: number;
  failed: number;
  records: number;
  evicted: number;
  entries: number;
  size: number;
  blocked?: number;
}

interface HeldList {
  id: string;
  title: string;
  entries: number;
  version: string;
  modified: string;
  fetched: number;
  error: string;
}

interface HeldRow extends HeldList {
  key: string;
  tag: string;
  first: boolean;
  idle: boolean;
}

interface NodeLists {
  nodeId: number;
  tag: string;
  lists: HeldList[];
}

const THREATS = 'tif';

const COLUMNS = [
  { id: 'light', title: 'Light' },
  { id: 'normal', title: 'Normal' },
  { id: 'pro', title: 'Pro' },
  { id: 'proplus', title: 'Pro++' },
  { id: 'ultimate', title: 'Ultimate' },
  { id: THREATS, title: 'TIF' },
];

const COVERAGE = [
  { key: 'fake', levels: [0, 4, 4, 4, 4, 4] },
  { key: 'popups', levels: [1, 2, 4, 4, 4, 2] },
  { key: 'threats', levels: [0, 1, 1, 2, 2, 4] },
  { key: 'native', levels: [1, 1, 2, 3, 4, 0] },
  { key: 'crash', levels: [0, 0, 4, 4, 4, 0] },
  { key: 'referral', levels: [0, 0, 1, 2, 2, 0] },
];

const blank: DnsRecord = { id: 0, suffix: '', v4: '', v6: '', comment: '', enable: true, allow: false };

export default function DnsTab({ allSetting, updateSetting }: DnsTabProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState<DnsRecord | null>(null);
  const { data: held = [] } = useQuery<NodeLists[]>({
    queryKey: ['dns', 'lists'],
    queryFn: async () => {
      const msg = await HttpUtil.get<NodeLists[]>('/panel/api/dns/lists', undefined, { silent: true });
      return msg?.success ? (msg.obj ?? []).sort((a, b) => a.nodeId - b.nodeId) : [];
    },
    refetchInterval: 10000,
  });
  const on = (id: string) => (id === THREATS ? !!allSetting.blockTif : allSetting.blockTier === id);

  const { data: records = [], isFetching } = useQuery<DnsRecord[]>({
    queryKey: ['dns', 'records'],
    queryFn: async () => {
      const msg = await HttpUtil.get<DnsRecord[]>('/panel/api/dns/records', undefined, { silent: true });
      return msg?.success ? (msg.obj ?? []) : [];
    },
  });

  const { data: stats = [] } = useQuery<DnsNodeStats[]>({
    queryKey: ['dns', 'stats'],
    queryFn: async () => {
      const msg = await HttpUtil.get<DnsNodeStats[]>('/panel/api/dns/stats', undefined, { silent: true });
      return msg?.success ? (msg.obj ?? []).sort((a, b) => a.nodeId - b.nodeId) : [];
    },
    refetchInterval: 10000,
  });

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['dns'] });
  }, [queryClient]);

  const save = useMutation({
    mutationFn: async (row: DnsRecord) => HttpUtil.post('/panel/api/dns/records/save', row),
    onSuccess: (msg) => {
      if (!msg?.success) return;
      setEditing(null);
      refresh();
    },
  });

  const remove = useMutation({
    mutationFn: async (id: number) => HttpUtil.post('/panel/api/dns/records/del', { id }),
    onSuccess: () => refresh(),
  });

  const flush = useMutation({
    mutationFn: async () => HttpUtil.post('/panel/api/dns/flush', {}),
    onSuccess: (msg) => {
      if (msg?.success) toast.success(t('pages.settings.dnsFlushed'));
      refresh();
    },
  });

  const pull = useMutation({
    mutationFn: async () => HttpUtil.post('/panel/api/dns/lists/refresh', {}),
    onSuccess: () => refresh(),
  });

  const heldRows = useMemo<HeldRow[]>(() => held.flatMap((node): HeldRow[] => (node.lists.length > 0
    ? node.lists.map((l, i) => ({ ...l, key: `${node.nodeId}:${l.id}`, tag: node.tag, first: i === 0, idle: false }))
    : [{
      key: String(node.nodeId), tag: node.tag, first: true, idle: true,
      id: '', title: '—', entries: 0, version: '', modified: '', fetched: 0, error: '',
    }])), [held]);

  const listColumns = useMemo<ColumnDef<HeldRow, unknown>[]>(() => [
    {
      id: 'node',
      header: t('pages.settings.blockNode'),
      cell: ({ row }) => (row.original.first
        ? <Tag tone="success" className="dns-stats__node">{row.original.tag}</Tag>
        : null),
    },
    { id: 'list', header: t('pages.settings.blockList'), cell: ({ row }) => <b>{row.original.title}</b> },
    {
      id: 'names',
      header: t('pages.settings.blockNames'),
      cell: ({ row }) => (row.original.entries > 0 ? row.original.entries.toLocaleString() : '—'),
    },
    { id: 'version', header: t('pages.settings.blockVersion'), cell: ({ row }) => row.original.version || '—' },
    {
      id: 'checked',
      header: t('pages.settings.blockChecked'),
      cell: ({ row }) => (row.original.fetched > 0
        ? new Date(row.original.fetched * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        : '—'),
    },
    {
      id: 'state',
      header: t('pages.settings.blockState'),
      cell: ({ row }) => {
        const l = row.original;
        if (l.error) {
          return (
            <Tag tone="warning" title={l.error}>
              {t(l.entries > 0 ? 'pages.settings.blockStale' : 'pages.settings.blockFailed')}
            </Tag>
          );
        }
        if (l.idle) return t('pages.settings.blockIdle');
        return t(l.entries > 0 ? 'pages.settings.blockInForce' : 'pages.settings.blockLoading');
      },
    },
  ], [t]);

  const columns = useMemo<ColumnDef<DnsRecord, unknown>[]>(() => [
    {
      header: t('pages.settings.dnsRecordName'),
      accessorKey: 'suffix',
      cell: ({ row }) => (
        <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span>{row.original.suffix}</span>
          {!row.original.enable && <Tag tone="warning">off</Tag>}
        </span>
      ),
    },
    { header: 'IPv4', accessorKey: 'v4' },
    { header: 'IPv6', accessorKey: 'v6' },
    { header: t('pages.settings.dnsRecordComment'), accessorKey: 'comment' },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => (
        <span style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
          <Button size="sm" variant="text" onClick={() => setEditing({ ...row.original })}>
            <EditOutlined />
          </Button>
          <Button
            size="sm"
            variant="text"
            danger
            onClick={() => remove.mutate(row.original.id)}
          >
            <DeleteOutlined />
          </Button>
        </span>
      ),
    },
  ], [t, remove]);

  const allowColumns = useMemo(
    () => columns.filter((c) => !('accessorKey' in c) || (c.accessorKey !== 'v4' && c.accessorKey !== 'v6')),
    [columns],
  );
  const own = useMemo(() => records.filter((r) => !r.allow), [records]);
  const spared = useMemo(() => records.filter((r) => r.allow), [records]);

  return (
    <div className="dns-tab">
      <Card title={t('pages.settings.dns', { defaultValue: 'Resolver' })}>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.dnsPrimary')}
        description={t('pages.settings.dnsPrimaryDesc')}
      >
        <Input
          value={allSetting.dnsPrimary ?? ''}
          placeholder="1.1.1.1"
          onChange={(e) => updateSetting({ dnsPrimary: e.target.value })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.dnsSecondary')}
        description={t('pages.settings.dnsSecondaryDesc')}
      >
        <Input
          value={allSetting.dnsSecondary ?? ''}
          placeholder="2606:4700:4700::1111"
          onChange={(e) => updateSetting({ dnsSecondary: e.target.value })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.dnsCache')}
        description={t('pages.settings.dnsCacheDesc')}
      >
        <Input
          type="number"
          min={16}
          max={1000000}
          value={allSetting.dnsCache ?? 4096}
          onChange={(e) => updateSetting({ dnsCache: Number(e.target.value) || 16 })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.dnsTtl')}
        description={t('pages.settings.dnsTtlDesc')}
      >
        <div className="setting-pair">
          <Input
            type="number"
            min={0}
            value={allSetting.dnsMinTtl ?? 10}
            onChange={(e) => updateSetting({ dnsMinTtl: Number(e.target.value) || 0 })}
          />
          <Input
            type="number"
            min={1}
            value={allSetting.dnsMaxTtl ?? 3600}
            onChange={(e) => updateSetting({ dnsMaxTtl: Number(e.target.value) || 1 })}
          />
        </div>
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.dnsStale')}
        description={t('pages.settings.dnsStaleDesc')}
      >
        <Input
          type="number"
          min={0}
          value={allSetting.dnsStale ?? 60}
          onChange={(e) => updateSetting({ dnsStale: Number(e.target.value) || 0 })}
        />
      </SettingListItem>
      </Card>

      <Card
        title={t('pages.settings.blockTitle')}
        extra={(
          <Button size="sm" variant="text" onClick={() => pull.mutate()} disabled={pull.isPending}>
            <ReloadOutlined /> {t('pages.settings.blockRefresh')}
          </Button>
        )}
      >
        <p className="dns-route__note">{t('pages.settings.blockDesc')}</p>
        <div className="bl-scroll">
          <div className="bl-matrix">
            <span />
            {COLUMNS.map((c) => (
              <button
                key={c.id}
                type="button"
                className={`bl-col${on(c.id) ? ' is-on' : ''}`}
                aria-pressed={on(c.id)}
                onClick={() => updateSetting(c.id === THREATS
                  ? { blockTif: !allSetting.blockTif }
                  : { blockTier: allSetting.blockTier === c.id ? '' : c.id })}
              >
                {c.title}
              </button>
            ))}
            <span className="bl-col bl-col--now">{t('pages.settings.blockNow')}</span>
            {COVERAGE.map((row) => (
              <Fragment key={row.key}>
                <span className="bl-row">{t(`pages.settings.blockCovers.${row.key}`)}</span>
                {COLUMNS.map((c, i) => (
                  <span key={c.id} className={`bl-cell${on(c.id) ? ' is-on' : ''}`}>
                    <i className="bl-dot" data-level={row.levels[i]} />
                  </span>
                ))}
                <span className="bl-cell bl-cell--now">
                  <i
                    className="bl-dot"
                    data-level={Math.max(0, ...COLUMNS.map((c, i) => (on(c.id) ? row.levels[i] : 0)))}
                  />
                </span>
              </Fragment>
            ))}
          </div>
        </div>
        <div className="bl-legend">
          <span><i className="bl-dot" data-level={4} /> {t('pages.settings.blockFull')}</span>
          <span><i className="bl-dot" data-level={2} /> {t('pages.settings.blockPartial')}</span>
          <span><i className="bl-dot" data-level={0} /> {t('pages.settings.blockNone')}</span>
        </div>

        <div className="dns-table bl-table">
          <DataTable
            data={heldRows}
            columns={listColumns}
            getRowId={(row) => row.key}
            sortable={false}
            empty={t('pages.settings.blockWaiting')}
          />
        </div>

        <div className="bl-allow">
          <span className="dns-route__note">{t('pages.settings.blockAllow')}</span>
          <Button size="sm" onClick={() => setEditing({ ...blank, allow: true })}>
            <PlusOutlined /> {t('pages.settings.blockAllowAdd')}
          </Button>
        </div>
        <div className="dns-table">
          <DataTable
            data={spared}
            columns={allowColumns}
            getRowId={(row) => String(row.id)}
            empty={isFetching ? '…' : t('pages.settings.blockAllowNone')}
          />
        </div>
      </Card>

      <Card
        title={t('pages.settings.dnsRecords')}
        extra={(
          <span className="dns-head__actions">
          <Button size="sm" onClick={() => setEditing({ ...blank })}>
            <PlusOutlined /> {t('pages.settings.dnsRecordAdd')}
          </Button>
          <Button size="sm" variant="text" onClick={() => flush.mutate()} disabled={flush.isPending}>
            <ReloadOutlined /> {t('pages.settings.dnsFlush')}
          </Button>
          </span>
        )}
      >
      <div className="dns-table">
        <DataTable
          data={own}
          columns={columns}
          getRowId={(row) => String(row.id)}
          empty={isFetching ? '…' : t('pages.settings.dnsNoRecords')}
        />
      </div>

      {stats.length > 0 && (
        <div className="dns-stats">
          {stats.map((s) => (
            <div key={s.nodeId} className="dns-stats__row">
              <Tag tone="success" className="dns-stats__node">{s.tag}</Tag>
              <span className="dns-stats__cell">
                <b>{s.queries}</b> {t('pages.settings.dnsStatQueries')}
              </span>
              <span className="dns-stats__cell">
                <b>{s.queries > 0 ? Math.round((100 * s.hits) / s.queries) : 0}%</b> {t('pages.settings.dnsStatCached')}
              </span>
              <span className="dns-stats__cell">
                <b>{s.entries}/{s.size}</b> {t('pages.settings.dnsStatNames')}
              </span>
              <span className="dns-stats__cell">
                <b>{s.upstream}</b> {t('pages.settings.dnsStatUpstream')}
              </span>
              <span className="dns-stats__cell">
                <b>{s.records}</b> {t('pages.settings.dnsStatOwn')}
              </span>
              <span className="dns-stats__cell">
                <b>{s.blocked ?? 0}</b> {t('pages.settings.dnsStatBlocked')}
              </span>
              <span className="dns-stats__cell">
                <b>{s.failed}</b> {t('pages.settings.dnsStatFailed')}
              </span>
            </div>
          ))}
        </div>
      )}
      </Card>

      <Dialog
        open={editing !== null}
        onOpenChange={(o) => { if (!o) setEditing(null); }}
        title={editing?.allow
          ? t(editing.id ? 'pages.settings.blockAllowEdit' : 'pages.settings.blockAllowAdd')
          : editing?.id ? t('pages.settings.dnsRecordEdit') : t('pages.settings.dnsRecordAdd')}
        okText={t('save')}
        cancelText={t('cancel')}
        okDisabled={!editing?.suffix || (!editing?.allow && !editing?.v4 && !editing?.v6)}
        confirmLoading={save.isPending}
        onOk={() => { if (editing) save.mutate(editing); }}
        autoHeight
        width={480}
      >
        {editing && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <Field label={t('pages.settings.dnsRecordName')}>
              <Input
                value={editing.suffix}
                placeholder={editing.allow ? 'ads.example.com' : 'internal.example.com'}
                onChange={(e) => setEditing({ ...editing, suffix: e.target.value })}
              />
            </Field>
            {!editing.allow && (
              <>
                <Field label="IPv4">
                  <Input
                    value={editing.v4}
                    placeholder="10.0.0.1"
                    onChange={(e) => setEditing({ ...editing, v4: e.target.value })}
                  />
                </Field>
                <Field label="IPv6">
                  <Input
                    value={editing.v6}
                    placeholder="2001:db8::1"
                    onChange={(e) => setEditing({ ...editing, v6: e.target.value })}
                  />
                </Field>
              </>
            )}
            <Field label={t('pages.settings.dnsRecordComment')}>
              <Input
                value={editing.comment}
                onChange={(e) => setEditing({ ...editing, comment: e.target.value })}
              />
            </Field>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Switch
                checked={editing.enable}
                onChange={(v) => setEditing({ ...editing, enable: v })}
              />
              {t('pages.settings.dnsRecordEnable')}
            </label>
          </div>
        )}
      </Dialog>
    </div>
  );
}
