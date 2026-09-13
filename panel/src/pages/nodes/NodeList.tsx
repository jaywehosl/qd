import { useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CloudSyncOutlined,
  ClusterOutlined,
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  InfoCircleOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import {
  Button,
  Card,
  DataTable,
  Dialog,
  DropdownMenu,
  Switch,
  Tag,
  Tooltip,
  TooltipProvider,
  type ColumnDef,
  type MenuEntry,
} from '@/components/ds';
import { usePublish } from '@/layouts/PublishController';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
interface NodeListProps {
  nodes: NodeRecord[];
  loading?: boolean;
  isMobile?: boolean;
  onAdd: () => void;
  onEdit: (node: NodeRecord) => void;
  onDelete: (node: NodeRecord) => void;
  onProbe: (node: NodeRecord) => void;
  onSync: (node: NodeRecord) => void;
  onToggleEnable: (node: NodeRecord, next: boolean) => void;
}

interface NodeRow extends NodeRecord {
  url: string;
  key: number;
}

function HStack({ gap = 8, children }: { gap?: number; children: ReactNode }) {
  return <span style={{ display: 'inline-flex', alignItems: 'center', gap }}>{children}</span>;
}

function StatusDot({ status }: { status?: string }) {
  if (status === 'online') return <span className="online-dot" />;
  const color = status === 'offline'
    ? 'var(--color-error)'
    : status === 'waiting' ? 'var(--color-warning)' : 'var(--text-3)';
  return <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', background: color }} />;
}

function StatusLabel({ status }: { status?: string }) {
  const { t } = useTranslation();
  const color = status === 'online'
    ? 'var(--color-success)'
    : status === 'waiting' ? 'var(--color-warning)' : undefined;
  return (
    <span style={color ? { color } : undefined}>
      {t(`pages.nodes.statusValues.${status || 'unknown'}`)}
    </span>
  );
}

function formatPct(p?: number): string {
  if (typeof p !== 'number' || Number.isNaN(p)) return '-';
  return `${p.toFixed(1)}%`;
}

function formatUptime(secs?: number): string {
  if (!secs) return '-';
  const days = Math.floor(secs / 86400);
  const hours = Math.floor((secs % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  const mins = Math.floor((secs % 3600) / 60);
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function useRelativeTime() {
  const { t } = useTranslation();
  return (unixSeconds?: number) => {
    if (!unixSeconds) return t('pages.nodes.never');
    const diffSec = Math.max(0, Math.floor(Date.now() / 1000 - unixSeconds));
    if (diffSec < 5) return t('pages.nodes.justNow');
    if (diffSec < 60) return `${diffSec}s`;
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h`;
    return `${Math.floor(diffSec / 86400)}d`;
  };
}

export default function NodeList({
  nodes,
  isMobile = false,
  onAdd,
  onEdit,
  onDelete,
  onProbe,
  onSync,
  onToggleEnable,
}: NodeListProps) {
  const { t } = useTranslation();
  const relativeTime = useRelativeTime();
  const { draft } = usePublish();
  const published = draft?.publishedRevision ?? 0;

  const [showAddress, setShowAddress] = useState(false);
  const [statsNode, setStatsNode] = useState<NodeRow | null>(null);

  const dataSource = useMemo<NodeRow[]>(
    () => nodes
      .map((n) => ({
        ...n,
        url: `qd://${n.address}:${n.port}`,
        key: n.id,
      }))
      .sort((a, b) => {
        const side = (n: NodeRow) => ((n as { role?: string }).role === 'egress' ? 1 : 0);
        return side(a) - side(b) || (a.name || '').localeCompare(b.name || '');
      }),
    [nodes],
  );

  const columns = useMemo<ColumnDef<NodeRow, unknown>[]>(() => [
    {
      id: 'actions',
      header: () => t('pages.nodes.actions'),
      cell: ({ row }) => {
        const record = row.original;
        return (
          <div className="row-actions">
            <Tooltip title={t('edit')}>
              <Button size="sm" icon={<EditOutlined />} onClick={() => onEdit(record)} />
            </Tooltip>
            <Tooltip title={t('pages.nodes.syncNode', { defaultValue: 'Copy the network database onto this node' })}>
              <Button size="sm" icon={<CloudSyncOutlined />} onClick={() => onSync(record)} />
            </Tooltip>
            <Tooltip title={t('delete')}>
              <Button size="sm" className="row-delete" icon={<DeleteOutlined />} onClick={() => onDelete(record)} />
            </Tooltip>
          </div>
        );
      },
    },
    {
      id: 'enable',
      size: 80,
      accessorFn: (n: NodeRow) => (n.enable ? 1 : 0),
      header: () => t('pages.nodes.enable'),
      cell: ({ row }) => (
        <Switch checked={!!row.original.enable} onChange={(v) => onToggleEnable(row.original, v)} />
      ),
    },
    {
      id: 'name',
      accessorFn: (n: NodeRow) => n.name || '',
      header: () => t('pages.nodes.name'),
      cell: ({ row }) => (
        <div className="name-cell">
          <span className="name">{row.original.name}</span>
          {row.original.remark && <span className="remark">{row.original.remark}</span>}
        </div>
      ),
    },
    {
      id: 'url',
      accessorFn: (n: NodeRow) => n.url,
      header: () => (
        <span className="address-header">
          {t('pages.nodes.address')}
          <Tooltip title={t('pages.index.toggleIpVisibility')}>
            {showAddress ? (
              <EyeOutlined className="ip-toggle-icon" onClick={() => setShowAddress(false)} />
            ) : (
              <EyeInvisibleOutlined className="ip-toggle-icon" onClick={() => setShowAddress(true)} />
            )}
          </Tooltip>
        </span>
      ),
      cell: ({ row }) => (
        <span className={showAddress ? 'address-visible' : 'address-hidden'}>{row.original.url}</span>
      ),
    },
    {
      id: 'status',
      accessorFn: (n: NodeRow) => n.status || '',
      header: () => t('pages.nodes.status'),
      cell: ({ row }) => {
        const record = row.original;
        return (
          <HStack gap={6}>
            <StatusDot status={record.status} />
            <StatusLabel status={record.status} />
            {record.lastError && (
              <Tooltip title={record.lastError}>
                <ExclamationCircleOutlined style={{ color: 'var(--color-warning)' }} />
              </Tooltip>
            )}
            {record.status === 'offline' && (
              <Button size="sm" icon={<ReloadOutlined />} onClick={() => onProbe(record)}>
                {t('pages.nodes.reconnect')}
              </Button>
            )}
          </HStack>
        );
      },
    },
    {
      id: 'role',
      accessorFn: (n: NodeRow) => ((n as { role?: string }).role === 'egress' ? 1 : 0),
      header: () => t('pages.nodes.role', { defaultValue: 'Role' }),
      cell: ({ row }) => (row.original as { role?: string }).role || '-',
    },
    {
      id: 'revision',
      accessorFn: (n: NodeRow) => n.appliedRevision || 0,
      header: () => t('pages.nodes.revision'),
      cell: ({ row }) => {
        const applied = row.original.appliedRevision || 0;
        if (!published) return <Tag>{applied || '—'}</Tag>;
        if (!applied) return <Tag tone="warning">{t('pages.nodes.neverApplied')}</Tag>;
        if (applied >= published) return <Tag tone="success">{applied}</Tag>;
        return (
          <Tooltip title={t('pages.nodes.behindBy', { n: published - applied, current: published })}>
            <Tag tone="warning">{applied} / {published}</Tag>
          </Tooltip>
        );
      },
    },
    {
      id: 'uptime',
      accessorFn: (n: NodeRow) => n.uptimeSecs || 0,
      header: () => t('pages.nodes.uptime'),
      cell: ({ row }) => formatUptime(row.original.uptimeSecs),
    },
    {
      id: 'clients',
      accessorFn: (n: NodeRow) => n.onlineCount || 0,
      header: () => t('clients'),
      cell: ({ row }) => {
        const online = row.original.onlineCount || 0;
        const total = row.original.clientCount || 0;
        return (
          <Tag tone={online > 0 ? 'primary' : 'neutral'}>
            {online} / {total}
          </Tag>
        );
      },
    },
    {
      id: 'latency',
      accessorFn: (n: NodeRow) => n.latencyMs || 0,
      header: () => t('pages.nodes.latency'),
      cell: ({ row }) => {
        const record = row.original;
        const ms = record.latencyMs || 0;
        return (
          <HStack gap={6}>
            <Tooltip title={t('pages.nodes.probe')}>
              <Button size="sm" icon={<ThunderboltOutlined />} onClick={() => onProbe(record)} />
            </Tooltip>
            <Tooltip title={`${t('pages.nodes.lastHeartbeat')}: ${relativeTime(record.lastHeartbeat)}`}>
              <Tag tone={ms > 0 ? 'success' : 'neutral'}>{ms > 0 ? `${ms} ms` : '—'}</Tag>
            </Tooltip>
          </HStack>
        );
      },
    },
  ], [t, showAddress, relativeTime, published, onToggleEnable, onProbe, onSync, onEdit, onDelete]);

  function mobileMenu(record: NodeRow): MenuEntry[] {
    return [
      { key: 'probe', icon: <ThunderboltOutlined />, label: t('pages.nodes.probe'), onSelect: () => onProbe(record) },
      { key: 'edit', icon: <EditOutlined />, label: t('edit'), onSelect: () => onEdit(record) },
      { key: 'sync', icon: <CloudSyncOutlined />, label: t('pages.nodes.syncNode', { defaultValue: 'Copy the network database onto this node' }), onSelect: () => onSync(record) },
      { key: 'delete', icon: <DeleteOutlined />, label: t('delete'), danger: true, onSelect: () => onDelete(record) },
    ];
  }

  return (
    <TooltipProvider>
      <>
      <div className="clients-add">
        <div className="vertical-tabs-container">
          <button type="button" className="vtab-btn is-active" onClick={onAdd}>
            <span className="vtab-icon"><PlusOutlined /></span>
            {t('pages.nodes.addNode')}
          </button>
        </div>
      </div>

      <Card flush>
        {isMobile ? (
          <>
            <div className="node-cards">
              {dataSource.length === 0 ? (
                <div className="card-empty">
                  <ClusterOutlined style={{ fontSize: 28, opacity: 0.5 }} />
                  <div>{t('noData')}</div>
                </div>
              ) : (
                dataSource.map((record) => (
                  <div key={record.id} className="node-card">
                    <div className="card-head">
                      <StatusDot status={record.status} />
                      <span className="node-name">{record.name}</span>
                      <div className="card-actions" onClick={(e) => e.stopPropagation()}>
                        <Tooltip title={t('info')}>
                          <InfoCircleOutlined className="row-action-trigger" onClick={() => setStatsNode(record)} />
                        </Tooltip>
                        <Switch checked={!!record.enable} onChange={(v) => onToggleEnable(record, v)} />
                        <DropdownMenu
                          items={mobileMenu(record)}
                          trigger={<MoreOutlined className="row-action-trigger" />}
                        />
                      </div>
                    </div>

                  </div>
                ))
              )}
            </div>

            <Dialog
              open={!!statsNode}
              onOpenChange={(o) => !o && setStatsNode(null)}
              width={360}
              title={statsNode?.name || ''}
            >
              {statsNode && (
                <div className="card-stats">
                  {statsNode.remark && (
                    <div className="stat-row">
                      <span className="stat-label">{t('pages.nodes.name')}</span>
                      <span>{statsNode.remark}</span>
                    </div>
                  )}
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.address')}</span>
                    <span className={showAddress ? 'address-visible' : 'address-hidden'}>{statsNode.url}</span>
                    <Tooltip title={t('pages.index.toggleIpVisibility')}>
                      {showAddress ? (
                        <EyeOutlined className="ip-toggle-icon" onClick={() => setShowAddress(false)} />
                      ) : (
                        <EyeInvisibleOutlined className="ip-toggle-icon" onClick={() => setShowAddress(true)} />
                      )}
                    </Tooltip>
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.status')}</span>
                    <StatusDot status={statsNode.status} />
                    <StatusLabel status={statsNode.status} />
                    {statsNode.lastError && (
                      <Tooltip title={statsNode.lastError}>
                        <ExclamationCircleOutlined style={{ color: 'var(--color-warning)' }} />
                      </Tooltip>
                    )}
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.cpu')}</span>
                    <Tag>{formatPct(statsNode.cpuPct)}</Tag>
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.mem')}</span>
                    <Tag>{formatPct(statsNode.memPct)}</Tag>
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.uptime')}</span>
                    <Tag>{formatUptime(statsNode.uptimeSecs)}</Tag>
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.revision')}</span>
                    {(() => {
                      const applied = statsNode.appliedRevision || 0;
                      if (!published) return <Tag>{applied || '—'}</Tag>;
                      if (!applied) return <Tag tone="warning">{t('pages.nodes.neverApplied')}</Tag>;
                      if (applied >= published) return <Tag tone="success">{applied}</Tag>;
                      return <Tag tone="warning">{applied} / {published}</Tag>;
                    })()}
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.latency')}</span>
                    <Tag>{statsNode.latencyMs && statsNode.latencyMs > 0 ? `${statsNode.latencyMs} ms` : '-'}</Tag>
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('clients')}</span>
                    <Tag tone={(statsNode.onlineCount || 0) > 0 ? 'primary' : 'neutral'}>{statsNode.onlineCount || 0}</Tag>
                  </div>
                  <div className="stat-row">
                    <span className="stat-label">{t('pages.nodes.lastHeartbeat')}</span>
                    <Tag>{relativeTime(statsNode.lastHeartbeat)}</Tag>
                  </div>
                </div>
              )}
            </Dialog>
          </>
        ) : (
          <div className="node-table" style={{ padding: '0 4px 4px' }}>
            <DataTable<NodeRow>
              data={dataSource}
              columns={columns}
              getRowId={(n) => String(n.id)}
              empty={
                <>
                  <ClusterOutlined style={{ fontSize: 32, marginBottom: 8 }} />
                  <div>{t('noData')}</div>
                </>
              }
            />
          </div>
        )}
      </Card>
      </>
    </TooltipProvider>
  );
}
