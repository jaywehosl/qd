import { lazy, useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, DownloadOutlined, DownOutlined, PlusOutlined, UploadOutlined } from '@ant-design/icons';

import { Alert, Button, Card, Dialog, Select, Tag, toast } from '@/components/ds';
import { Spin } from '@/components/ui';
import { LazyMount } from '@/components/utility';
import { ROUTING_ROLES, type RoutingRole } from '@/schemas/client-routing';
import { useClientRouting } from '@/hooks/useClientRouting';
import { HttpUtil } from '@/utils';
const ProcessPickerDialog = lazy(() => import('./ProcessPickerDialog'));

interface RoutingSectionProps {
  connected: boolean;
  onReconnect: () => Promise<unknown>;
}

export default function RoutingSection({ connected, onReconnect }: RoutingSectionProps) {
  const { t } = useTranslation();
  const { state, loading, setRole, setDefaultRole, remove, add, reset, refresh } = useClientRouting();

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

  const roleOptions = useMemo(
    () => ROUTING_ROLES.map((r) => ({ value: r, label: t(`client.routing.role.${r}`) })),
    [t],
  );

  const baseOptions = useMemo(
    () => roleOptions.filter((o) => o.value === 'direct' || o.value === 'tunnel'
      || o.value === state?.defaultRole),
    [roleOptions, state?.defaultRole],
  );

  const existing = useMemo(
    () => new Set((state?.rules ?? []).map((r) => (r.path || r.process).toLowerCase())),
    [state],
  );

  const onPick = useCallback((pick: { process: string; path?: string }) => {
    if (!state) return;
    const role: RoutingRole = state.defaultRole === 'direct' ? 'tunnel' : 'direct';
    void add({ ...pick, role });
  }, [state, add]);

  const fileRef = useRef<HTMLInputElement>(null);

  const doExport = useCallback(async () => {
    const msg = await HttpUtil.get<{ code: string; name: string }>('/client/api/routing/export');
    if (!msg?.success || !msg.obj) return;
    const url = URL.createObjectURL(new Blob([msg.obj.code], { type: 'application/octet-stream' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = msg.obj.name;
    link.click();
    URL.revokeObjectURL(url);
  }, []);

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
      await reset();
      setConfirmReset(false);
    } finally {
      setBusy(false);
    }
  }, [reset]);

  if (loading || !state) {
    return <div className="rt-boot"><Spin spinning size="large" /></div>;
  }

  const { rules, defaultRole, applyMode, pendingRestart } = state;

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
            <span>{t('client.routing.everythingElseDesc')}</span>
          </div>
          <Select
            value={defaultRole}
            options={baseOptions}
            onChange={(v) => void setDefaultRole(v as RoutingRole)}
          />
        </div>
      </Card>

      <Card
        title={t('client.routing.rules')}
        extra={(
          <Button size="sm" icon={<PlusOutlined />} onClick={() => setPicking(true)}>
            {t('client.routing.addRule')}
          </Button>
        )}
        flush
      >
        <div className="rt-rules">
          {rules.length === 0 ? (
            <div className="rt-empty">{t('client.routing.noRules')}</div>
          ) : rules.map((r) => (
            <div key={r.id} className="rt-rule">
              <span className={`rt-dot${r.running ? ' is-up' : ''}`} />
              {r.icon
                ? <img className="rt-proc__icon" src={r.icon} alt="" aria-hidden="true" />
                : <span className="rt-proc__icon rt-proc__icon--blank" aria-hidden="true" />}
              <div className="rt-rule__id">
                <span className="rt-rule__name">{r.process}</span>
                {r.path && <span className="rt-rule__path">{r.path}</span>}
              </div>
              {r.matched ? <Tag>{t('client.routing.flows', { count: r.matched })}</Tag> : null}
              <Select
                value={r.role}
                options={roleOptions}
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
          <div className="rt-danger__actions">
            <Button size="sm" icon={<DownloadOutlined />} onClick={() => void doExport()}>
              {t('client.routing.exportRules')}
            </Button>
            <Button size="sm" icon={<UploadOutlined />} onClick={() => fileRef.current?.click()}>
              {t('client.routing.importRules')}
            </Button>
            <Button size="sm" danger disabled={rules.length === 0} onClick={() => setConfirmReset(true)}>
              {t('client.routing.resetRules')}
            </Button>
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

        <div className={`rt-legend__fold${legendOpen ? ' is-open' : ''}`}>
          <div className="rt-legend__inner">
            <div className="rt-legend">
              {ROUTING_ROLES.map((r) => (
                <div key={r} className="rt-legend__row">
                  <Tag tone={r === 'direct' ? 'warning' : 'primary'}>{t(`client.routing.role.${r}`)}</Tag>
                  <span>{t(`client.routing.roleDesc.${r}`)}</span>
                </div>
              ))}
            </div>
            <p className="rt-note">{t('client.routing.newFlowsNote')}</p>
            <p className="rt-note">{t('client.routing.matchNote')}</p>
          </div>
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
        title={t('client.routing.resetRules')}
        okText={t('client.settings.resetConfirm')}
        okDanger
        confirmLoading={busy}
        onOk={() => void doReset()}
      >
        <p style={{ margin: 0 }}>{t('client.routing.resetRulesDesc')}</p>
      </Dialog>
    </div>
  );
}
