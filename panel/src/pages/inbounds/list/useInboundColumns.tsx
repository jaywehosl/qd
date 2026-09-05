import type React from 'react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { TeamOutlined, RetweetOutlined } from '@ant-design/icons';

import { Button, Popover, Tag, type ColumnDef } from '@/components/ds';
import { SizeFormatter } from '@/utils';

import type { NodeRecord } from '@/api/queries/useNodesQuery';

import type { ClientCountEntry, DBInboundRecord, RowAction } from './types';

interface UseInboundColumnsParams {
  hasActiveNode: boolean;
  nodesById: Map<number, NodeRecord>;
  clientCount: Record<number, ClientCountEntry>;
  subEnable: boolean;
  expireDiff: number;
  trafficDiff: number;
  onRowAction: (action: { key: RowAction; dbInbound: DBInboundRecord }) => void;
  onSwitchEnable: (dbInbound: DBInboundRecord, next: boolean) => void;
  roleLabel?: string;
}

function uptimeText(seconds: number): string {
  if (!seconds || seconds <= 0) return '—';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function useInboundColumns({
  roleLabel,
  nodesById,
  clientCount,
  trafficDiff,
  onRowAction,
}: UseInboundColumnsParams): ColumnDef<DBInboundRecord, unknown>[] {
  const { t } = useTranslation();

  return useMemo<ColumnDef<DBInboundRecord, unknown>[]>(() => {
    const emailList = (title: string, emails: string[], trigger: React.ReactElement) => (
      <Popover
        side="bottom"
        content={
          <div className="client-email-list">
            <div className="ds-popover-title">{title}</div>
            {emails.map((e) => <div key={e}>{e}</div>)}
          </div>
        }
        trigger={trigger}
      />
    );

    const cols: ColumnDef<DBInboundRecord, unknown>[] = [
      {
        id: 'actions',
        header: () => <span className="entry-role-name">{roleLabel ?? ''}</span>,
        cell: ({ row }) => (
          <div className="row-actions">
            <Button
              size="sm"
              title={t('pages.inbounds.resetTraffic', { defaultValue: 'Reset traffic' })}
              onClick={() => onRowAction({ key: 'resetTraffic' as RowAction, dbInbound: row.original })}
            >
              <RetweetOutlined />
            </Button>
          </div>
        ),
      },
    ];

    cols.push({
      id: 'port',
      accessorFn: (r) => r.port ?? 0,
      header: () => t('pages.inbounds.port'),
      cell: ({ row }) => <span className="entry-port">{row.original.port}</span>,
    });

    cols.push({
      id: 'node',
      accessorFn: (r) => {
        const node = nodesById.get((r as unknown as { nodeId?: number }).nodeId ?? 0);
        return node?.name || node?.address || '';
      },
      header: () => t('pages.inbounds.node'),
      cell: ({ row }) => {
        const nodeId = (row.original as unknown as { nodeId?: number }).nodeId ?? 0;
        const node = nodesById.get(nodeId);
        if (!node) return <Tag tone="neutral">—</Tag>;
        return (
          <Tag tone={node.status === 'online' ? 'success' : 'neutral'}>
            {node.name || node.address}
          </Tag>
        );
      },
    });

    cols.push(
      {
        id: 'clients',
        accessorFn: (r) => clientCount[r.id]?.online.length ?? 0,
        header: () => t('online', { defaultValue: 'Online' }),
        cell: ({ row }) => {
          const cc = clientCount[row.original.id];
          const count = cc?.online.length ?? 0;
          const pill = (
            <Tag tone={count > 0 ? 'primary' : 'neutral'} className="client-count-tag">
              <TeamOutlined /> {count}
            </Tag>
          );
          if (count === 0) return pill;
          return emailList(t('online'), cc!.online, pill);
        },
      },
      {
        id: 'traffic',
        accessorFn: (r) => r.up + r.down,
        header: () => t('pages.inbounds.traffic'),
        cell: ({ row }) => (
          <Tag tone="neutral" style={{ whiteSpace: 'nowrap' }}>
            ↑ {SizeFormatter.sizeFormat(row.original.up)} ↓ {SizeFormatter.sizeFormat(row.original.down)}
          </Tag>
        ),
      },
      {
        id: 'uptime',
        accessorFn: (r) => nodesById.get((r as unknown as { nodeId?: number }).nodeId ?? 0)?.uptimeSecs ?? 0,
        header: () => t('pages.inbounds.uptime', { defaultValue: 'Uptime' }),
        cell: ({ row }) => {
          const nodeId = (row.original as unknown as { nodeId?: number }).nodeId ?? 0;
          const node = nodesById.get(nodeId);
          const secs = node?.uptimeSecs ?? 0;
          return <Tag tone={secs > 0 ? 'neutral' : 'warning'}>{uptimeText(secs)}</Tag>;
        },
      },
    );

    return cols;
  }, [t, roleLabel, nodesById, clientCount, trafficDiff, onRowAction]);
}
