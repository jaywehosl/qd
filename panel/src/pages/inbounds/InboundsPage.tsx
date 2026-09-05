import { lazy, useCallback, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Col,
  Row,
} from '@/components/ui';
import { Card, Stat, Button, Spin, Dialog, Result, Tag, toast } from '@/components/ds';

import {
  SwapOutlined,
  PieChartOutlined,
  BarsOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter, RandomUtil } from '@/utils';
import { createDefaultInboundSettings } from '@/lib/qd/entry-compat';
import { genEntryLinks, preferPublicHost } from '@/lib/qd/entry-link';
import { inboundFromDb } from '@/lib/qd/entry-link';
import { coerceInboundJsonField, type DBInbound } from '@/models/dbinbound';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
const TextModal = lazy(() => import('@/components/feedback/TextModal'));

import { useInbounds } from './useInbounds';
import { useTheme } from '@/hooks/useTheme';
import { InboundList } from './list';
import { LazyMount } from '@/components/utility';
import VerticalTabs from '@/components/ui/VerticalTabs';
const InboundFormModal = lazy(() => import('./EntryFormModal'));
const InboundInfoModal = lazy(() => import('./info/InboundInfoModal'));
const AttachClientsModal = lazy(() => import('./clients/AttachClientsModal'));
const AttachExistingClientsModal = lazy(() => import('./clients/AttachExistingClientsModal'));
const DetachClientsModal = lazy(() => import('./clients/DetachClientsModal'));
const AddClientsToGroupModal = lazy(() => import('./clients/AddClientsToGroupModal'));

type RowAction =
  | 'edit'
  | 'showInfo'
  | 'qrcode'
  | 'export'
  | 'subs'
  | 'clipboard'
  | 'delete'
  | 'resetTraffic'
  | 'attachClients'
  | 'attachExisting'
  | 'detachClients'
  | 'addToGroup'
  | 'clone';

type GeneralAction = 'import' | 'export' | 'subs' | 'resetInbounds';

interface ClientMatchTarget {
  id?: string;
  email?: string;
  password?: string;
}

