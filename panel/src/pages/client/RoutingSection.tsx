import { lazy, useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, DownOutlined, PlusOutlined } from '@ant-design/icons';

import { Alert, Button, Card, Dialog, Select, Tag } from '@/components/ds';
import { Spin } from '@/components/ui';
import { LazyMount } from '@/components/utility';
import { ROUTING_ROLES, type RoutingRole } from '@/schemas/client-routing';
import { useClientRouting } from '@/hooks/useClientRouting';
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

  const existing = useMemo(
    () => new Set((state?.rules ?? []).map((r) => (r.path || r.process).toLowerCase())),
    [state],
  );

  const onPick = useCallback((pick: { process: string; path?: string }) => {
    if (!state) return;
    const role: RoutingRole = state.defaultRole === 'direct' ? 'tunnel' : 'direct';
    void add({ ...pick, role });
  }, [state, add]);

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
      {/* Android builds its per-app rules into VpnService when the tunnel comes
          up, so a change cannot land live. Restarting is left to the person —
          doing it automatically would read as a spontaneous drop. */}
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
            options={roleOptions}
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
              {/* Same icon the picker showed, read from the executable — so a
                  rule looks the same whether its program is running or not. */}
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
          <Button size="sm" danger disabled={rules.length === 0} onClick={() => setConfirmReset(true)}>
            {t('client.routing.resetRules')}
          </Button>
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
