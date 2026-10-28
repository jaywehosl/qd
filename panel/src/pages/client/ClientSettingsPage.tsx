import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { Button, Card, Dialog, Input, Select, Switch, Tag, toast } from '@/components/ds';
import { SettingListItem, Spin } from '@/components/ui';
import { HttpUtil, SizeFormatter } from '@/utils';
import { useClientState } from '@/hooks/useClientState';
import { useClientSettings } from '@/layouts/ClientSettingsController';
import { resetAll } from '@/stores/notificationStore';
import UpdateButton from './UpdateButton';
import RelaysCard, { useRelays } from './RelaysCard';
import { useTheme } from '@/hooks/useTheme';
import { HOST_EVENT, hostChecks, navLow, phone, setNavLow } from '@/lib/phone';

interface AboutPayload {
  version?: string;
  tag?: string;
  up?: number;
  down?: number;
  expiresAt?: number;
}

function remainingLabel(ms: number | undefined, never: string): string {
  if (!ms) return never;
  const left = ms - Date.now();
  if (left <= 0) return '—';
  const days = Math.floor(left / 86400000);
  const hours = Math.floor((left % 86400000) / 3600000);
  return days > 0 ? `${days}d ${hours}h` : `${hours}h`;
}

export default function ClientSettingsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { reset } = useClientState();
  const { settings, patch } = useClientSettings();
  const { data: relays } = useRelays();

  const [confirm, setConfirm] = useState<'data' | 'all' | null>(null);
  const [low, setLow] = useState(navLow);

  const onPhone = !!phone();
  const { follow, setFollow } = useTheme();
  const [checks, setChecks] = useState(hostChecks);
  const saveJournal = () => {
    const said = phone()?.journal?.() ?? '';
    if (said.startsWith('Downloads/')) toast.success(said);
    else toast.warning(said);
  };
  useEffect(() => {
    const read = () => setChecks(hostChecks());
    window.addEventListener(HOST_EVENT, read);
    return () => window.removeEventListener(HOST_EVENT, read);
  }, []);

  const { data: about } = useQuery<AboutPayload | null>({
    queryKey: ['client', 'about'],
    queryFn: async () => {
      const msg = await HttpUtil.get<AboutPayload>('/client/api/about', undefined, { silent: true });
      return msg?.success ? (msg.obj ?? null) : null;
    },
    refetchInterval: 10000,
  });

  const doReset = useCallback(async (withSubscription: boolean) => {
    await reset(withSubscription);
    if (withSubscription) resetAll();
    await queryClient.invalidateQueries({ queryKey: ['client'] });
    setConfirm(null);
    if (withSubscription) navigate('/client');
  }, [reset, queryClient, navigate]);

  if (!settings) {
    return (
      <section className="feed-section">
        <Spin spinning size="large" />
      </section>
    );
  }

  return (
    <section className="feed-section">
      <div className="section-header">
        <h2>{t('client.menu.settings')}</h2>
        <UpdateButton />
      </div>

      <div className="client-settings cset">
        <Card title={t('client.settings.subscription')}>
          <div className="cset-facts">
            <Tag tone="primary">{about?.tag || '—'}</Tag>
            <Tag>{remainingLabel(about?.expiresAt, t('client.settings.noExpiry'))}</Tag>
            <Tag>↑ {SizeFormatter.sizeFormat(about?.up ?? 0)} ↓ {SizeFormatter.sizeFormat(about?.down ?? 0)}</Tag>
          </div>
          <SettingListItem
            paddings="small"
            title={t('client.settings.refreshInterval')}
            description={t('client.settings.refreshIntervalDesc')}
          >
            <Input
              type="number"
              min={1}
              max={1440}
              value={settings.refreshMinutes}
              onChange={(e) => patch({ refreshMinutes: Number(e.target.value) || 1 })}
            />
          </SettingListItem>
          <div className="cset-foot">
            <Button danger size="sm" onClick={() => setConfirm('all')}>{t('client.settings.resetAll')}</Button>
          </div>
        </Card>

        {relays?.supported && <RelaysCard data={relays} />}

        {onPhone ? (
          <Card title={t('client.settings.device')}>
            <SettingListItem paddings="small" title={t('client.settings.connectOnLaunch')}>
              <Switch
                checked={settings.manualBehaviour === 'openConnect' || settings.manualBehaviour === 'connect'}
                onChange={(v) => patch({ manualBehaviour: v ? 'openConnect' : 'open' })}
              />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('client.settings.theme')}
              description={t('client.settings.themeDesc')}
            >
              <Select
                value={follow ? 'system' : 'button'}
                onChange={(v) => setFollow(v === 'system')}
                options={[
                  { value: 'button', label: t('client.settings.themeButton') },
                  { value: 'system', label: t('client.settings.themeSystem') },
                ]}
              />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('client.settings.navLow')}
              description={t('client.settings.navLowDesc')}
            >
              <Switch checked={low} onChange={(v) => { setNavLow(v); setLow(v); }} />
            </SettingListItem>

            {(settings.fixedRate ?? 0) > 0 && (
              <SettingListItem paddings="small" title={t('client.settings.rate')}>
                <b>{settings.fixedRate} Mbit/s</b>
              </SettingListItem>
            )}

            <div className="cset-head">{t('client.settings.resilience')}</div>
            {checks.map((c) => (
              <SettingListItem
                key={c.id}
                paddings="small"
                title={(
                  <span className="cset-check">
                    <i className={`cset-check__dot is-${c.state}`} />
                    {t(`client.settings.host.${c.id}.${c.state}`)}
                  </span>
                )}
              >
                <Button size="sm" onClick={() => phone()?.open?.(c.id)}>{t('client.settings.open')}</Button>
              </SettingListItem>
            ))}

            <SettingListItem paddings="small" title={t('client.settings.journal')}>
              <Button size="sm" onClick={saveJournal}>
                {t('client.settings.journalSave')}
              </Button>
            </SettingListItem>
          </Card>
        ) : (
          <Card title={t('client.settings.startup')}>
            <SettingListItem paddings="small" title={t('client.settings.autostart')}>
              <Switch checked={settings.autostart} onChange={(v) => patch({ autostart: v })} />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('client.settings.autostartBehaviour')}
              description={t('client.settings.autostartBehaviourDesc')}
            >
              <Select
                value={settings.autostartBehaviour}
                disabled={!settings.autostart}
                onChange={(v) => patch({ autostartBehaviour: v })}
                options={[
                  { value: 'tray', label: t('client.settings.behavTray') },
                  { value: 'open', label: t('client.settings.behavOpen') },
                  { value: 'connect', label: t('client.settings.behavConnect') },
                  { value: 'openConnect', label: t('client.settings.behavOpenConnect') },
                ]}
              />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('client.settings.manualBehaviour')}
              description={t('client.settings.manualBehaviourDesc')}
            >
              <Select
                value={settings.manualBehaviour}
                onChange={(v) => patch({ manualBehaviour: v })}
                options={[
                  { value: 'tray', label: t('client.settings.behavTray') },
                  { value: 'open', label: t('client.settings.behavOpen') },
                  { value: 'openConnect', label: t('client.settings.behavOpenConnect') },
                ]}
              />
            </SettingListItem>
          </Card>
        )}

        <div className="cset-end">
          <span>qd {about?.version || ''}</span>
          <Button size="sm" onClick={() => setConfirm('data')}>{t('client.settings.resetData')}</Button>
        </div>
      </div>

      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm === 'all' ? t('client.settings.resetAll') : t('client.settings.resetData')}
        okText={t('client.settings.resetConfirm')}
        okDanger
        onOk={() => void doReset(confirm === 'all')}
      >
        <p style={{ margin: 0 }}>
          {confirm === 'all' ? t('client.settings.resetAllDesc') : t('client.settings.resetDataDesc')}
        </p>
      </Dialog>
    </section>
  );
}
