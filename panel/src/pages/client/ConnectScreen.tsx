import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { Switch, toast } from '@/components/ds';
import { HttpUtil } from '@/utils';
import { fetchClientNodes, type ClientState } from '@/hooks/useClientState';
import { bitsPerSec } from '@/lib/rate';
import { progressOf, useUpdate } from '@/hooks/useUpdate';
import FlowCanvas from './FlowCanvas';
import UpdateShell from './UpdateShell';
import { updateWord } from './UpdateButton';
import '@/skin/connect.css';

if (import.meta.env.DEV) void import('@/skin/connect-slow.css');

const GATE = 0.45;
const RISE = 1.3;
const FINISH = 0.8;
const SPLIT = 1.6;

const PERIODS = [
  { minutes: 30, label: '30m', when: 'client.update.when30m' },
  { minutes: 720, label: '12h', when: 'client.update.when12h' },
  { minutes: 1440, label: '1d', when: 'client.update.when1d' },
  { minutes: -1, label: '∞', when: '' },
];

interface ConnectScreenProps {
  state: ClientState;
  onConnect: () => Promise<ClientState | null>;
  onDisconnect: () => Promise<ClientState | null>;
  onEgress: (v: boolean) => Promise<ClientState | null>;
  onAdblock: (v: boolean) => Promise<ClientState | null>;
  onRefresh: () => Promise<unknown>;
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

const rate = bitsPerSec;

function countdown(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (v: number) => String(v).padStart(2, '0');
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

export default function ConnectScreen({
  state, onConnect, onDisconnect, onEgress, onAdblock, onRefresh, refreshing,
}: ConnectScreenProps) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const { info: upd, run: runUpdate, postpone } = useUpdate();
  const [spot, setSpot] = useState(0);
  const [snap, setSnap] = useState(false);
  const period = spot % PERIODS.length;
  const [starting, setStarting] = useState(false);
  const offer = upd?.offer;
  const required = offer?.state === 'required';
  const noticed = !!offer && (required || upd?.postponedUntil === undefined);
  const updating = starting || !!upd?.status;
  const open = noticed && !required;
  const failed = noticed && upd?.failed === 'corrupt';
  const installing = noticed && upd?.status === 'installing';
  const leaving = failed || installing;
  const release = upd?.release ?? 'https://github.com/jaywehosl/qd/releases';

  const openRelease = useCallback(() => {
    const shell = window as unknown as { qdOpenURL?: (url: string) => void };
    if (shell.qdOpenURL) shell.qdOpenURL(release);
    else window.open(release, '_blank', 'noopener');
  }, [release]);

  const [cheer, setCheer] = useState(false);
  const [spent, setSpent] = useState(false);
  const [cheerKind, setCheerKind] = useState<'updated' | 'delayed'>('updated');
  const [delayed, setDelayed] = useState<number | null>(null);
  const [delayFor, setDelayFor] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!upd?.updatedFrom) {
      setSpent(false);
      return;
    }
    setCheerKind('updated');
    setCheer(true);
  }, [upd?.updatedFrom]);
  useEffect(() => {
    if (!cheer) return;
    const id = window.setTimeout(() => {
      setCheer(false);
      setSpent(true);
    }, 10000);
    return () => window.clearTimeout(id);
  }, [cheer]);
  useEffect(() => {
    if (delayed === null) return;
    const pace = root.current ? parseFloat(getComputedStyle(root.current).getPropertyValue('--cx-t')) || 1 : 1;
    const id = window.setTimeout(() => {
      setDelayed(null);
      setCheerKind('delayed');
      setCheer(true);
    }, 4000 * pace);
    return () => window.clearTimeout(id);
  }, [delayed]);

  const justUpdated = !!upd?.updatedFrom && !spent;
  const headState = cheer || justUpdated ? 'cheer' : failed ? 'manual' : noticed || delayed !== null ? 'service' : 'normal';
  const headTrack = useRef({ now: headState, was: headState });
  if (headTrack.current.now !== headState) headTrack.current = { now: headState, was: headTrack.current.now };
  const headWas = headTrack.current.was;
  const slide = (name: string) => {
    if (name === headState) return headWas === headState ? ' is-rest' : ' is-on';
    return name === headWas ? ' is-off' : '';
  };

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

  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const tick = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(tick);
  }, []);

  const untilRefresh = useMemo(() => {
    const last = state.subscription?.lastRefresh ?? 0;
    const every = (state.subscription?.intervalMinutes ?? 0) * 60_000;
    if (!last || !every) return null;
    return Math.max(0, last + every - now);
  }, [state.subscription, now]);

  const exiting = state.egress && state.allowExit !== false;

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

  const toggleTunnel = useCallback(async () => {
    setBusy(true);
    if (!state.connected) held.current = true;
    try {
      await (state.connected ? onDisconnect() : onConnect());
    } finally {
      held.current = false;
      setBusy(false);
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

  const lift = span(at, 0, GATE);
  const rosterAlpha = span(at, 0.58, 0.86) * dip(spread, 0.3, 0.8);
  const splitAlpha = span(at, 0.88, 1) * span(spread, 0.88, 1);
  const pairAlpha = span(at, 0.88, 1) * (1 - span(spread, 0, 0.15));
  const wide = spread > 0.55;

  const word = at <= 0.01
    ? t('client.connect.connect')
    : at < GATE + 0.09
      ? t('client.connect.connecting')
      : aim.current < at
        ? t('client.connect.disconnecting')
        : t('client.connect.connected');

  return (
    <div className="cx" ref={root}>
      <section className="cx-card cx-head">
        <div className={`cx-head__row${slide('normal')}`} aria-hidden={headState !== 'normal'}>
          <span className="cx-tag">{state.node?.name ?? ''}</span>
          <span className="cx-refresh-line">
            {untilRefresh !== null && (
              <>
                <span className="cx-refresh-line__label">{t('client.connect.nextRefresh')}</span>
                <span className="cx-refresh-line__value">{countdown(untilRefresh)}</span>
              </>
            )}
          </span>
          <div className="cx-switch">
            <Switch
              id="cx-adblock"
              checked={state.adblock}
              onChange={(v) => void onAdblock(v)}
              aria-label="+adblock"
            />
            <label htmlFor="cx-adblock">+adblock</label>
          </div>
          <button
            type="button"
            className={`cx-again${refreshing ? ' is-busy' : ''}`}
            aria-label={t('client.connect.refresh')}
            onClick={() => void refresh()}
          >
            ⟳
          </button>
        </div>
        <div className={`cx-head__say${slide('service')}`} aria-hidden={headState !== 'service'}>
          {t('client.update.serviceHead')}
        </div>
        <div className={`cx-head__say${slide('manual')}`} aria-hidden={headState !== 'manual'}>
          {t('client.update.manualHead')}
        </div>
        <div className={`cx-head__say${slide('cheer')}`} aria-hidden={headState !== 'cheer'}>
          {cheerKind === 'delayed' && !justUpdated
            ? (state.connected ? t('client.update.delayedOnline') : t('client.update.delayedHead'))
            : (state.connected ? t('client.update.doneConnected') : t('client.update.done'))}
        </div>
      </section>

      <section className={`cx-card cx-stage${noticed || delayed !== null ? ' is-notice' : ''}`}>
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
        <div className={`cx-slot${open && !leaving ? ' is-open' : ''}${leaving ? ' is-leaving' : ''}`}>
          <div className="cx-slot__inner">
            <section className="cx-card cx-controls up-shell">
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
            </section>
          </div>
        </div>

        <UpdateShell progress={progressOf(upd) ?? 0} busy={noticed && !leaving && updating}>
        <div className={`cx-power${noticed && upd?.status ? ' is-on' : at > GATE ? ' is-on' : at > 0.12 ? ' is-working' : ''}`}>
          <button
            type="button"
            className="cx-power__main"
            disabled={noticed ? !failed && updating : busy}
            onClick={() => void (noticed ? (failed ? openRelease() : startUpdate()) : toggleTunnel())}
          >
            <span className={`cx-flip${noticed ? ' is-flipped' : ''}`}>
              <span className="cx-flip__from" aria-hidden={noticed}>{word}</span>
              <span className="cx-flip__to" aria-hidden={!noticed}>
                {failed
                  ? t('client.update.manual')
                  : updateWord(t, upd, starting ? t('client.update.checking') : t('client.update.update'))}
              </span>
            </span>
          </button>
          {state.allowExit !== false && (
            <button
              type="button"
              className={`cx-exit${exiting ? ' is-on' : ''}${noticed ? ' is-gone' : ''}`}
              aria-label="+egress"
              tabIndex={noticed ? -1 : 0}
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
          )}
        </div>
        </UpdateShell>
      </div>
    </div>
  );
}
