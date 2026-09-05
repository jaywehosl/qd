import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { Switch, toast } from '@/components/ds';
import { HttpUtil } from '@/utils';
import { fetchClientNodes, type ClientState } from '@/hooks/useClientState';
import { bitsPerSec } from '@/lib/rate';
import FlowCanvas from './FlowCanvas';
import '@/skin/connect.css';

const GATE = 0.45;
const RISE = 1.3;
const FINISH = 0.8;
const SPLIT = 1.6;

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

  const refresh = useCallback(async () => {
    const result = await onRefresh();
    if (result) toast.success(t('client.connect.refreshed'));
  }, [onRefresh, t]);

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
    <div className="cx">
      <section className="cx-card cx-head">
        <span className="cx-tag">{state.node?.name ?? t('client.connect.idle')}</span>
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
      </section>

      <section className="cx-card cx-stage">
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
      </section>

      <section className="cx-card cx-controls">
        <div className={`cx-power${at > GATE ? ' is-on' : at > 0.12 ? ' is-working' : ''}`}>
          <button
            type="button"
            className="cx-power__main"
            disabled={busy}
            onClick={() => void toggleTunnel()}
          >
            {word}
          </button>
          {state.allowExit !== false && (
            <button
              type="button"
              className={`cx-exit${exiting ? ' is-on' : ''}`}
              aria-label="+egress"
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

      </section>
    </div>
  );
}
