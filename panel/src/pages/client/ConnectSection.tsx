import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { PoweroffOutlined, ReloadOutlined } from '@ant-design/icons';

import { Button, Card, Switch, Tag, Tooltip, TooltipProvider, toast } from '@/components/ds';
import { fetchClientNodes, type ClientState } from '@/hooks/useClientState';

interface ConnectSectionProps {
  state: ClientState;
  onConnect: () => Promise<ClientState | null>;
  onDisconnect: () => Promise<ClientState | null>;
  onEgress: (v: boolean) => Promise<ClientState | null>;
  onAdblock: (v: boolean) => Promise<ClientState | null>;
  onRefresh: () => Promise<unknown>;
  refreshing: boolean;
}

export default function ConnectSection({
  state, onConnect, onDisconnect, onEgress, onAdblock, onRefresh, refreshing,
}: ConnectSectionProps) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);

  const { data: nodes = [] } = useQuery({
    queryKey: ['client', 'nodes'],
    queryFn: fetchClientNodes,
    refetchInterval: 10000,
  });

  const toggleTunnel = useCallback(async () => {
    setBusy(true);
    try {
      await (state.connected ? onDisconnect() : onConnect());
    } finally {
      setBusy(false);
    }
  }, [state.connected, onConnect, onDisconnect]);

  // A refused exit has to be visible — silently staying off is the one
  // behaviour that leaves the operator guessing.
  const flipEgress = useCallback(async (v: boolean) => {
    const next = await onEgress(v);
    if (!next) toast.error(t('client.connect.exitRefused'));
  }, [onEgress, t]);

  const refresh = useCallback(async () => {
    const result = await onRefresh();
    if (result) toast.success(t('client.connect.refreshed'));
  }, [onRefresh, t]);

  const { total, reachable } = state.nodes;

  return (
    <TooltipProvider>
      <div className="cs">
        <div className="cs-grid">
          <Card title={t('client.connect.nodes')} flush>
            <div className="cs-nodes">
              {nodes.length === 0 ? (
                <div className="cs-empty">{t('client.connect.noNodes')}</div>
              ) : nodes.map((n) => (
                <div key={n.id} className={`cs-node${n.selected ? ' is-selected' : ''}`}>
                  <span className={`cs-dot${n.reachable ? ' is-up' : ''}`} />
                  <span className="cs-node__name">{n.name}</span>
                  {n.role && <Tag>{n.role}</Tag>}
                  <span className="cs-node__latency">
                    {n.reachable && n.latencyMs ? `${n.latencyMs} ms` : '—'}
                  </span>
                  {n.selected && <Tag tone="success">{t('client.connect.carrying')}</Tag>}
                </div>
              ))}
            </div>
          </Card>

          <Card>
            <div className="cs-main">
              <div className="cs-status">
                {state.connected && state.node ? (
                  <>
                    <Tag tone="success">{state.node.name}</Tag>
                    {state.node.latencyMs ? <Tag>{state.node.latencyMs} ms</Tag> : null}
                  </>
                ) : (
                  <Tag>{t('client.connect.idle')}</Tag>
                )}
                <Tooltip title={t('client.connect.nodesHint')}>
                  <Tag tone={reachable > 0 ? 'primary' : 'danger'}>
                    {t('client.connect.nodesAvailable', { reachable, total })}
                  </Tag>
                </Tooltip>
              </div>

              <button
                type="button"
                className={`cs-power${state.connected ? ' is-on' : ''}`}
                disabled={busy}
                onClick={toggleTunnel}
              >
                <PoweroffOutlined />
                <span>{state.connected ? t('client.connect.disconnect') : t('client.connect.connect')}</span>
              </button>

              <div className="cs-toggles">
                {state.allowExit !== false && (
                  <label className="cs-toggle">
                    <Switch
                      checked={state.egress}
                      onChange={(v) => void flipEgress(v)}
                    />
                    <span>+egress</span>
                  </label>
                )}
                <label className="cs-toggle">
                  <Switch checked={state.adblock} onChange={(v) => void onAdblock(v)} />
                  <span>+adblock</span>
                </label>
                <Button
                  size="sm"
                  icon={<ReloadOutlined />}
                  loading={refreshing}
                  onClick={() => void refresh()}
                >
                  {t('client.connect.refresh')}
                </Button>
              </div>
            </div>

          </Card>
        </div>
      </div>
    </TooltipProvider>
  );
}
