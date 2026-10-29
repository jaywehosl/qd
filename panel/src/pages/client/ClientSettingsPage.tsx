import { unit } from '@/lib/unit';
import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { DownloadOutlined, RightOutlined } from '@ant-design/icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { Button, Card, Dialog, Input, Select, Switch, Tag, toast } from '@/components/ds';
import { SettingListItem, Spin } from '@/components/ui';
import { HttpUtil, LanguageManager, SizeFormatter } from '@/utils';
import { useClientState } from '@/hooks/useClientState';
import { useClientSettings } from '@/layouts/ClientSettingsController';
import { resetAll } from '@/stores/notificationStore';
import { VersionPill } from './UpdateButton';
import RelaysCard, { useRelays } from './RelaysCard';
import { useTheme } from '@/hooks/useTheme';
import { HOST_EVENT, colsSwap, hostChecks, navLow, pageScale, phone, setColsSwap, setNavLow, setUiScale, uiScale, usePortrait } from '@/lib/phone';

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
  return days > 0 ? `${days}${unit('d')} ${hours}${unit('h')}` : `${hours}${unit('h')}`;
}

const SCALE_TICKS = [80, 85, 90, 95, 100, 105, 110, 115, 120];
const THUMB = 20;

const stick = (scale: number) => {
  const held = Math.min(1.2, Math.max(0.8, scale));
  const tick = Math.round(held * 20) / 20;
  return Math.abs(held - tick) < 0.012 ? tick : held;
};

function ScaleSlider({ label }: { label: string }) {
  const [value, setValue] = useState(uiScale);
  const track = useRef<HTMLSpanElement>(null);
  const drag = useRef<{ x: number; from: number; width: number; top: number } | null>(null);
  const pending = useRef(value);
  const frame = useRef(0);

  const show = (next: number) => {
    setValue(next);
    pending.current = next;
    if (frame.current) return;
    frame.current = requestAnimationFrame(() => {
      frame.current = 0;
      setUiScale(pending.current, false);
      const held = drag.current;
      const box = track.current?.getBoundingClientRect();
      if (held && box) window.scrollBy(0, box.top - held.top / pageScale());
    });
  };

  const grab = (e: PointerEvent<HTMLSpanElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    const scale = pageScale();
    const at = Math.min(1, Math.max(0, (e.clientX - box.left - THUMB / 2) / (box.width - THUMB)));
    const from = Math.abs(at - (value - 0.8) / 0.4) < 0.08 ? value : 0.8 + at * 0.4;
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { x: e.clientX * scale, from, width: (box.width - THUMB) * scale, top: box.top * scale };
    show(stick(from));
  };

  const move = (e: PointerEvent<HTMLSpanElement>) => {
    const held = drag.current;
    if (held) show(stick(held.from + ((e.clientX * pageScale() - held.x) / held.width) * 0.4));
  };

  const drop = () => {
    if (!drag.current) return;
    drag.current = null;
    const kept = Math.round(pending.current * 100) / 100;
    setValue(kept);
    pending.current = kept;
    setUiScale(kept);
  };

  const key = (e: KeyboardEvent<HTMLSpanElement>) => {
    const step = e.key === 'ArrowRight' || e.key === 'ArrowUp' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowDown' ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const next = stick(Math.round(value * 20 + step) / 20);
    setValue(next);
    setUiScale(next);
  };

  return (
    <span className="cset-scale">
      <span
        ref={track}
        className="cset-track"
        role="slider"
        tabIndex={0}
        aria-label={label}
        aria-valuemin={80}
        aria-valuemax={120}
        aria-valuenow={Math.round(value * 100)}
        style={{ '--fill': (value - 0.8) / 0.4 } as CSSProperties}
        onPointerDown={grab}
        onPointerMove={move}
        onPointerUp={drop}
        onPointerCancel={drop}
        onKeyDown={key}
      >
        <span className="cset-thumb" />
      </span>
      <span className="cset-ticks" aria-hidden="true">
        {SCALE_TICKS.map((v) => <i key={v} className={v % 10 ? undefined : 'is-major'} />)}
      </span>
    </span>
  );
}

