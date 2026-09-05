import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  DataTable,
  DropdownMenu,
  Switch,
  Tooltip,
  TooltipProvider,
} from '@/components/ds';
import {
  MoreOutlined,
  ImportOutlined,
  InfoCircleOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';

import { buildRowActionsMenu } from './RowActions';
import { useInboundColumns } from './useInboundColumns';
import InboundStatsModal from './InboundStatsModal';
import type { DBInboundRecord, InboundListProps } from './types';
export default function InboundList({
  dbInbounds,
  clientCount,
  lastOnlineMap: _lastOnlineMap,
  expireDiff,
  trafficDiff,
  pageSize,
  isMobile,
  subEnable,
  nodesById,
  hasActiveNode,
  onGeneralAction: _onGeneralAction,
  roleLabel,
  onRowAction,
  onBulkDelete: _onBulkDelete,
}: InboundListProps) {
  const { t } = useTranslation();
  const [statsRecord, setStatsRecord] = useState<DBInboundRecord | null>(null);
  const [selectedRowKeys, setSelectedRowKeys] = useState<number[]>([]);
  const [currentPage, setCurrentPage] = useState(1);

  const onSwitchEnable = useCallback(async (dbInbound: DBInboundRecord, next: boolean) => {
    const previous = dbInbound.enable;
    dbInbound.enable = next;
    try {
      const formData = new FormData();
      formData.append('enable', String(next));
      const msg = await HttpUtil.post(`/panel/api/inbounds/setEnable/${dbInbound.id}`, formData);
      if (!msg?.success) dbInbound.enable = previous;
    } catch {
      dbInbound.enable = previous;
    }
  }, []);

  const toggleSelect = useCallback((id: number, checked: boolean) => {
    setSelectedRowKeys((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id); else next.delete(id);
      return Array.from(next);
    });
  }, []);

  const selectAll = useCallback((checked: boolean) => {
    setSelectedRowKeys(checked ? dbInbounds.map((i) => i.id) : []);
  }, [dbInbounds]);

  const allSelected = dbInbounds.length > 0 && selectedRowKeys.length === dbInbounds.length;
  const someSelected = selectedRowKeys.length > 0 && selectedRowKeys.length < dbInbounds.length;


  const columns = useInboundColumns({
    roleLabel,
    hasActiveNode,
    nodesById,
    clientCount,
    subEnable,
    expireDiff,
    trafficDiff,
    onRowAction,
    onSwitchEnable,
  });

  const effectivePageSize = pageSize > 0 ? pageSize : 0;

  const pagination = effectivePageSize > 0 && dbInbounds.length > effectivePageSize
    ? {
      page: currentPage,
      pageSize: effectivePageSize,
      total: dbInbounds.length,
      onChange: (p: number) => setCurrentPage(p),
    }
    : undefined;

  return (
    <TooltipProvider>
      <Card flush className="inbound-table">
        {isMobile ? (
          <div className="inbound-cards" style={{ padding: '0 12px 12px' }}>
            {dbInbounds.length === 0 ? (
              <div className="card-empty">
                <ImportOutlined style={{ fontSize: 28, opacity: 0.5 }} />
                <div>{t('noData')}</div>
              </div>
            ) : (
              <>
                <div className="card-bulk-bar">
                  <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <input
                      type="checkbox"
                      className="ds-check"
                      checked={allSelected}
                      ref={(el) => { if (el) el.indeterminate = someSelected; }}
                      onChange={(e) => selectAll(e.target.checked)}
                    />
                    {t('pages.inbounds.selectAll')}
                  </label>
                  {selectedRowKeys.length > 0 && <span className="bulk-count">{selectedRowKeys.length}</span>}
                </div>
                {dbInbounds.map((record) => (
                  <div key={record.id} className={`inbound-card${selectedRowKeys.includes(record.id) ? ' is-selected' : ''}`}>
                    <div className="card-head">
                      <input
                        type="checkbox"
                        className="ds-check"
                        checked={selectedRowKeys.includes(record.id)}
                        onChange={(e) => toggleSelect(record.id, e.target.checked)}
                      />
                      <span className="card-id">#{record.id}</span>
                      <span className="tag-name">{record.remark}</span>
                      <div className="card-actions" onClick={(e) => e.stopPropagation()}>
                        <Tooltip title={t('pages.inbounds.inboundInfo')}>
                          <InfoCircleOutlined className="row-action-trigger" onClick={() => setStatsRecord(record)} />
                        </Tooltip>
                        <Switch checked={record.enable} onChange={(next) => onSwitchEnable(record, next)} />
                        <DropdownMenu
                          align="end"
                          items={buildRowActionsMenu({
                            record,
                            subEnable,
                            t,
                            isMobile: true,
                            hasClients: (clientCount[record.id]?.clients || 0) > 0,
                            onClick: (key) => onRowAction({ key, dbInbound: record }),
                          })}
                          trigger={<MoreOutlined className="row-action-trigger" />}
                        />
                      </div>
                    </div>
                  </div>
                ))}
              </>
            )}
          </div>
        ) : (
          <div style={{ padding: '0 4px 4px' }}>
            <DataTable<DBInboundRecord>
              data={dbInbounds}
              columns={columns}
              getRowId={(r) => String(r.id)}
              paginateRows
              pagination={pagination}
              empty={
                <>
                  <ImportOutlined style={{ fontSize: 32, marginBottom: 8 }} />
                  <div>{t('noData')}</div>
                </>
              }
            />
          </div>
        )}
      </Card>

      <InboundStatsModal
        open={isMobile && !!statsRecord}
        record={statsRecord}
        hasActiveNode={hasActiveNode}
        nodesById={nodesById}
        clientCount={clientCount}
        trafficDiff={trafficDiff}
        expireDiff={expireDiff}
        onClose={() => setStatsRecord(null)}
      />
    </TooltipProvider>
  );
}
