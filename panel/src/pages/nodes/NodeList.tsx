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
  PlusOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import {
  Button,
  Card,
  DataTable,
  Dialog,
  Switch,
  Tag,
  Tooltip,
  TooltipProvider,
  type ColumnDef,
} from '@/components/ds';
import { cellOf } from '@/components/ui';
import { usePublish } from '@/layouts/PublishController';
import { useHold } from '@/hooks/useHold';
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

function dotOf(n: NodeRecord) {
  if (!n.enable) return 'disabled';
  if (n.status === 'online') return 'online';
  if (n.status === 'waiting') return 'expiring';
  return 'depleted';
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
  const hold = useHold();
  const [openedId, setOpenedId] = useState<number | null>(null);

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

  const opened = dataSource.find((n) => n.id === openedId) ?? null;

  return (
    <TooltipProvider>
      <>
      {!isMobile && (
        <div className="clients-add">
          <div className="vertical-tabs-container">
            <button type="button" className="vtab-btn is-active" onClick={onAdd}>
              <span className="vtab-icon"><PlusOutlined /></span>
              {t('pages.nodes.addNode')}
            </button>
          </div>
        </div>
      )}

      <Card flush>
        {isMobile ? (
          <>
            <div className="mc-head">
              <span>{t('pages.nodes.totalNodes')}: {dataSource.length}</span>
              <button type="button" className="mc-add" aria-label={t('pages.nodes.addNode')} onClick={onAdd}>
                <PlusOutlined />
              </button>
            </div>
            <div className="mc-list">
              {dataSource.length === 0 && (
                <div className="card-empty">
                  <ClusterOutlined style={{ fontSize: 28, opacity: 0.5 }} />
                  <div>{t('noData')}</div>
                </div>
              )}
              {dataSource.map((record) => {
                const applied = record.appliedRevision || 0;
                const lagging = published > 0 && applied < published;
                const ms = record.latencyMs || 0;
                return (
                  <div key={record.id} className="mc is-tap" {...hold(() => setOpenedId(record.id), () => onEdit(record))}>
                    <span className={`mc__dot is-${dotOf(record)}`} />
                    <div className="mc__body">
                      <span className="mc__name">{record.name}</span>
                      <span className="mc__sub">
                        <span>{(record as { role?: string }).role || '-'}</span>
                        {record.status === 'online' && <span>{formatUptime(record.uptimeSecs)}</span>}
                        <span>{record.onlineCount || 0} / {record.clientCount || 0}</span>
                        <span>{ms > 0 ? `${ms} ms` : t(`pages.nodes.statusValues.${record.status || 'unknown'}`)}</span>
                      </span>
                      {lagging && (
                        <span className="mc__note is-warning">
                          {applied ? t('pages.nodes.behindBy', { n: published - applied, current: published }) : t('pages.nodes.neverApplied')}
                        </span>
                      )}
                    </div>
                    <span className="mc__end" onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
                      <Switch checked={!!record.enable} onChange={(v) => onToggleEnable(record, v)} />
                    </span>
                  </div>
                );
              })}
            </div>
            <Dialog
              open={opened !== null}
              onOpenChange={(o) => !o && setOpenedId(null)}
              title={opened?.name ?? ''}
              autoHeight
              footer={opened && (
                <div className="ms-foot">
                  <Button danger onClick={() => { setOpenedId(null); onDelete(opened); }}>{t('delete')}</Button>
                  <Button icon={<CloudSyncOutlined />} onClick={() => { setOpenedId(null); onSync(opened); }}>
                    {t('pages.nodes.sync', { defaultValue: 'Sync' })}
                  </Button>
                  <Button variant="primary" onClick={() => { setOpenedId(null); onEdit(opened); }}>{t('edit')}</Button>
                </div>
              )}
            >
              {opened && (
                <div className="ms">
                  <div className="ms-status">
                    <span className={`mc__dot is-${dotOf(opened)}`} />
                    <StatusLabel status={opened.status} />
                    {opened.status === 'offline' && (
                      <Button size="sm" icon={<ReloadOutlined />} onClick={() => onProbe(opened)}>{t('pages.nodes.reconnect')}</Button>
                    )}
                    <span className="ms-status__state">{opened.enable ? t('enabled') : t('disabled')}</span>
                  </div>
                  {opened.lastError && (
                    <div className="ms-error"><ExclamationCircleOutlined /> {opened.lastError}</div>
                  )}
                  <section className="ms-sec">
                    <div className="ms-row">
                      <span className="ms-row__key">{t('pages.nodes.address')}</span>
                      <span className="ms-row__val is-mono">
                        <span className={showAddress ? 'address-visible' : 'address-hidden'}>{opened.url}</span>
                      </span>
                      <button type="button" className="mc-icon" aria-label={t('pages.index.toggleIpVisibility')} onClick={() => setShowAddress(!showAddress)}>
                        {showAddress ? <EyeOutlined /> : <EyeInvisibleOutlined />}
                      </button>
                    </div>
                    <div className="ms-row">
                      <span className="ms-row__key">{t('pages.nodes.role')}</span>
                      <span className="ms-row__val">{(opened as { role?: string }).role || '-'}</span>
                    </div>
                    <div className="ms-row">
                      <span className="ms-row__key">{t('pages.nodes.revision')}</span>
                      <span className="ms-row__val">{cellOf(columns, 'revision', opened)}</span>
                    </div>
                    <div className="ms-row">
                      <span className="ms-row__key">{t('pages.nodes.uptime')}</span>
                      <span className="ms-row__val">{formatUptime(opened.uptimeSecs)}</span>
                    </div>
                    <div className="ms-row">
                      <span className="ms-row__key">{t('clients')}</span>
                      <span className="ms-row__val">{opened.onlineCount || 0} / {opened.clientCount || 0}</span>
                    </div>
                    <div className="ms-row">
                      <span className="ms-row__key">{t('pages.nodes.latency')}</span>
                      <span className="ms-row__val">{(opened.latencyMs || 0) > 0 ? `${opened.latencyMs} ms` : '—'}</span>
                      <button type="button" className="mc-icon" aria-label={t('pages.nodes.probe')} onClick={() => onProbe(opened)}>
                        <ThunderboltOutlined />
                      </button>
                    </div>
                    <div className="ms-row">
                      <span className="ms-row__key">{t('pages.nodes.lastHeartbeat')}</span>
                      <span className="ms-row__val">{relativeTime(opened.lastHeartbeat)}</span>
                    </div>
                  </section>
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