export default function ClientSettingsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { reset } = useClientState();
  const { settings, patch } = useClientSettings();
  const { data: relays } = useRelays();

  const [confirm, setConfirm] = useState(false);
  const [low, setLow] = useState(navLow);
  const [swapped, setSwapped] = useState(colsSwap);
  const portrait = usePortrait();
  const [lang, setLang] = useState(() => LanguageManager.getLanguage());

  const onPhone = !!phone();
  const { follow, setFollow } = useTheme();
  const [checks, setChecks] = useState(hostChecks);
  const saveJournal = async () => {
    const host = phone();
    if (host) {
      const said = host.journal?.() ?? '';
      if (said.startsWith('Downloads/')) toast.success(said);
      else toast.warning(said);
      return;
    }
    const msg = await HttpUtil.post<{ name: string }>('/client/api/journal', {}, {
      headers: { 'Content-Type': 'application/json' },
      silent: true,
    });
    if (msg?.success && msg.obj) toast.success(t('client.settings.journalSaved', { name: msg.obj.name }));
    else toast.error(msg?.msg || t('somethingWentWrong'));
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

  const doReset = useCallback(async () => {
    await reset(true);
    resetAll();
    await queryClient.invalidateQueries({ queryKey: ['client'] });
    setConfirm(false);
    navigate('/client');
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
      </div>

      <div className="client-settings cset">
        <Card title={t('client.settings.subscription')} extra={<Tag tone="primary">{about?.tag || '—'}</Tag>}>
          <div className="cset-facts">
            <Tag>{remainingLabel(about?.expiresAt, t('client.settings.noExpiry'))}</Tag>
            <Tag>↓ {SizeFormatter.short(about?.down)} ↑ {SizeFormatter.short(about?.up)}</Tag>
          </div>
          <SettingListItem paddings="small" title={t('client.settings.refreshInterval')}>
            <span className="cset-num">
              <Input
                type="number"
                min={1}
                max={1440}
                value={settings.refreshMinutes}
                onChange={(e) => patch({ refreshMinutes: Math.min(1440, Number(e.target.value) || 1) })}
              />
              <span>{unit('min')}</span>
            </span>
          </SettingListItem>
          <div className="cset-foot">
            <VersionPill version={about?.version} />
            <Button variant="text" danger onClick={() => setConfirm(true)}>{t('client.settings.resetAll')}</Button>
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

            <SettingListItem paddings="small" title={t('client.settings.uiScale')}>
              <ScaleSlider label={t('client.settings.uiScale')} />
            </SettingListItem>

            {portrait ? (
              <SettingListItem
                paddings="small"
                title={t('client.settings.navLow')}
                description={t('client.settings.navLowDesc')}
              >
                <Switch checked={low} onChange={(v) => { setNavLow(v); setLow(v); }} />
              </SettingListItem>
            ) : (
              <SettingListItem
                paddings="small"
                title={t('client.settings.colsSwap')}
                description={t('client.settings.colsSwapDesc')}
              >
                <Switch checked={swapped} onChange={(v) => { setColsSwap(v); setSwapped(v); }} />
              </SettingListItem>
            )}

            <SettingListItem paddings="small" title={t('pages.settings.language')}>
              <Select
                value={lang}
                onChange={(v) => { LanguageManager.apply(v); setLang(v); }}
                options={LanguageManager.supportedLanguages.map((l) => ({ value: l.value, label: l.name }))}
              />
            </SettingListItem>

            {(settings.fixedRate ?? 0) > 0 && (
              <SettingListItem paddings="small" title={t('client.settings.rate')}>
                <b>{settings.fixedRate} {unit('Mbit/s')}</b>
              </SettingListItem>
            )}

            <div className="cset-head">{t('client.settings.resilience')}</div>
            <div className="cset-links">
              {checks.map((c) => (
                <button key={c.id} type="button" className="cset-link" onClick={() => phone()?.open?.(c.id)}>
                  <span className="cset-link__lead"><i className={`cset-check__dot is-${c.state}`} /></span>
                  <span className="cset-link__text">{t(`client.settings.host.${c.id}.${c.state}`)}</span>
                  <RightOutlined className="cset-link__go" />
                </button>
              ))}
              <button type="button" className="cset-link" onClick={() => void saveJournal()}>
                <span className="cset-link__lead"><DownloadOutlined /></span>
                <span className="cset-link__text">{t('client.settings.journalRow')}</span>
                <RightOutlined className="cset-link__go" />
              </button>
            </div>
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

            <div className="cset-head">{t('client.settings.diagnostics')}</div>
            <div className="cset-links">
              <button type="button" className="cset-link" onClick={() => void saveJournal()}>
                <span className="cset-link__lead"><DownloadOutlined /></span>
                <span className="cset-link__text">{t('client.settings.journalRow')}</span>
                <RightOutlined className="cset-link__go" />
              </button>
            </div>
          </Card>
        )}

      </div>

      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t('client.settings.resetAll')}
        okText={t('client.settings.resetConfirm')}
        okDanger
        onOk={() => void doReset()}
      >
        <p style={{ margin: 0 }}>{t('client.settings.resetAllDesc')}</p>
      </Dialog>
    </section>
  );
}