export default function InboundsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra } = useTheme();
  const { isMobile } = useMediaQuery();

  const pageClass = useMemo(() => ['inbounds-page', isDark && 'is-dark', isUltra && 'is-ultra'].filter(Boolean).join(' '), [isDark, isUltra]);

  const {
    fetched,
    fetchError,
    dbInbounds,
    clientCount,
    onlineClients,
    lastOnlineMap,
    totals,
    expireDiff,
    trafficDiff,
    pageSize,
    subSettings,
    tgBotEnable,
    ipLimitEnable,
    remarkModel,
    refresh,
    hydrateInbound,
    applyTrafficEvent,
    applyClientStatsEvent,
  } = useInbounds();

  interface ConfirmConfig {
    // The reset confirm carries the node as a pill, so the title is markup.
    title: ReactNode;
    content: string;
    okText?: string;
    okDanger?: boolean;
    onOk: () => void | Promise<void>;
    onCancel?: () => void;
  }
  const [confirm, setConfirm] = useState<ConfirmConfig | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);
  const runConfirm = useCallback(() => {
    if (!confirm) return;
    setConfirmBusy(true);
    Promise.resolve(confirm.onOk()).finally(() => {
      setConfirmBusy(false);
      setConfirm(null);
    });
  }, [confirm]);

  const { nodes: nodesList } = useNodesQuery();
  const nodesById = useMemo(() => {
    const map = new Map<number, ReturnType<typeof useNodesQuery>['nodes'][number]>();
    for (const n of nodesList || []) map.set(n.id, n);
    return map;
  }, [nodesList]);

  const byRole = useMemo(() => {
    const ingress: typeof dbInbounds = [];
    const egress: typeof dbInbounds = [];

    for (const ib of dbInbounds || []) {
      const node = nodesById.get((ib as unknown as { nodeId?: number }).nodeId ?? 0);
      const role = (node as unknown as { role?: string } | undefined)?.role;
      if (role === 'egress') egress.push(ib); else ingress.push(ib);
    }

    return [
      { role: 'ingress', label: t('pages.inbounds.roleIngress', { defaultValue: 'Ingress' }), items: ingress },
      { role: 'egress', label: t('pages.inbounds.roleEgress', { defaultValue: 'Egress' }), items: egress },
    ].filter((g) => g.items.length > 0);
  }, [dbInbounds, nodesById, t]);

  const [role, setRole] = useState('ingress');
  // A role can empty out while it is the one on screen, so fall back rather
  // than render a switch pointing at nothing.
  const shownRole = byRole.find((g) => g.role === role) ?? byRole[0];

  const hasActiveNode = useMemo(
    () => (nodesList || []).some((n) => n.enable && n.status === 'online'),
    [nodesList],
  );
  const hasNodeAttachedInbound = useMemo(
    () => (dbInbounds || []).some((ib) => ib?.nodeId != null),
    [dbInbounds],
  );
  const showNodeInfo = hasNodeAttachedInbound || hasActiveNode;

  useWebSocket({
    traffic: applyTrafficEvent,
    client_stats: applyClientStatsEvent,
  });

  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [formDbInbound, setFormDbInbound] = useState<DBInbound | null>(null);

  const [infoOpen, setInfoOpen] = useState(false);
  const [infoDbInbound, setInfoDbInbound] = useState<DBInbound | null>(null);
  const [infoClientIndex, setInfoClientIndex] = useState(0);


  const [attachOpen, setAttachOpen] = useState(false);
  const [attachSource, setAttachSource] = useState<DBInbound | null>(null);
  const [attachExistingOpen, setAttachExistingOpen] = useState(false);
  const [attachExistingTarget, setAttachExistingTarget] = useState<DBInbound | null>(null);
  const [detachOpen, setDetachOpen] = useState(false);
  const [detachSource, setDetachSource] = useState<DBInbound | null>(null);

  const [groupOpen, setGroupOpen] = useState(false);
  const [groupSource, setGroupSource] = useState<DBInbound | null>(null);

  const [textOpen, setTextOpen] = useState(false);
  const [textTitle, setTextTitle] = useState('');
  const [textContent, setTextContent] = useState('');
  const [textFileName, setTextFileName] = useState('');

  const hostOverrideFor = useCallback((dbInbound: DBInbound | null) => {
    if (!dbInbound || dbInbound.nodeId == null) return '';
    return nodesById.get(dbInbound.nodeId)?.address || '';
  }, [nodesById]);

  const infoNodeAddress = useMemo(() => hostOverrideFor(infoDbInbound), [infoDbInbound, hostOverrideFor]);

  const openText = useCallback((opts: { title: string; content: string; fileName?: string }) => {
    setTextTitle(opts.title);
    setTextContent(opts.content);
    setTextFileName(opts.fileName || '');
    setTextOpen(true);
  }, []);

  const projectChildThroughMaster = useCallback((child: DBInbound, master: DBInbound): DBInbound => {
    const projected = JSON.parse(JSON.stringify(child)) as DBInbound;
    projected.listen = master.listen;
    projected.port = master.port;
    const masterStream = coerceInboundJsonField(master.streamSettings) as Record<string, unknown>;
    const childStream = { ...(coerceInboundJsonField(child.streamSettings) as Record<string, unknown>) };
    childStream.security = masterStream.security;
    childStream.tlsSettings = masterStream.tlsSettings;
    childStream.realitySettings = masterStream.realitySettings;
    childStream.externalProxy = masterStream.externalProxy;
    projected.streamSettings = JSON.stringify(childStream);
    const Ctor = child.constructor as new (data: DBInbound) => DBInbound;
    return new Ctor(projected);
  }, []);

  const checkFallback = useCallback((dbInbound: DBInbound): DBInbound => {
    const parent = dbInbound?.fallbackParent;
    if (parent?.masterId) {
      const master = dbInbounds.find((ib) => ib.id === parent.masterId);
      if (master) return projectChildThroughMaster(dbInbound, master);
    }
    if (!dbInbound?.listen?.startsWith?.('@')) return dbInbound;
    for (const candidate of dbInbounds) {
      if (candidate.id === dbInbound.id) continue;
      if (!['trojan', 'vless'].includes(candidate.protocol)) continue;
      const candStream = coerceInboundJsonField(candidate.streamSettings) as { network?: string };
      if (candStream.network !== 'tcp') continue;
      const candSettings = coerceInboundJsonField(candidate.settings) as { fallbacks?: { dest?: string }[] };
      const fallbacks = candSettings.fallbacks || [];
      if (!fallbacks.find((f) => f.dest === dbInbound.listen)) continue;
      return projectChildThroughMaster(dbInbound, candidate);
    }
    return dbInbound;
  }, [dbInbounds, projectChildThroughMaster]);

  const findClientIndex = useCallback((dbInbound: DBInbound, client: ClientMatchTarget | null) => {
    if (!client) return 0;
    const settings = coerceInboundJsonField(dbInbound.settings) as { clients?: ClientMatchTarget[] };
    const clients = settings.clients || [];
    const idx = clients.findIndex((c) => {
      if (!c) return false;
      switch (dbInbound.protocol) {
        case 'trojan':
        case 'shadowsocks':
          return c.password === client.password && c.email === client.email;
        default:
          return c.id === client.id && c.email === client.email;
      }
    });
    return idx >= 0 ? idx : 0;
  }, []);

  const exportInboundLinks = useCallback((dbInbound: DBInbound) => {
    const projected = checkFallback(dbInbound);
    openText({
      title: t('pages.inbounds.exportLinksTitle'),
      content: genEntryLinks({
        inbound: inboundFromDb(projected),
        remark: projected.remark,
        remarkModel,
        hostOverride: hostOverrideFor(dbInbound),
        fallbackHostname: preferPublicHost(window.location.hostname, subSettings.publicHost),
      }),
      fileName: projected.remark || 'inbound',
    });
  }, [checkFallback, remarkModel, hostOverrideFor, subSettings.publicHost, openText, t]);

  const exportInboundClipboard = useCallback((dbInbound: DBInbound) => {
    openText({ title: t('pages.inbounds.inboundJsonTitle'), content: JSON.stringify(dbInbound, null, 2) });
  }, [openText, t]);

  const exportInboundSubs = useCallback((dbInbound: DBInbound) => {
    const settings = coerceInboundJsonField(dbInbound.settings) as { clients?: { subId?: string }[] };
    const clients = settings.clients || [];
    const subLinks: string[] = [];
    for (const c of clients) {
      if (c.subId && subSettings.subURI) {
        subLinks.push(subSettings.subURI + c.subId);
      }
    }
    openText({
      title: t('pages.inbounds.exportSubsTitle'),
      content: [...new Set(subLinks)].join('\n'),
      fileName: `${dbInbound.remark || 'inbound'}-Subs`,
    });
  }, [subSettings, openText, t]);

  const exportAllLinks = useCallback(async () => {
    const hydrated = await Promise.all(
      dbInbounds.map((ib) => hydrateInbound(ib.id).then((r) => r ?? ib)),
    );
    const out: string[] = [];
    for (const ib of hydrated) {
      const projected = checkFallback(ib);
      out.push(genEntryLinks({
        inbound: inboundFromDb(projected),
        remark: projected.remark,
        remarkModel,
        hostOverride: hostOverrideFor(ib),
        fallbackHostname: preferPublicHost(window.location.hostname, subSettings.publicHost),
      }));
    }
    openText({ title: t('pages.inbounds.exportAllLinksTitle'), content: out.join('\r\n'), fileName: t('pages.inbounds.exportAllLinksFileName') });
  }, [dbInbounds, hydrateInbound, checkFallback, remarkModel, hostOverrideFor, subSettings.publicHost, openText, t]);

  const exportAllSubs = useCallback(async () => {
    const hydrated = await Promise.all(
      dbInbounds.map((ib) => hydrateInbound(ib.id).then((r) => r ?? ib)),
    );
    const out: string[] = [];
    for (const ib of hydrated) {
      const settings = coerceInboundJsonField(ib.settings) as { clients?: { subId?: string }[] };
      const clients = settings.clients || [];
      for (const c of clients) {
        if (c.subId && subSettings.subURI) {
          out.push(subSettings.subURI + c.subId);
        }
      }
    }
    openText({ title: t('pages.inbounds.exportAllSubsTitle'), content: [...new Set(out)].join('\r\n'), fileName: t('pages.inbounds.exportAllSubsFileName') });
  }, [dbInbounds, hydrateInbound, subSettings, openText, t]);

  const openEdit = useCallback((dbInbound: DBInbound) => {
    setFormMode('edit');
    setFormDbInbound(dbInbound);
    setFormOpen(true);
  }, []);

  const confirmDelete = useCallback((dbInbound: DBInbound) => {
    setConfirm({
      title: t('pages.inbounds.deleteConfirmTitle', { remark: dbInbound.remark }),
      content: t('pages.inbounds.deleteConfirmContent'),
      okText: t('delete'),
      okDanger: true,
      onOk: async () => {
        const msg = await HttpUtil.post(`/panel/api/inbounds/del/${dbInbound.id}`);
        if (msg?.success) await refresh();
      },
    });
  }, [refresh, t]);

  const confirmBulkDelete = useCallback((ids: number[]) => new Promise<boolean>((resolve) => {
    if (ids.length === 0) {
      resolve(false);
      return;
    }
    setConfirm({
      title: t('pages.inbounds.bulkDeleteConfirmTitle', { count: ids.length }),
      content: t('pages.inbounds.bulkDeleteConfirmContent'),
      okText: t('delete'),
      okDanger: true,
      onOk: async () => {
        const msg = await HttpUtil.post('/panel/api/inbounds/bulkDel', { ids }, { headers: { 'Content-Type': 'application/json' } });
        const obj = (msg?.obj ?? {}) as { deleted?: number; skipped?: { id: number; reason: string }[] };
        const ok = obj.deleted ?? 0;
        const skipped = obj.skipped ?? [];
        if (msg?.success && skipped.length === 0) {
          toast.success(t('pages.inbounds.toasts.bulkDeleted', { count: ok }));
        } else {
          const firstError = skipped[0]?.reason ?? msg?.msg ?? '';
          const base = t('pages.inbounds.toasts.bulkDeletedMixed', { ok, failed: skipped.length });
          toast.warning(firstError ? `${base} — ${firstError}` : base);
        }
        await refresh();
        resolve(true);
      },
      onCancel: () => resolve(false),
    });
  }), [refresh, t]);

  const confirmResetTraffic = useCallback((dbInbound: DBInbound) => {
    // Name the node and the port rather than the stored remark: that label is a
    // leftover nobody edits any more, so it can still name a renamed node.
    const owner = nodesById.get((dbInbound as unknown as { nodeId?: number }).nodeId ?? 0);
    const where = owner?.name || owner?.address || '—';
    setConfirm({
      title: (
        <span className="ef-title">
          {t('reset', { defaultValue: 'Reset' })}
          <Tag tone={owner?.status === 'online' ? 'success' : 'neutral'} className="ef-title__node">{where}</Tag>
          {t('pages.inbounds.trafficWord', { defaultValue: 'traffic?' })}
        </span>
      ),
      content: t('pages.inbounds.resetConfirmContent'),
      okText: t('reset'),
      onOk: async () => {
        const msg = await HttpUtil.post(`/panel/api/inbounds/${dbInbound.id}/resetTraffic`);
        if (msg?.success) await refresh();
      },
    });
  }, [refresh, t, nodesById]);

  const confirmClone = useCallback((dbInbound: DBInbound) => {
    setConfirm({
      title: t('pages.inbounds.cloneConfirmTitle', { remark: dbInbound.remark }),
      content: t('pages.inbounds.cloneConfirmContent'),
      okText: t('pages.inbounds.clone'),
      onOk: async () => {
        let clonedSettings: string;
        try {
          const raw = coerceInboundJsonField(dbInbound.settings);
          raw.clients = [];
          clonedSettings = JSON.stringify(raw);
        } catch {
          const fallback = createDefaultInboundSettings(dbInbound.protocol);
          clonedSettings = fallback ? JSON.stringify(fallback, null, 2) : '{}';
        }
        const streamSettingsString = typeof dbInbound.streamSettings === 'string'
          ? dbInbound.streamSettings
          : JSON.stringify(dbInbound.streamSettings ?? {});
        const sniffingString = typeof dbInbound.sniffing === 'string'
          ? dbInbound.sniffing
          : JSON.stringify(dbInbound.sniffing ?? {});
        const data = {
          up: 0,
          down: 0,
          total: 0,
          remark: `${dbInbound.remark} (clone)`,
          enable: false,
          expiryTime: 0,
          listen: '',
          port: RandomUtil.randomInteger(10000, 60000),
          protocol: dbInbound.protocol,
          settings: clonedSettings,
          streamSettings: streamSettingsString,
          sniffing: sniffingString,
        };
        const msg = await HttpUtil.post('/panel/api/inbounds/add', data);
        if (msg?.success) await refresh();
      },
    });
  }, [refresh, t]);

  const onGeneralAction = useCallback((key: GeneralAction) => {
    switch (key) {
      case 'export': exportAllLinks(); break;
      case 'subs': exportAllSubs(); break;
      case 'resetInbounds':
        setConfirm({
          title: t('pages.inbounds.resetAllTrafficTitle'),
          content: t('pages.inbounds.resetAllTrafficContent'),
          okText: t('reset'),
          onOk: async () => {
            const msg = await HttpUtil.post('/panel/api/inbounds/resetAllTraffics');
            if (msg?.success) await refresh();
          },
        });
        break;
      default:
        toast.info(`General action "${key}" — coming in a later 5f subphase`);
    }
  }, [exportAllLinks, exportAllSubs, refresh, t]);


  const onRowAction = useCallback(async ({ key, dbInbound }: { key: RowAction; dbInbound: DBInbound }) => {
    // Actions that touch per-client secrets (uuid, password, flow, ...) need
    // the full payload that the slim list view does not ship. Hydrate first
    // and then operate on the rehydrated record.
    const hydratingKeys: RowAction[] = ['edit', 'showInfo', 'qrcode', 'export', 'subs', 'clipboard', 'clone', 'attachClients', 'addToGroup'];
    let target = dbInbound;
    if (hydratingKeys.includes(key)) {
      const hydrated = await hydrateInbound(dbInbound.id);
      if (hydrated) target = hydrated;
    }
    switch (key) {
      case 'edit':
        openEdit(target);
        break;
      case 'showInfo':
        setInfoDbInbound(checkFallback(target));
        setInfoClientIndex(findClientIndex(target, null));
        setInfoOpen(true);
        break;
      case 'export':
        exportInboundLinks(target);
        break;
      case 'subs':
        exportInboundSubs(target);
        break;
      case 'clipboard':
        exportInboundClipboard(target);
        break;
      case 'delete':
        confirmDelete(target);
        break;
      case 'resetTraffic':
        confirmResetTraffic(target);
        break;
      case 'attachClients':
        setAttachSource(target);
        setAttachOpen(true);
        break;
      case 'attachExisting':
        setAttachExistingTarget(target);
        setAttachExistingOpen(true);
        break;
      case 'detachClients':
        setDetachSource(target);
        setDetachOpen(true);
        break;
      case 'addToGroup':
        setGroupSource(target);
        setGroupOpen(true);
        break;
      case 'clone':
        confirmClone(target);
        break;
      default:
        toast.info(`Action "${key}" — coming in a later 5f subphase`);
    }
  }, [hydrateInbound, openEdit, checkFallback, findClientIndex, exportInboundLinks, exportInboundSubs, exportInboundClipboard, confirmDelete, confirmResetTraffic, confirmClone]);

  return (
    <div className={`section-content-wrapper inbounds-section-wrapper ${pageClass}`}>
      <Spin spinning={!fetched} description={t('loading')} size="large">
        {!fetched ? (
          <div className="loading-spacer" />
        ) : fetchError ? (
          <Result
            status="error"
            title={t('somethingWentWrong')}
            subTitle={fetchError}
            extra={<Button variant="primary" onClick={refresh}>{t('refresh')}</Button>}
          />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, 12]}>
                  <Col span={24}>
                    <Card className="summary-card">
                      <div className="ds-stats-grid">
                        <Stat
                          title={t('pages.inbounds.totalDownUp')}
                          value={`${SizeFormatter.sizeFormat(totals.up)} / ${SizeFormatter.sizeFormat(totals.down)}`}
                          prefix={<SwapOutlined />}
                        />
                        <Stat
                          title={t('pages.inbounds.totalUsage')}
                          value={SizeFormatter.sizeFormat(totals.up + totals.down)}
                          prefix={<PieChartOutlined />}
                        />
                        <Stat
                          title={t('pages.inbounds.inboundCount')}
                          value={String(dbInbounds.length)}
                          prefix={<BarsOutlined />}
                        />
                      </div>
                    </Card>
                  </Col>

                  {byRole.length > 1 && (
                    <Col span={24}>
                      <div className="inb-roles">
                        <VerticalTabs
                          items={byRole.map(({ role, label }) => ({ key: role, label }))}
                          activeKey={shownRole?.role ?? ''}
                          onChange={setRole}
                        />
                      </div>
                    </Col>
                  )}

                  {(shownRole ? [shownRole] : []).map(({ role, label, items }) => (
                    <Col span={24} key={role} className="qd-page-swap">
                      <InboundList
                        roleLabel={label}
                        dbInbounds={items}
                        clientCount={clientCount}
                        onlineClients={onlineClients}
                        lastOnlineMap={lastOnlineMap}
                        expireDiff={expireDiff}
                        trafficDiff={trafficDiff}
                        pageSize={pageSize}
                        isMobile={isMobile}
                        subEnable={subSettings.enable}
                        nodesById={nodesById}
                        hasActiveNode={showNodeInfo}
                        onGeneralAction={onGeneralAction}
                        onRowAction={({ key, dbInbound }) => onRowAction({ key, dbInbound: dbInbound as unknown as DBInbound })}
                        onBulkDelete={confirmBulkDelete}
                      />
                    </Col>
                  ))}
                </Row>
              )}
            </Spin>
        <LazyMount when={formOpen}>
          <InboundFormModal
            open={formOpen}
            onClose={() => setFormOpen(false)}
            onSaved={refresh}
            mode={formMode}
            dbInbound={formDbInbound}
            dbInbounds={dbInbounds}
            availableNodes={nodesList}
          />
        </LazyMount>
        <LazyMount when={infoOpen}>
          <InboundInfoModal
            open={infoOpen}
            onClose={() => setInfoOpen(false)}
            dbInbound={infoDbInbound}
            clientIndex={infoClientIndex}
            remarkModel={remarkModel}
            expireDiff={expireDiff}
            trafficDiff={trafficDiff}
            ipLimitEnable={ipLimitEnable}
            tgBotEnable={tgBotEnable}
            subSettings={subSettings}
            lastOnlineMap={lastOnlineMap}
            nodeAddress={infoNodeAddress}
          />
        </LazyMount>
        <LazyMount when={attachOpen}>
          <AttachClientsModal
            open={attachOpen}
            onClose={() => setAttachOpen(false)}
            onAttached={refresh}
            source={attachSource}
            dbInbounds={dbInbounds}
          />
        </LazyMount>
        <LazyMount when={attachExistingOpen}>
          <AttachExistingClientsModal
            open={attachExistingOpen}
            onClose={() => setAttachExistingOpen(false)}
            onAttached={refresh}
            target={attachExistingTarget}
          />
        </LazyMount>
        <LazyMount when={detachOpen}>
          <DetachClientsModal
            open={detachOpen}
            onClose={() => setDetachOpen(false)}
            onDetached={refresh}
            source={detachSource}
          />
        </LazyMount>
        <LazyMount when={groupOpen}>
          <AddClientsToGroupModal
            open={groupOpen}
            onClose={() => setGroupOpen(false)}
            onAdded={refresh}
            source={groupSource}
          />
        </LazyMount>

        <LazyMount when={textOpen}>
          <TextModal
            open={textOpen}
            onClose={() => setTextOpen(false)}
            title={textTitle}
            content={textContent}
            fileName={textFileName}
          />
        </LazyMount>

        <Dialog
          open={confirm !== null}
          onOpenChange={(o) => {
            if (!o) {
              if (confirm?.onCancel) confirm.onCancel();
              setConfirm(null);
            }
          }}
          title={confirm?.title ?? ''}
          okText={confirm?.okText ?? t('confirm')}
          okDanger={confirm?.okDanger}
          confirmLoading={confirmBusy}
          onOk={runConfirm}
        >
          <p style={{ margin: 0 }}>{confirm?.content}</p>
        </Dialog>
      </div>
  );
}
