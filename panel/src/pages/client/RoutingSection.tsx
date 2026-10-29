import { lazy, useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AppstoreOutlined, BlockOutlined, DeleteOutlined, DownloadOutlined, DownOutlined, EllipsisOutlined, GlobalOutlined,
  PlusOutlined, UploadOutlined,
} from '@ant-design/icons';

import { Alert, Button, Card, Dialog, DropdownMenu, Field, Input, Select, Tag, toast } from '@/components/ds';
import { Spin, VerticalTabs } from '@/components/ui';
import { LazyMount } from '@/components/utility';
import { iconOf } from '@/pages/settings/routeServices';
import { ROUTING_ROLES, type RoutingRole } from '@/schemas/client-routing';
import { useClientRouting } from '@/hooks/useClientRouting';
import { HttpUtil } from '@/utils';
import { phone, worded } from '@/lib/phone';
const ProcessPickerDialog = lazy(() => import('./ProcessPickerDialog'));

type Page = 'process' | 'domain' | 'bundle';

function hostOf(raw: string): string {
  const bare = raw.trim().replace(/^[a-z][a-z0-9+.-]*:\/\//i, '').replace(/^\*\./, '');
  try {
    const host = new URL(`http://${bare}`).hostname.replace(/^\.+|\.+$/g, '');
    return host.includes('.') && !/^[\d.]+$/.test(host) && !host.includes(':') ? host : '';
  } catch {
    return '';
  }
}

interface RoutingSectionProps {
  connected: boolean;
  onReconnect: () => Promise<unknown>;
}

export default function RoutingSection({ connected, onReconnect }: RoutingSectionProps) {
  const { t } = useTranslation();
  const {
    state, loading, setRole, setDefaultRole, remove, add, reset,
    setDomainRole, removeDomain, addDomain, resetDomains, setBundleRole, resetBundles, refresh,
  } = useClientRouting();

  const [page, setPage] = useState<Page>('process');
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState('');
  const [picking, setPicking] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const [legendOpen, setLegendOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);

  const doReconnect = useCallback(async () => {
    setReconnecting(true);
    try {
      await onReconnect();
      refresh();
    } finally {
      setReconnecting(false);
    }
  }, [onReconnect, refresh]);

  const global = state?.defaultRole ?? 'tunnel';
  const roleOptions = useMemo(
    () => ROUTING_ROLES.map((r) => ({
      value: r,
      label: t(`client.routing.role.${r}`, { role: t(`client.routing.role.${global}`) }),
      face: r === 'default' ? t(`client.routing.role.${global}`) : undefined,
    })),
    [t, global],
  );

  const baseOptions = useMemo(
    () => roleOptions.filter((o) => o.value === 'direct' || o.value === 'tunnel'),
    [roleOptions],
  );

  const existing = useMemo(
    () => new Set((state?.rules ?? []).map((r) => (r.path || r.process).toLowerCase())),
    [state],
  );

  const onPick = useCallback((pick: { process: string; path?: string }) => {
    if (!state) return;
    void add({ ...pick, role: 'default' });
  }, [state, add]);

  const pages = useMemo(() => [
    { key: 'process', label: t('client.routing.byProcess', worded()), icon: <AppstoreOutlined /> },
    { key: 'domain', label: t('client.routing.byDomain'), icon: <GlobalOutlined /> },
    { key: 'bundle', label: t('client.routing.byBundle'), icon: <BlockOutlined /> },
  ], [t]);

  const host = hostOf(name);
  const hostTaken = (state?.domains ?? []).some((d) => d.domain === host);

  const doName = useCallback(async () => {
    if (!state || !host || hostTaken) return;
    setBusy(true);
    try {
      const next = await addDomain(host, 'default');
      if (next) {
        setNaming(false);
        setName('');
      }
    } finally {
      setBusy(false);
    }
  }, [state, host, hostTaken, addDomain]);

  const fileRef = useRef<HTMLInputElement>(null);

  const doExport = useCallback(async () => {
    const msg = await HttpUtil.get<{ code: string; name: string }>('/client/api/routing/export');
    if (!msg?.success || !msg.obj) return;
    const host = phone();
    if (host?.save) {
      const said = host.save(msg.obj.name, msg.obj.code);
      if (said.startsWith('Downloads/')) toast.success(said);
      else toast.error(said);
      return;
    }
    const url = URL.createObjectURL(new Blob([msg.obj.code], { type: 'application/octet-stream' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = msg.obj.name;
    link.click();
    URL.revokeObjectURL(url);
    toast.success(t('client.routing.rulesSaved', { name: msg.obj.name }));
  }, [t]);

  const doImport = useCallback(async (file: File) => {
    const msg = await HttpUtil.post<{ rules: number }>(
      '/client/api/routing/import',
      { code: await file.text() },
      { headers: { 'Content-Type': 'application/json' } },
    );
    if (!msg?.success) return;
    toast.success(t('client.routing.rulesImported', { count: msg.obj?.rules ?? 0 }));
    refresh();
  }, [refresh, t]);

  const doReset = useCallback(async () => {
    setBusy(true);
    try {
      await (page === 'bundle' ? resetBundles() : page === 'domain' ? resetDomains() : reset());
      setConfirmReset(false);
    } finally {
      setBusy(false);
    }
  }, [page, reset, resetDomains, resetBundles]);

  if (loading || !state) {
    return <div className="rt-boot"><Spin spinning size="large" /></div>;
  }

  const { rules, domains, bundles, defaultRole, allowExit, applyMode, pendingRestart, byDomain } = state;
  const view: Page = byDomain ? page : 'process';
  const stack = phone() ? 7 : 9;
  const named = view === 'domain';
  const idle = view === 'bundle' ? !bundles.some((b) => b.role && b.role !== 'default') : (named ? domains : rules).length === 0;
  const plain = (r: string) => r !== 'egress' && r !== 'noEgress';
  const shownRoles = ROUTING_ROLES.filter((r) => allowExit || plain(r));
  const offered = (role: string) => roleOptions.filter((o) => allowExit || plain(o.value) || o.value === role);

  return (
    <div className="rt">
      {applyMode === 'restart' && pendingRestart && (
        <Alert
          tone="warning"
          title={t('client.routing.restartNeeded')}
          description={connected ? (
            <div className="rt-restart">
              <span>{t('client.routing.restartNeededDesc')}</span>
              <Button size="sm" loading={reconnecting} onClick={() => void doReconnect()}>
                {t('client.routing.reconnect')}
              </Button>
            </div>
          ) : t('client.routing.restartNeededIdle')}
        />
      )}

      <Card>
        <div className="rt-default">
          <div className="rt-default__text">
            <b>{t('client.routing.everythingElse')}</b>
            <span>{t('client.routing.everythingElseDesc', worded())}</span>
          </div>
          <Select
            className={`rt-role rt-role--${defaultRole}`}
            value={defaultRole}
            options={baseOptions}
            onChange={(v) => void setDefaultRole(v as 'direct' | 'tunnel')}
          />
        </div>
      </Card>

      {byDomain && (
        <div className="inb-roles">
          <VerticalTabs items={pages} activeKey={page} onChange={(key) => setPage(key as Page)} />
        </div>
      )}

      <Card
        className="rt-main"
        title={t('client.routing.rules')}
        extra={(
          <div className="rt-head">
            {view !== 'bundle' && (
              <Button icon={<PlusOutlined />} onClick={() => (named ? setNaming(true) : setPicking(true))}>
                {t(named ? 'client.routing.addDomain' : 'client.routing.addRule')}
              </Button>
            )}
            <DropdownMenu
              align="end"
              trigger={<Button variant="text" icon={<EllipsisOutlined />} aria-label={t('client.routing.more')} />}
              items={[
                { key: 'save', icon: <DownloadOutlined />, label: t('client.routing.exportRules'), onSelect: () => void doExport() },
                { key: 'load', icon: <UploadOutlined />, label: t('client.routing.importRules'), onSelect: () => fileRef.current?.click() },
                { type: 'divider' },
                {
                  key: 'reset',
                  icon: <DeleteOutlined />,
                  danger: true,
                  disabled: idle,
                  label: t(view === 'bundle' ? 'client.routing.resetBundles' : named ? 'client.routing.resetDomains' : 'client.routing.resetRules', worded()),
                  onSelect: () => setConfirmReset(true),
                },
              ]}
            />
          </div>
        )}
        flush
      >
        <div className="rt-rules qd-page-swap" key={view}>
          {view === 'bundle' ? bundles.map((b) => {
            const role = b.role || 'default';
            return (
              <div key={b.id} className="rt-rule rt-bundle">
                <div className="rt-bundle__id">
                  <span className="rt-rule__name">{t(`client.routing.bundleName.${b.id}`, { defaultValue: b.name })}</span>
                  <span className="rt-stack">
                    {b.services.slice(0, stack).map((s, i) => {
                      const icon = iconOf(s.id);
                      return (
                        <span
                          key={s.id}
                          className={`rt-stack__one${icon?.mono ? ' is-mono' : ''}`}
                          title={s.name}
                          style={{ zIndex: stack - i }}
                        >
                          {icon ? (
                            <>
                              <img className={icon.night ? 'rt-day' : undefined} src={icon.src} alt={s.name} />
                              {icon.night && <img className="rt-night" src={icon.night} alt={s.name} />}
                            </>
                          ) : (
                            <GlobalOutlined aria-label={s.name} />
                          )}
                        </span>
                      );
                    })}
                    {b.services.length > stack && (
                      <span
                        className="rt-stack__one rt-stack__more"
                        title={b.services.slice(stack).map((s) => s.name).join(', ')}
                      >
                        +{b.services.length - stack}
                      </span>
                    )}
                  </span>
                </div>
                {b.matched ? <Tag className="rt-flows">{t('client.routing.flows', { count: b.matched })}</Tag> : null}
                {!allowExit && (role === 'egress' || role === 'noEgress') && <Tag tone="warning">{t('client.routing.noExit')}</Tag>}
                <Select
                  className={`rt-role rt-role--${role}`}
                  value={role}
                  options={offered(role)}
                  onChange={(v) => void setBundleRole(b.id, String(v))}
                />
              </div>
            );
          }) : view === 'domain' ? (domains.length === 0 ? (
            <div className="rt-empty">{t('client.routing.noDomains', worded())}</div>
          ) : domains.map((d) => (
            <div key={d.id} className="rt-rule">
              <GlobalOutlined className="rt-proc__icon rt-domain__icon" aria-hidden="true" />
              <div className="rt-rule__id">
                <span className="rt-rule__name">{d.domain}</span>
              </div>
              {d.matched ? <Tag className="rt-flows">{t('client.routing.flows', { count: d.matched })}</Tag> : null}
              {!allowExit && !plain(d.role) && <Tag tone="warning">{t('client.routing.noExit')}</Tag>}
              <Select
                className={`rt-role rt-role--${d.role}`}
                value={d.role}
                options={offered(d.role)}
                onChange={(v) => void setDomainRole(d.id, v as RoutingRole)}
              />
              <Button
                size="sm"
                danger
                icon={<DeleteOutlined />}
                aria-label={t('client.routing.removeRule')}
                onClick={() => void removeDomain(d.id)}
              />
            </div>
          ))) : rules.length === 0 ? (
            <div className="rt-empty">{t('client.routing.noRules', worded())}</div>
          ) : rules.map((r) => (
            <div key={r.id} className="rt-rule">
              <span className={`rt-dot${r.running ? ' is-up' : ''}`} />
              {r.icon
                ? <img className="rt-proc__icon" src={r.icon} alt="" aria-hidden="true" />
                : <span className="rt-proc__icon rt-proc__icon--blank" aria-hidden="true" />}
              <div className="rt-rule__id">
                <span className="rt-rule__name">{r.title || r.process}</span>
                {(r.path || r.title) && <span className="rt-rule__path">{r.path || r.process}</span>}
              </div>
              {r.matched ? <Tag className="rt-flows">{t('client.routing.flows', { count: r.matched })}</Tag> : null}
              {!allowExit && !plain(r.role) && <Tag tone="warning">{t('client.routing.noExit')}</Tag>}
              <Select
                className={`rt-role rt-role--${r.role}`}
                value={r.role}
                options={offered(r.role)}
                onChange={(v) => void setRole(r.id, v as RoutingRole)}
              />
              <Button
                size="sm"
                danger
                icon={<DeleteOutlined />}
                aria-label={t('client.routing.removeRule')}
                onClick={() => void remove(r.id)}
              />
            </div>
          ))}
        </div>

        <div className="rt-danger">
          <button
            type="button"
            className={`rt-legend__toggle${legendOpen ? ' is-open' : ''}`}
            aria-expanded={legendOpen}
            onClick={() => setLegendOpen((v) => !v)}
          >
            <DownOutlined className="rt-legend__chev" />
            {t('client.routing.legend')}
          </button>
          <div className={`rt-legend__fold${legendOpen ? ' is-open' : ''}`}>
            <div className="rt-legend__inner">
              <div className="rt-legend">
                {shownRoles.map((r) => (
                  <div key={r} className="rt-legend__row">
                    <Tag className={`rt-role rt-role--${r}`}>
                      {t(r === 'default' ? 'client.routing.defaultWord' : `client.routing.role.${r}`)}
                    </Tag>
                    <span>{t(`client.routing.roleDesc.${r}`, worded())}</span>
                  </div>
                ))}
              </div>
              {[
                t('client.routing.chainNote', worded()),
                t('client.routing.newFlowsNote'),
                t(view === 'bundle' ? 'client.routing.bundleNote' : named ? 'client.routing.domainNote' : 'client.routing.matchNote', worded()),
              ].filter(Boolean).map((note) => <p key={note} className="rt-note">{note}</p>)}
            </div>
          </div>
          <input
            ref={fileRef}
            type="file"
            accept=".qdr,text/plain"
            hidden
            onChange={(e) => {
              const file = e.target.files?.[0];
              e.target.value = '';
              if (file) void doImport(file);
            }}
          />
        </div>
      </Card>

      <LazyMount when={picking}>
        <ProcessPickerDialog
          open={picking}
          onOpenChange={setPicking}
          existing={existing}
          onPick={onPick}
        />
      </LazyMount>

      <Dialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={t(view === 'bundle' ? 'client.routing.resetBundles' : named ? 'client.routing.resetDomains' : 'client.routing.resetRules', worded())}
        okText={t('client.settings.resetConfirm')}
        okDanger
        confirmLoading={busy}
        onOk={() => void doReset()}
      >
        <p style={{ margin: 0 }}>
          {t(view === 'bundle' ? 'client.routing.resetBundlesDesc'
            : named ? 'client.routing.resetDomainsDesc'
              : byDomain ? 'client.routing.resetProcessDesc' : 'client.routing.resetRulesDesc', worded())}
        </p>
      </Dialog>

      <Dialog
        open={naming}
        onOpenChange={(o) => { if (!o) setName(''); setNaming(o); }}
        title={t('client.routing.addDomain')}
        okText={t('client.routing.addDomain')}
        okDisabled={!host || hostTaken}
        confirmLoading={busy}
        onOk={() => void doName()}
      >
        <Field
          label={t('client.routing.domainHint')}
          error={hostTaken ? t('client.routing.domainTaken') : undefined}
        >
          <Input
            autoFocus
            value={name}
            placeholder="example.com"
            spellCheck={false}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') void doName(); }}
          />
        </Field>
      </Dialog>
    </div>
  );
}
