import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { CloseOutlined, SyncOutlined } from '@ant-design/icons';

import { Button, Card, Input, Select, Switch, toast } from '@/components/ds';
import { HttpUtil } from '@/utils';
import { useClientSettings } from '@/layouts/ClientSettingsController';

interface RelayState {
  reach: number;
  shake: number;
  checked: number;
  fault?: string;
  broken?: boolean;
}

interface RelayRow {
  id: number;
  enabled: boolean;
  name: string;
  server: string;
  transport: string;
  security: string;
  state: RelayState;
}

export interface RelaysPayload {
  mode: string;
  supported: boolean;
  relays: RelayRow[];
}

export const RELAYS_KEY = ['client', 'relays'];

export function useRelays() {
  return useQuery<RelaysPayload | null>({
    queryKey: RELAYS_KEY,
    queryFn: async () => {
      const msg = await HttpUtil.get<RelaysPayload>('/client/api/relays', undefined, { silent: true });
      return msg?.success ? (msg.obj ?? null) : null;
    },
    refetchInterval: 3000,
  });
}

function statusOf(r: RelayRow): string {
  const s = r.state;
  if (!r.enabled) return 'idle';
  if (s.broken || s.reach < 0) return 'down';
  if (s.shake > 0) return 'up';
  if (s.shake < 0) return 'half';
  return 'idle';
}

export default function RelaysCard({ data }: { data: RelaysPayload }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { settings, patch } = useClientSettings();
  const [link, setLink] = useState('');
  const [busy, setBusy] = useState(false);
  const [probing, setProbing] = useState(false);

  const call = async (path: string, body: object) => {
    setBusy(true);
    const msg = await HttpUtil.post<RelaysPayload>(`/client/api/relays/${path}`, body, {
      headers: { 'Content-Type': 'application/json' },
      silent: true,
    });
    setBusy(false);
    if (!msg?.success) {
      toast.error(msg?.msg || t('somethingWentWrong'));
      return false;
    }
    queryClient.setQueryData(RELAYS_KEY, msg.obj ?? null);
    return true;
  };

  const probe = async () => {
    setProbing(true);
    await call('probe', {});
    setProbing(false);
  };

  useEffect(() => {
    void probe();
  }, []);

  const add = async () => {
    if (link.trim() && (await call('add', { link: link.trim() }))) setLink('');
  };

  const mode = settings?.relayMode || data.mode || 'auto';
  const stranded = mode === 'only' && !data.relays.some((r) => r.enabled && !r.state.broken);

  const tone: Record<string, string> = { off: 'default', auto: 'tunnel', only: 'egress' };

  return (
    <Card
      title={t('client.settings.relays')}
      extra={(
        <div className="rl-head">
          <Button
            variant="text"
            icon={<SyncOutlined spin={probing} />}
            aria-label={t('client.settings.relayCheck')}
            title={t('client.settings.relayCheck')}
            disabled={probing || data.relays.length === 0}
            onClick={() => void probe()}
          />
          <Select
            className={`rt-role rt-role--${tone[mode] ?? 'tunnel'}`}
            value={mode}
            onChange={(v) => patch({ relayMode: v })}
            options={[
              { value: 'off', label: t('client.settings.relayOff') },
              { value: 'auto', label: t('client.settings.relayAuto') },
              { value: 'only', label: t('client.settings.relayOnly') },
            ]}
          />
        </div>
      )}
    >
      <div className="rl-list">
        {data.relays.length === 0 && (
          <div className={`cset-empty${stranded ? ' is-warn' : ''}`}>
            {t(stranded ? 'client.settings.relayNoneForced' : 'client.settings.relayNone')}
          </div>
        )}
        {data.relays.map((r) => (
          <div
            key={r.id}
            className={`rl-row${r.enabled ? '' : ' is-off'}`}
            title={r.state.fault || [r.server, r.transport, r.security].filter(Boolean).join(' · ')}
          >
            <i className={`rl-dot is-${statusOf(r)}`} />
            <span className="rl-name">{r.name || r.server || `#${r.id}`}</span>
            <Switch
              checked={r.enabled}
              disabled={busy}
              onChange={(v) => void call('enable', { id: r.id, enabled: v })}
            />
            <button
              type="button"
              className="rl-drop"
              aria-label={t('client.settings.relayRemove')}
              disabled={busy}
              onClick={() => void call('remove', { id: r.id })}
            >
              <CloseOutlined />
            </button>
          </div>
        ))}
      </div>

      {stranded && data.relays.length > 0 && <p className="cset-warn">{t('client.settings.relayStranded')}</p>}

      <div className="rl-add">
        <Input
          value={link}
          placeholder="vless://"
          spellCheck={false}
          onChange={(e) => setLink(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && void add()}
        />
        <Button disabled={busy || !link.trim()} onClick={() => void add()}>{t('client.settings.relayAdd')}</Button>
      </div>
    </Card>
  );
}
