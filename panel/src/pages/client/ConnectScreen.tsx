import { useCallback, useEffect, useRef, useState, type CSSProperties } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { SnippetsOutlined } from '@ant-design/icons';

import { Switch, toast } from '@/components/ds';
import { HttpUtil } from '@/utils';
import { fetchClientNodes, type ClientState } from '@/hooks/useClientState';
import { shortBits } from '@/lib/rate';
import { progressOf, useUpdate } from '@/hooks/useUpdate';
import { useSetup } from '@/hooks/useSetup';
import { HOST_EVENT, phone, worded } from '@/lib/phone';
import FlowCanvas from './FlowCanvas';
import UpdateShell from './UpdateShell';
import { updateWord } from './UpdateButton';
import '@/skin/connect.css';

if (import.meta.env.DEV) void import('@/skin/connect-slow.css');

const GATE = 0.45;
const RISE = 1.3;
const FINISH = 0.8;
const SPLIT = 1.6;

const RING = 2400;
const BEAT = 1100;
const GETTING = 1400;
const SETTLE = 1740;
const PULSE = 1600;

const PERIODS = [
  { minutes: 30, label: '30m', when: 'client.update.when30m' },
  { minutes: 720, label: '12h', when: 'client.update.when12h' },
  { minutes: 1440, label: '1d', when: 'client.update.when1d' },
  { minutes: -1, label: '∞', when: '' },
];

type Stage = 'idle' | 'installing' | 'installed' | 'getting';
type Phase = 'wizard' | 'installing' | 'installed' | 'import' | 'getting' | 'main';

interface ConnectScreenProps {
  state: ClientState;
  onConnect: () => Promise<ClientState | null>;
  onDisconnect: () => Promise<ClientState | null>;
  onEgress: (v: boolean) => Promise<ClientState | null>;
  onAdblock: (v: boolean) => Promise<ClientState | null>;
  onRefresh: () => Promise<unknown>;
  onImport: (uri: string) => Promise<ClientState | null>;
  onSettle: () => Promise<unknown>;
  onStage: (busy: boolean) => void;
  refreshing: boolean;
}

