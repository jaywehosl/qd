import { useTranslation } from 'react-i18next';
import { Dialog, Tag, type TagTone } from '@/components/ds';
import { InfinityIcon } from '@/components/ui';

import { SizeFormatter, IntlUtil, ColorUtils } from '@/utils';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

import {
  readStreamHints,
  networkLabel,
  networkL4,
  shadowsocksNetworkLabel,
  tunnelNetworkLabel,
  mixedNetworkLabel,
} from './helpers';
import type { ClientCountEntry, DBInboundRecord } from './types';

interface InboundStatsModalProps {
  open: boolean;
  record: DBInboundRecord | null;
  hasActiveNode: boolean;
  nodesById: Map<number, NodeRecord>;
  clientCount: Record<number, ClientCountEntry>;
  trafficDiff: number;
  expireDiff: number;
  onClose: () => void;
}

function tn(c?: string): TagTone {
  switch (c) {
    case 'green': case 'lime': return 'success';
    case 'red': case 'magenta': case 'volcano': return 'danger';
    case 'gold': case 'orange': return 'warning';
    case 'blue': case 'geekblue': case 'cyan': case 'purple': return 'primary';
    default: return 'neutral';
  }
}

export default function InboundStatsModal({
  open, record, hasActiveNode, nodesById, clientCount, trafficDiff, expireDiff, onClose,
}: InboundStatsModalProps) {
  const { t } = useTranslation();
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      footer={null}
      width={360}
      title={record ? `#${record.id} ${record.remark || ''}`.trim() : ''}
    >
      {record && (
        <div className="card-stats">
          <div className="stat-row">
            <span className="stat-label">{t('pages.inbounds.protocol')}</span>
            <Tag tone="primary">{record.protocol}</Tag>
            {(record.isWireguard || record.isHysteria) && <Tag tone="success">UDP</Tag>}
            {record.isSS && (() => {
              const stream = readStreamHints(record.streamSettings);
              return (
                <>
                  <Tag tone="success">{shadowsocksNetworkLabel(record.settings)}</Tag>
                  {stream.isTls && <Tag tone="primary">TLS</Tag>}
                </>
              );
            })()}
            {record.isTunnel && <Tag tone="success">{tunnelNetworkLabel(record.settings)}</Tag>}
            {record.isMixed && <Tag tone="success">{mixedNetworkLabel(record.settings)}</Tag>}
            {(record.isVMess || record.isVLess || record.isTrojan) && (() => {
              const stream = readStreamHints(record.streamSettings);
              const l4 = networkL4(stream.network);
              return (
                <>
                  <Tag tone="success">{networkLabel(stream.network)}</Tag>
                  {l4 && <Tag tone="success">{l4}</Tag>}
                  {stream.isTls && <Tag tone="primary">TLS</Tag>}
                  {stream.isReality && <Tag tone="primary">Reality</Tag>}
                </>
              );
            })()}
          </div>
          <div className="stat-row">
            <span className="stat-label">{t('pages.inbounds.port')}</span>
            <Tag>{record.port}</Tag>
          </div>
          {hasActiveNode && (
            <div className="stat-row">
              <span className="stat-label">{t('pages.inbounds.node')}</span>
              {record.nodeId == null ? (
                <Tag>{t('pages.inbounds.localPanel')}</Tag>
              ) : nodesById.get(record.nodeId) ? (
                <Tag tone={nodesById.get(record.nodeId)!.status === 'online' ? 'primary' : 'danger'}>
                  {nodesById.get(record.nodeId)!.name}
                </Tag>
              ) : (
                <Tag tone="warning">#{record.nodeId}</Tag>
              )}
            </div>
          )}
          <div className="stat-row">
            <span className="stat-label">{t('pages.inbounds.traffic')}</span>
            <Tag tone={tn(ColorUtils.usageColor(record.up + record.down, trafficDiff, record.total))}>
              {SizeFormatter.sizeFormat(record.up + record.down)} /
              {' '}
              {record.total > 0 ? SizeFormatter.sizeFormat(record.total) : <InfinityIcon />}
            </Tag>
          </div>
          {clientCount[record.id] && (
            <div className="stat-row">
              <span className="stat-label">{t('clients')}</span>
              <Tag tone="success" className="client-count-tag">{clientCount[record.id].clients}</Tag>
              {clientCount[record.id].online.length > 0 && <Tag tone="primary">{clientCount[record.id].online.length} {t('online')}</Tag>}
              {clientCount[record.id].depleted.length > 0 && <Tag tone="danger">{clientCount[record.id].depleted.length} {t('depleted')}</Tag>}
              {clientCount[record.id].expiring.length > 0 && <Tag tone="warning">{clientCount[record.id].expiring.length} {t('depletingSoon')}</Tag>}
            </div>
          )}
          <div className="stat-row">
            <span className="stat-label">{t('pages.inbounds.expireDate')}</span>
            {record.expiryTime > 0 ? (
              <Tag tone={tn(ColorUtils.usageColor(Date.now(), expireDiff, record._expiryTime))}>
                {IntlUtil.formatRelativeTime(record.expiryTime)}
              </Tag>
            ) : (
              <Tag tone="primary"><InfinityIcon /></Tag>
            )}
          </div>
        </div>
      )}
    </Dialog>
  );
}
