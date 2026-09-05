import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CloseCircleOutlined,
  CloudServerOutlined,
  ThunderboltOutlined,
  WifiOutlined,
} from '@ant-design/icons';

import { Button, Card, Dialog, Stat } from '@/components/ds';
import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { useNodeMutations } from '@/api/queries/useNodeMutations';
import NodeList from './NodeList';
import NodeFormModal from './NodeFormModal';
import { getMessage } from '@/utils/messageBus';
import { HttpUtil } from '@/utils';

interface ConfirmState {
  title: string;
  content: string;
  okText: string;
  onOk: () => void | Promise<void>;
}

export default function NodesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra } = useTheme();
  const { isMobile } = useMediaQuery();
  const message = getMessage();

  const { nodes, loading, fetched, fetchError, refetch, totals } = useNodesQuery();
  const { create, update, remove, setEnable, probe } = useNodeMutations();

  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [formNode, setFormNode] = useState<NodeRecord | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

  function runConfirm() {
    if (!confirm) return;
    setConfirmBusy(true);
    Promise.resolve(confirm.onOk()).finally(() => { setConfirmBusy(false); setConfirm(null); });
  }

  const onAdd = useCallback(() => { setFormMode('add'); setFormNode(null); setFormOpen(true); }, []);
  const onEdit = useCallback((node: NodeRecord) => { setFormMode('edit'); setFormNode({ ...node }); setFormOpen(true); }, []);

  const onSave = useCallback(async (payload: Partial<NodeRecord>) => {
    if (formMode === 'edit' && formNode?.id) return update(formNode.id, payload);
    return create(payload);
  }, [formMode, formNode, update, create]);

  const onDelete = useCallback((node: NodeRecord) => {
    setConfirm({
      title: t('pages.nodes.deleteConfirmTitle', { name: node.name }),
      content: t('pages.nodes.deleteConfirmContent'),
      okText: t('delete'),
      onOk: async () => {
        const msg = await remove(node.id);
        if (msg?.success) message.success(t('pages.nodes.toasts.deleted'));
      },
    });
  }, [t, remove, message]);

  const onProbe = useCallback(async (node: NodeRecord) => {
    const msg = await probe(node.id);
    if (msg?.success && msg.obj) {
      if (msg.obj.status === 'online') message.success(t('pages.nodes.connectionOk', { ms: msg.obj.latencyMs }));
      else message.error(msg.obj.error || t('pages.nodes.toasts.probeFailed'));
    }
  }, [probe, t, message]);

  const onToggleEnable = useCallback(async (node: NodeRecord, next: boolean) => {
    await setEnable(node.id, next);
  }, [setEnable]);

  const onSync = useCallback((node: NodeRecord) => {
    setConfirm({
      title: t('pages.nodes.syncConfirmTitle', { defaultValue: `Copy the network database onto ${node.name}?` }),
      content: t('pages.nodes.syncConfirmContent', {
        defaultValue: 'The freshest node in the network hands over its database and it replaces everything this node holds.',
      }),
      okText: t('pages.nodes.syncNode', { defaultValue: 'Synchronise' }),
      onOk: async () => {
        const msg = await HttpUtil.post('/panel/api/server/sync', { nodeId: node.id },
          { headers: { 'Content-Type': 'application/json' }, silent: true });
        if (msg?.success) {
          message.success(t('pages.nodes.syncDone', { defaultValue: 'Database copied' }));
          void refetch();
          return;
        }
        message.error(msg?.msg || t('somethingWentWrong'));
      },
    });
  }, [t, message, refetch]);

  const pageClass = useMemo(
    () => ['nodes-page', isDark && 'is-dark', isUltra && 'is-ultra'].filter(Boolean).join(' '),
    [isDark, isUltra],
  );

  return (
    <div className={`section-content-wrapper nodes-section-wrapper ${pageClass}`}>
      {!fetched ? (
        <div className="ds-table__empty">{t('loading')}</div>
      ) : fetchError ? (
        <Card>
          <div style={{ textAlign: 'center', padding: 24 }}>
            <h3>{t('somethingWentWrong')}</h3>
            <p className="ds-muted">{fetchError}</p>
            <Button variant="primary" loading={loading} onClick={() => refetch()}>{t('refresh')}</Button>
          </div>
        </Card>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: isMobile ? 8 : 12 }}>
          <Card>
            <div className="ds-stats-grid">
              <Stat title={t('pages.nodes.totalNodes')} value={totals.total} prefix={<CloudServerOutlined />} />
              <Stat title={t('pages.nodes.onlineNodes')} value={totals.online} prefix={<WifiOutlined />} />
              <Stat title={t('pages.nodes.offlineNodes')} value={totals.offline} prefix={<CloseCircleOutlined style={{ color: 'var(--color-error)' }} />} />
              <Stat title={t('pages.nodes.avgLatency')} value={totals.avgLatency > 0 ? `${totals.avgLatency} ms` : '-'} prefix={<ThunderboltOutlined />} />
            </div>
          </Card>

          <NodeList
            nodes={nodes}
            loading={loading}
            isMobile={isMobile}
            onAdd={onAdd}
            onEdit={onEdit}
            onDelete={onDelete}
            onProbe={onProbe}
            onSync={onSync}
            onToggleEnable={onToggleEnable}
          />
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
        <p style={{ margin: 0 }}>{confirm?.content}</p>
      </Dialog>

      <NodeFormModal
        open={formOpen}
        mode={formMode}
        node={formNode}
        taken={nodes.map((n) => n.name ?? '').filter((name) => name !== '')}
        usedIds={nodes.map((n) => n.id)}
        save={onSave}
        onOpenChange={setFormOpen}
      />
    </div>
  );
}