function span(at: number, from: number, to: number) {
  if (to <= from) return at >= to ? 1 : 0;
  const v = (at - from) / (to - from);
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

function dip(spread: number, gone: number, back: number) {
  return Math.max(1 - span(spread, 0, gone), span(spread, back, 1));
}

const CURVES = [
  'cubic-bezier(0.34, 1.56, 0.64, 1)',
  'cubic-bezier(0.34, 1.46, 0.64, 1)',
  'cubic-bezier(0.36, 1.64, 0.62, 1)',
  'cubic-bezier(0.32, 1.5, 0.66, 1)',
  'cubic-bezier(0.35, 1.7, 0.6, 1)',
];

const anyCurve = () => CURVES[Math.floor(Math.random() * CURVES.length)];
const eased = (curve: string) => ({ '--cx-ease': curve }) as CSSProperties;

function useSlide(now: string) {
  const track = useRef({ now, was: now, curve: CURVES[0] });
  if (track.current.now !== now) track.current = { now, was: track.current.now, curve: anyCurve() };
  const { was, curve } = track.current;
  const cls = (name: string) => {
    if (name === now) return was === now ? ' is-rest' : ' is-on';
    return name === was ? ' is-off' : '';
  };
  return [cls, eased(curve)] as const;
}

function Roll({ text, tone: shade, inline }: { text: string; tone: string; inline?: boolean }) {
  const tone = text ? shade : '';
  const track = useRef({ faces: [{ text, tone }, { text: '', tone: '' }], top: 0, moved: false, curve: CURVES[0] });
  const held = track.current;
  if (held.faces[held.top].text !== text || held.faces[held.top].tone !== tone) {
    const top = 1 - held.top;
    const faces = [...held.faces];
    faces[top] = { text, tone };
    track.current = { faces, top, moved: true, curve: anyCurve() };
  }
  const { faces, top, moved, curve } = track.current;
  return (
    <span className={`cx-chips${inline ? ' is-inline' : ''}`} style={eased(curve)}>
      {faces.map((f, i) => (
        <span
          key={i}
          className={`cx-chip${f.tone}${f.text ? '' : ' is-void'}${i === top ? (moved ? ' is-on' : ' is-rest') : moved ? ' is-off' : ''}`}
        >
          {f.text}
        </span>
      ))}
    </span>
  );
}

const rate = shortBits;

let cheered = '';

export default function ConnectScreen({
  state, onConnect, onDisconnect, onEgress, onAdblock, onRefresh, onImport, onSettle, onStage, refreshing,
}: ConnectScreenProps) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const { info: upd, run: runUpdate, postpone } = useUpdate();
  const { setup, install } = useSetup();
  const [spot, setSpot] = useState(0);
  const [snap, setSnap] = useState(false);
  const period = spot % PERIODS.length;
  const [starting, setStarting] = useState(false);

  const [stage, setStage] = useState<Stage>('idle');
  const [autostart, setAutostart] = useState(setup?.autostart ?? true);
  const [desktop, setDesktop] = useState(true);
  const [ring, setRing] = useState(0);
  const [uri, setUri] = useState('');
  const [trouble, setTrouble] = useState('');
  const field = useRef<HTMLInputElement>(null);
  const root = useRef<HTMLDivElement>(null);
  const tempo = useCallback(
    () => (root.current ? parseFloat(getComputedStyle(root.current).getPropertyValue('--cx-t')) || 1 : 1),
    [],
  );
  const wait = (ms: number) => new Promise<void>((done) => { window.setTimeout(done, ms * tempo()); });

  const phase: Phase = stage !== 'idle' ? stage : setup?.offered ? 'wizard' : !state.imported ? 'import' : 'main';
  const main = phase === 'main';
  const [lag, setLag] = useState<Phase>(phase);
  useEffect(() => {
    if (phase === 'main' && (lag === 'import' || lag === 'getting')) {
      const id = window.setTimeout(() => setLag('main'), SETTLE * tempo());
      return () => window.clearTimeout(id);
    }
    setLag(phase);
    return undefined;
  }, [phase, lag, tempo]);
  const settled = lag === 'main';
  useEffect(() => onStage(!main), [main, onStage]);

  const offer = upd?.offer;
  const required = offer?.state === 'required';
  const noticed = main && !!offer && (required || upd?.postponedUntil === undefined);
  const updating = starting || !!upd?.status;
  const open = noticed && !required;
  const failed = noticed && upd?.failed === 'corrupt';
  const installing = noticed && upd?.status === 'installing';
  const leaving = failed || installing;
  const release = upd?.release ?? 'https://github.com/jaywehosl/qd/releases';
  const version = (upd?.version ?? '').replace(/^v/, '');

  const openRelease = useCallback(() => {
    const shell = window as unknown as { qdOpenURL?: (url: string) => void };
    if (shell.qdOpenURL) shell.qdOpenURL(release);
    else window.open(release, '_blank', 'noopener');
  }, [release]);

  const [cheer, setCheer] = useState(false);
  const [cheerKind, setCheerKind] = useState<'updated' | 'delayed'>('updated');
  const [delayed, setDelayed] = useState<number | null>(null);
  const [delayFor, setDelayFor] = useState(0);
  const fresh = upd?.updatedFrom ? `${upd.updatedFrom}>${upd.version}` : '';
  useEffect(() => {
    if (!fresh || cheered === fresh) return;
    cheered = fresh;
    setCheerKind('updated');
    setCheer(true);
  }, [fresh]);
  useEffect(() => {
    if (!cheer) return;
    const id = window.setTimeout(() => setCheer(false), 10000);
    return () => window.clearTimeout(id);
  }, [cheer]);
  useEffect(() => {
    if (delayed === null) return;
    const id = window.setTimeout(() => {
      setDelayed(null);
      setCheerKind('delayed');
      setCheer(true);
    }, 4000 * tempo());
    return () => window.clearTimeout(id);
  }, [delayed, tempo]);

  const justUpdated = !!fresh && cheered !== fresh;
  const headState = !main
    ? phase
    : cheer || justUpdated ? 'cheer' : failed ? 'manual' : noticed || delayed !== null ? 'service' : 'normal';
  const [slide, headEase] = useSlide(headState);

  const stamp = state.subscription?.lastRefresh ?? 0;
  const seen = useRef(stamp);
  const [pulse, setPulse] = useState(false);
  const shown = useRef(0);
  if (refreshing) shown.current = Number.POSITIVE_INFINITY;
  else if (shown.current === Number.POSITIVE_INFINITY) shown.current = Date.now() + 4000;
  useEffect(() => {
    if (stamp === seen.current) return;
    const first = seen.current === 0;
    seen.current = stamp;
    if (main && !first && Date.now() > shown.current) setPulse(true);
  }, [stamp, main]);
  useEffect(() => {
    if (refreshing) setPulse(true);
  }, [refreshing]);
  useEffect(() => {
    if (!pulse) return undefined;
    const id = window.setTimeout(() => setPulse(false), PULSE * tempo());
    return () => window.clearTimeout(id);
  }, [pulse, tempo]);

  const [at, setAt] = useState(state.connected ? 1 : 0);
  const [spread, setSpread] = useState(state.egress && state.connected ? 1 : 0);

  const aim = useRef(state.connected ? 1 : 0);
  const spreadAim = useRef(state.egress && state.connected ? 1 : 0);
  const held = useRef(false);

  const { data: nodes = [] } = useQuery({
    queryKey: ['client', 'nodes'],
    queryFn: fetchClientNodes,
    refetchInterval: 10000,
  });

  const { data: pace } = useQuery({
    queryKey: ['client', 'pace'],
    queryFn: async () => {
      const msg = await HttpUtil.get<{ points?: { down?: number; up?: number }[] }>(
        '/client/api/history/1', undefined, { silent: true },
      );
      const points = msg?.success ? msg.obj?.points ?? [] : [];
      return points[points.length - 1] ?? { down: 0, up: 0 };
    },
    refetchInterval: 2000,
    enabled: state.connected,
  });

  const exiting = state.egress && state.allowExit !== false;
  const road = state.connected && state.road ? state.road + (exiting ? '+H3' : '') : '';
  const lastNode = useRef('');
  if (state.node?.name) lastNode.current = state.node.name;

  useEffect(() => {
    aim.current = state.connected ? 1 : 0;
    if (state.connected) held.current = false;
  }, [state.connected]);

  useEffect(() => {
    spreadAim.current = exiting && state.connected ? 1 : 0;
  }, [exiting, state.connected]);

  useEffect(() => {
    let frame = 0;
    let last = 0;

    const tick = (now: number) => {
      frame = requestAnimationFrame(tick);
      const gap = last === 0 ? 0 : Math.min((now - last) / 1000, 0.25);
      last = now;
      if (gap === 0) return;

      setAt((prev) => {
        const goal = held.current ? GATE : aim.current;
        if (Math.abs(goal - prev) < 0.0005) return goal;
        const pace = prev < GATE ? GATE / RISE : (1 - GATE) / FINISH;
        const move = pace * gap;
        return goal > prev ? Math.min(goal, prev + move) : Math.max(goal, prev - move * 1.25);
      });

      setSpread((prev) => {
        const goal = spreadAim.current;
        if (prev === goal) return prev;
        const move = gap / SPLIT;
        return goal > prev ? Math.min(goal, prev + move) : Math.max(goal, prev - move);
      });
    };

    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, []);

  const [going, setGoing] = useState(false);
  const toggleTunnel = useCallback(async () => {
    setBusy(true);
    setGoing(state.connected);
    if (!state.connected) held.current = true;
    try {
      await (state.connected ? onDisconnect() : onConnect());
    } finally {
      held.current = false;
      setBusy(false);
      setGoing(false);
    }
  }, [state.connected, onConnect, onDisconnect]);

  const flipEgress = useCallback(async () => {
    const next = await onEgress(!state.egress);
    if (!next) toast.error(t('client.connect.exitRefused'));
  }, [onEgress, state.egress, t]);

  const startUpdate = useCallback(async () => {
    setStarting(true);
    const msg = await runUpdate();
    setStarting(false);
    if (!msg?.success) toast.error(msg?.msg || t('client.update.failed'));
  }, [runUpdate, t]);

  const putOff = useCallback(async () => {
    setDelayFor(period);
    setDelayed(period);
    const msg = await postpone(PERIODS[period].minutes);
    if (!msg?.success) {
      setDelayed(null);
      toast.error(msg?.msg || t('client.update.failed'));
    }
  }, [postpone, period, t]);

  const refresh = useCallback(async () => {
    await onRefresh();
  }, [onRefresh]);

  const startInstall = async () => {
    setTrouble('');
    setStage('installing');
    requestAnimationFrame(() => requestAnimationFrame(() => setRing(1)));
    const [msg] = await Promise.all([install({ autostart, desktop }), wait(RING)]);
    if (!msg?.success) {
      setRing(0);
      setStage('idle');
      setTrouble(msg?.msg || t('client.setup.failed'));
      return;
    }
    setStage('installed');
    await wait(BEAT);
    setRing(0);
    setStage('idle');
  };

  const submit = async (value: string) => {
    const link = value.trim();
    if (!link || phase !== 'import') return;
    setTrouble('');
    setStage('getting');
    const [next] = await Promise.all([onImport(link), wait(GETTING)]);
    if (next) {
      await onSettle();
      setUri('');
    } else {
      setTrouble(t('client.import.failed'));
    }
    setStage('idle');
  };
  const submitRef = useRef(submit);
  submitRef.current = submit;

  const adopt = (link: string) => {
    if (phase === 'import') {
      setUri(link);
      void submit(link);
      return;
    }
    void onImport(link).then((next) => next && onSettle());
  };
  const adoptRef = useRef(adopt);
  adoptRef.current = adopt;
  useEffect(() => {
    const take = () => {
      const link = phone()?.pending?.();
      if (link) adoptRef.current(link);
    };
    take();
    window.addEventListener(HOST_EVENT, take);
    return () => window.removeEventListener(HOST_EVENT, take);
  }, []);

  const fromClipboard = async () => {
    try {
      const host = phone();
      const text = (host?.clipboard ? host.clipboard() : await navigator.clipboard.readText()).trim();
      if (!text) return;
      setUri(text);
      void submit(text);
    } catch {
      setTrouble(t('client.import.clipboardDenied'));
    }
  };

  useEffect(() => {
    if (phase !== 'import') return undefined;
    const onPaste = (e: ClipboardEvent) => {
      const text = (e.clipboardData?.getData('text') ?? '').trim();
      if (!text) return;
      e.preventDefault();
      setUri(text);
      void submitRef.current(text);
    };
    window.addEventListener('paste', onPaste);
    return () => window.removeEventListener('paste', onPaste);
  }, [phase]);

  const mounted = useRef(false);
  useEffect(() => {
    const first = !mounted.current;
    mounted.current = true;
    if (phase !== 'import' || phone()) return undefined;
    const id = window.setTimeout(() => field.current?.focus({ preventScroll: true }), first ? 0 : SETTLE * tempo());
    return () => window.clearTimeout(id);
  }, [phase, tempo]);

  const lift = span(at, 0, GATE);
  const rosterAlpha = span(at, 0.58, 0.86) * dip(spread, 0.3, 0.8);
  const splitAlpha = span(at, 0.88, 1) * span(spread, 0.88, 1);
  const pairAlpha = span(at, 0.88, 1) * (1 - span(spread, 0, 0.15));
  const wide = spread > 0.55;

  const peaked = useRef(state.connected);
  if (at > GATE + 0.09) peaked.current = true;
  if (at <= 0.01) peaked.current = false;
  const parting = going || (!state.connected && peaked.current && at > 0.01);
  const tunnel = parting ? 'disconnecting' : at <= 0.01 ? 'ready' : at < GATE + 0.09 ? 'connecting' : 'node';
  const live = (refreshing || pulse) && (tunnel === 'ready' || tunnel === 'node') ? 'updating' : tunnel;
  const kept = useRef(live);
  if (headState === 'normal') kept.current = live;
  const tag = kept.current;
  const [slideTag, tagEase] = useSlide(tag);

  const word = tunnel === 'ready'
    ? t('client.connect.connect')
    : tunnel === 'node' ? t('client.connect.connected') : t(`client.connect.${tunnel}`);

  const face = settled ? 'tunnel' : lag === 'import' || lag === 'getting' ? 'import' : 'install';
  const [slideFace, faceEase] = useSlide(face);
  const [, flipEase] = useSlide(noticed ? 'update' : 'tunnel');
  const setting = phase === 'installing' || phase === 'installed';
  const slotOpen = phase === 'import' || phase === 'getting' || (open && !leaving);
  const noExit = noticed || !settled || state.allowExit === false;
  const veiled = noticed || delayed !== null || phase === 'wizard' || (phase === 'import' && !!trouble);

  const press = () => {
    if (phase === 'wizard') void startInstall();
    else if (phase === 'import') {
      if (uri.trim()) void submit(uri);
      else field.current?.focus({ preventScroll: true });
    } else if (noticed) void (failed ? openRelease() : startUpdate());
    else void toggleTunnel();
  };
  const idle = !settled || phase !== lag
    ? phase !== 'wizard' && phase !== 'import'
    : noticed ? !failed && updating : busy;

  return (
    <div className="cx" ref={root}>
      <section className="cx-card cx-head" style={headEase}>
        <div className={`cx-head__row${slide('normal')}`} aria-hidden={headState !== 'normal'}>
          <span className="cx-tag cx-reel" style={tagEase}>
            <span className={slideTag('ready')}>{t('client.connect.ready')}</span>
            <span className={slideTag('connecting')}>{t('client.connect.connecting')}</span>
            <span className={slideTag('node')}>{lastNode.current}</span>
            <span className={slideTag('disconnecting')}>{t('client.connect.disconnecting')}</span>
            <span className={slideTag('updating')}>{t('client.connect.updating')}</span>
          </span>
          <Roll text={tag === 'node' ? road : ''} tone={road.startsWith('H3') ? '' : ' is-spare'} />
          <button
            type="button"
            className="cx-ad"
            aria-label="adblock"
            aria-pressed={state.adblock}
            onClick={() => void onAdblock(!state.adblock)}
          >
            <Roll text="Adblock" tone={state.adblock ? '' : ' is-idle'} inline />
          </button>
          <button
            type="button"
            className={`cx-again${refreshing ? ' is-busy' : ''}`}
            aria-label={t('client.connect.refresh')}
            onClick={() => void refresh()}
          >
            <svg className="cx-again__icon" viewBox="0 0 24 24" aria-hidden="true">
              <path d="M15.05 16.95A7 7 0 1 1 17.1 12" />
              <path d="M12.34 10.46L16.4 14.1L20.9 10.04" />
            </svg>
          </button>
        </div>
        <div className={`cx-head__say${slide('wizard')}`} aria-hidden={headState !== 'wizard'}>
          {t('client.setup.wizardHead')}
        </div>
        <div className={`cx-head__say${slide('installing')}`} aria-hidden={headState !== 'installing'}>
          {t('client.setup.installingHead')}
        </div>
        <div className={`cx-head__say${slide('installed')}`} aria-hidden={headState !== 'installed'}>
          {t('client.setup.installedHead')}
        </div>
        <div className={`cx-head__say${slide('import')}`} aria-hidden={headState !== 'import'}>
          {t('client.import.head')}
        </div>
        <div className={`cx-head__say${slide('getting')}`} aria-hidden={headState !== 'getting'}>
          {t('client.import.gettingHead')}
        </div>
        <div className={`cx-head__say${slide('service')}`} aria-hidden={headState !== 'service'}>
          {t('client.update.serviceHead')}
        </div>
        <div className={`cx-head__say${slide('manual')}`} aria-hidden={headState !== 'manual'}>
          {t('client.update.manualHead')}
        </div>
        <div className={`cx-head__say${slide('cheer')}`} aria-hidden={headState !== 'cheer'}>
          {cheerKind === 'delayed' && !justUpdated
            ? (state.connected ? t('client.update.delayedOnline', worded()) : t('client.update.delayedHead'))
            : (state.connected ? t('client.update.doneConnected', worded()) : t('client.update.done'))}
        </div>
      </section>

      <section className={`cx-card cx-stage${veiled ? ' is-notice' : ''}`}>
        <div className="cx-stage__live">
          <FlowCanvas
            lift={lift}
            apart={span(spread, 0.3, 0.85)}
            fade={span(at, GATE - 0.21, GATE)}
            alive={at > GATE + 0.02}
          />

          <div
            className={`cx-roster${wide ? ' is-wide' : ''}`}
            style={{ opacity: rosterAlpha }}
          >
            {nodes.map((n) => (
              <div key={n.id} className="cx-node">
                <span className="cx-node__name">{n.name}</span>
                <span className={`cx-dot${n.reachable ? ' is-up' : ''}`} />
                <span className="cx-node__ping">
                  {n.reachable && n.latencyMs ? `${n.latencyMs}ms` : '—'}
                </span>
              </div>
            ))}
          </div>

          <div className="cx-pair" style={{ opacity: pairAlpha }}>
            <span className="cx-pill">{rate(pace?.down ?? 0)} ↓</span>
            <span className="cx-pill">↑ {rate(pace?.up ?? 0)}</span>
          </div>

          <div className="cx-single" style={{ opacity: splitAlpha }}>
            <span className="cx-pill cx-pill--wide">
              <span>{rate(pace?.down ?? 0)} ↓</span>
              <span>↑ {rate(pace?.up ?? 0)}</span>
            </span>
          </div>
        </div>

        <div
          className={`cx-notice cx-notice--wizard${phase === 'wizard' ? ' is-shown' : ''}`}
          aria-hidden={phase !== 'wizard'}
        >
          <span className="cx-notice__title">{t('client.setup.welcomeTitle', { version })}</span>
          <p className="cx-notice__text">{t('client.setup.welcomeText')}</p>
          {phase === 'wizard' && trouble && <p className="cx-notice__error">{trouble}</p>}
          <div className="cx-options">
            <label className="cx-option">
              <span>{t('client.setup.autostart')}</span>
              <Switch checked={autostart} disabled={phase !== 'wizard'} onChange={setAutostart} />
            </label>
            <label className="cx-option">
              <span>{t('client.setup.desktop')}</span>
              <Switch checked={desktop} disabled={phase !== 'wizard'} onChange={setDesktop} />
            </label>
          </div>
        </div>

        <div
          className={`cx-notice${phase === 'import' && trouble ? ' is-shown' : ''}`}
          aria-hidden={phase !== 'import' || !trouble}
        >
          <span className="cx-notice__title">{t('client.import.failedTitle')}</span>
          <p className="cx-notice__text">{trouble}</p>
        </div>

        <div className={`cx-notice${noticed && !leaving ? ' is-shown' : ''}`} aria-hidden={!noticed || leaving}>
          <span className="cx-notice__title">
            {required ? t('client.update.requiredTitle') : t('client.update.availableTitle')}
          </span>
          <p className="cx-notice__text">
            {t(required ? 'client.update.requiredText' : 'client.update.availableText')}
          </p>
          {upd?.error && <p className="cx-notice__error">{upd.error}</p>}
        </div>

        <div className={`cx-notice${installing ? ' is-shown' : ''}`} aria-hidden={!installing}>
          <span className="cx-notice__title">{t('client.update.installingTitle')}</span>
          <p className="cx-notice__text">{t('client.update.installingText')}</p>
        </div>

        <div className={`cx-notice${failed ? ' is-shown' : ''}`} aria-hidden={!failed}>
          <span className="cx-notice__title">{t('client.update.failedTitle')}</span>
          <p className="cx-notice__text">
            {t('client.update.failedText')}{' '}
            <a href={release} onClick={(e) => { e.preventDefault(); openRelease(); }}>{t('client.update.here')}</a>.
          </p>
        </div>

        <div className={`cx-notice${delayed !== null && !noticed ? ' is-shown' : ''}`} aria-hidden={delayed === null || noticed}>
          <span className="cx-notice__title">{t('client.update.delayedTitle')}</span>
          <p className="cx-notice__text">
            {PERIODS[delayFor].when
              ? t('client.update.delayedText', { when: t(PERIODS[delayFor].when) })
              : t('client.update.delayedNever')}
          </p>
        </div>
      </section>

      <div className="cx-duo">
        <div className={`cx-slot${slotOpen ? ' is-open' : ''}${leaving ? ' is-leaving' : ''}`}>
          <div className="cx-slot__inner">
            <section className="cx-card cx-controls up-shell">
              {settled ? (
                <div className="cx-power is-later">
                  <button
                    type="button"
                    className="cx-power__main"
                    disabled={!noticed || updating}
                    tabIndex={noticed ? 0 : -1}
                    onClick={() => void putOff()}
                  >
                    {t('client.update.later')}
                  </button>
                  <button
                    type="button"
                    className="cx-exit cx-period"
                    aria-label={t('client.update.period')}
                    disabled={!noticed || updating}
                    tabIndex={noticed ? 0 : -1}
                    onClick={() => {
                      if (spot === PERIODS.length) return;
                      setSnap(false);
                      setSpot(spot + 1);
                    }}
                  >
                    <span
                      className={`cx-period__strip${snap ? ' is-snap' : ''}`}
                      style={{ transform: `translateX(${-spot * 60}px)` }}
                      onTransitionEnd={() => {
                        if (spot !== PERIODS.length) return;
                        setSnap(true);
                        setSpot(0);
                      }}
                    >
                      {[...PERIODS, PERIODS[0]].map((p, i) => (
                        <span key={i} className="cx-period__face">{p.label}</span>
                      ))}
                    </span>
                  </button>
                </div>
              ) : (
                <div className="cx-power is-field">
                  <input
                    ref={field}
                    className="cx-uri"
                    value={uri}
                    placeholder="qd://…"
                    spellCheck={false}
                    inputMode="url"
                    autoCapitalize="none"
                    autoCorrect="off"
                    autoComplete="off"
                    enterKeyHint="go"
                    disabled={phase !== 'import'}
                    tabIndex={phase === 'import' ? 0 : -1}
                    onChange={(e) => { setUri(e.target.value); setTrouble(''); }}
                    onKeyDown={(e) => { if (e.key === 'Enter') void submit(uri); }}
                  />
                  <button
                    type="button"
                    className="cx-exit cx-paste"
                    aria-label={t('client.import.fromClipboard')}
                    title={t('client.import.fromClipboard')}
                    disabled={phase !== 'import'}
                    tabIndex={phase === 'import' ? 0 : -1}
                    onClick={() => void fromClipboard()}
                  >
                    <SnippetsOutlined />
                  </button>
                </div>
              )}
            </section>
          </div>
        </div>

        <UpdateShell
          progress={setting ? ring : progressOf(upd) ?? 0}
          busy={setting || (noticed && !leaving && updating)}
          sweep={setting ? RING * tempo() : undefined}
        >
        <div className={`cx-power${!settled || (noticed && upd?.status) ? ' is-on' : at > GATE ? ' is-on' : at > 0.12 ? ' is-working' : ''}`}>
          <button
            type="button"
            className="cx-power__main"
            disabled={idle}
            onClick={press}
          >
            <span className={`cx-flip${noticed ? ' is-flipped' : ''}`} style={flipEase}>
              <span className="cx-flip__from cx-reel" style={faceEase} aria-hidden={noticed}>
                <span className={slideFace('install')}>{t('client.setup.action')}</span>
                <span className={slideFace('import')}>{t('client.import.action')}</span>
                <span className={slideFace('tunnel')}>{word}</span>
              </span>
              <span className="cx-flip__to" aria-hidden={!noticed}>
                {failed
                  ? t('client.update.manual')
                  : updateWord(t, upd, starting ? t('client.update.checking') : t('client.update.update'))}
              </span>
            </span>
          </button>
          <button
            type="button"
            className={`cx-exit${exiting ? ' is-on' : ''}${noExit ? ' is-gone' : ''}`}
            aria-label="+egress"
            tabIndex={noExit ? -1 : 0}
            onClick={() => void flipEgress()}
          >
            <svg viewBox="0 0 60 60" aria-hidden="true">
              <g className="cx-exit__face cx-exit__face--off">
                <path d="M11.9 17.9 L16 15.9 L20 18.2 L24 16.1 L28 17.6" />
                <path d="M32 39.9 L36 39.9 M39 39.9 L43 39.9" className="cx-exit__dash" />
                <path d="M30.7 11.8 L30.7 50.7" className="cx-exit__rail" />
              </g>
              <g className="cx-exit__face cx-exit__face--on">
                <path d="M12 17.7 L15.3 15.6 L18.6 17.4 L21.4 16.9" />
                <path d="M21.4 33.3 L26 31.5 L30.6 33.6 L35.2 31.2 L39.7 33" />
                <path d="M41 38.7 L44 38.7 M46.5 38.7 L48.1 38.7" className="cx-exit__dash" />
                <path d="M21.4 11.8 L21.4 48.9" className="cx-exit__rail" />
                <path d="M39.7 11.8 L39.7 48.9" className="cx-exit__rail" />
              </g>
            </svg>
          </button>
        </div>
        </UpdateShell>
      </div>
    </div>
  );
}
