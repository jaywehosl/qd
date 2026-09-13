import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Select, Tabs } from '@/components/ds';
import {
  ApiOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  DownloadOutlined,
  GlobalOutlined,
  HddOutlined,
  LineChartOutlined,
  PieChartOutlined,
  SyncOutlined,
  TeamOutlined,
} from '@ant-design/icons';

import { FileManager, HttpUtil, SizeFormatter } from '@/utils';
import { Sparkline } from '@/components/viz';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import NodeSelect from './NodeSelect';
import LogView from './LogView';
import type { Status } from '@/models/status';
interface SystemHistoryModalProps {
  status: Status;
}

interface MetricDef {
  key: string;
  tab: string;
  tabKey?: string;
  title: string;
  icon: ReactNode;
  valueMax: number | null;
  unit: string;
  stroke: string;
  key2?: string;
  stroke2?: string;
  name1?: string;
  name2?: string;
  key3?: string;
  stroke3?: string;
  name3?: string;
}

const METRICS: MetricDef[] = [
  { key: 'cpu', tab: 'CPU', tabKey: 'pages.index.cpu', title: 'pages.index.historyTitleCpu', icon: <DashboardOutlined />, valueMax: 100, unit: '%', stroke: '' },
  { key: 'mem', tab: 'RAM', tabKey: 'pages.index.memory', title: 'pages.index.historyTitleMem', icon: <DatabaseOutlined />, valueMax: 100, unit: '%', stroke: '#7c4dff', key2: 'swap', stroke2: '#ffa940', name1: 'pages.index.memory', name2: 'pages.index.swap' },
  { key: 'netUp', tab: 'Bandwidth', tabKey: 'pages.index.historyTabBandwidth', title: 'pages.index.historyTitleNetwork', icon: <GlobalOutlined />, valueMax: null, unit: 'B/s', stroke: '#1890ff', key2: 'netDown', stroke2: '#13c2c2', name1: 'Up', name2: 'Down' },
  { key: 'pktUp', tab: 'Packets', tabKey: 'pages.index.historyTabPackets', title: 'pages.index.historyTitlePackets', icon: <DeploymentUnitOutlined />, valueMax: null, unit: 'pkt/s', stroke: '#2f54eb', key2: 'pktDown', stroke2: '#36cfc9', name1: 'Up', name2: 'Down' },
  { key: 'tcpCount', tab: 'Connections', tabKey: 'pages.index.historyTabConnections', title: 'pages.index.historyTitleConnections', icon: <ApiOutlined />, valueMax: null, unit: '', stroke: '#597ef7', key2: 'udpCount', stroke2: '#73d13d', name1: 'TCP', name2: 'UDP' },
  { key: 'diskRead', tab: 'Disk I/O', tabKey: 'pages.index.historyTabDisk', title: 'pages.index.historyTitleDisk', icon: <HddOutlined />, valueMax: null, unit: 'B/s', stroke: '#eb2f96', key2: 'diskWrite', stroke2: '#722ed1', name1: 'Read', name2: 'Write' },
  { key: 'diskUsage', tab: 'Disk Usage', tabKey: 'pages.index.historyTabDiskUsage', title: 'pages.index.historyTitleDiskUsage', icon: <PieChartOutlined />, valueMax: 100, unit: '%', stroke: '#13c2c2' },
  { key: 'online', tab: 'Online', tabKey: 'pages.index.historyTabOnline', title: 'pages.index.historyTitleOnline', icon: <TeamOutlined />, valueMax: null, unit: '', stroke: '#52c41a' },
  { key: 'load1', tab: 'Load', tabKey: 'pages.index.historyTabLoad', title: 'pages.index.historyTitleLoad', icon: <LineChartOutlined />, valueMax: null, unit: '', stroke: '#fa8c16', key2: 'load5', stroke2: '#f5222d', name1: '1m', name2: '5m', key3: 'load15', stroke3: '#a0d911', name3: '15m' },
];

function unitFormatter(unit: string, activeKey: string): (v: number) => string {
  if (unit === 'B/s') {
    return (v) => `${SizeFormatter.sizeFormat(Math.max(0, Number(v) || 0)).replace(/\.\d+/, '')}/s`;
  }
  if (unit === 'pkt/s') {
    return (v) => `${Math.round(Math.max(0, Number(v) || 0)).toLocaleString()}/s`;
  }
  if (unit === '%') {
    return (v) => `${Number(v).toFixed(1)}%`;
  }
  return (v) => {
    const n = Number(v) || 0;
    if (activeKey === 'online' || activeKey === 'tcpCount' || activeKey === 'udpCount') {
      return Math.round(n).toLocaleString();
    }
    return n.toFixed(2);
  };
}

const LOG_ROWS = [10, 20, 50, 100];

const WINDOWS: { minutes: number; label: string }[] = [
  { minutes: 1, label: '1m' },
  { minutes: 5, label: '5m' },
  { minutes: 15, label: '15m' },
  { minutes: 30, label: '30m' },
  { minutes: 60, label: '1h' },
];

const CHART_POINTS = 180;

function bucketFor(minutes: number): number {
  return Math.max(1, Math.ceil((minutes * 60) / CHART_POINTS));
}

function formatFullTimestamp(unixSec: number): string {
  const d = new Date(unixSec * 1000);
  const today = new Date();
  const sameDay = d.getFullYear() === today.getFullYear()
    && d.getMonth() === today.getMonth()
    && d.getDate() === today.getDate();
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const ss = String(d.getSeconds()).padStart(2, '0');
  const time = `${hh}:${mm}:${ss}`;
  if (sameDay) return time;
  const MM = String(d.getMonth() + 1).padStart(2, '0');
  const DD = String(d.getDate()).padStart(2, '0');
  return `${MM}-${DD} ${time}`;
}

export default function SystemHistoryPanel({ status }: SystemHistoryModalProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const { nodes } = useNodesQuery();
  const [nodeId, setNodeId] = useState<number | null>(null);
  const [mode, setMode] = useState<'charts' | 'logs'>('charts');
  const [activeKey, setActiveKey] = useState('cpu');
  const [span, setSpan] = useState(5);
  const [refreshing, setRefreshing] = useState(false);
  const [logRows, setLogRows] = useState(20);
  const [logLevel, setLogLevel] = useState('info');
  const [logLines, setLogLines] = useState<string[]>([]);
  const [points, setPoints] = useState<number[]>([]);
  const [points2, setPoints2] = useState<number[]>([]);
  const [points3, setPoints3] = useState<number[]>([]);
  const [labels, setLabels] = useState<string[]>([]);
  const [timestamps, setTimestamps] = useState<number[]>([]);
  const asked = useRef(0);

  const activeMetric = useMemo(() => METRICS.find((m) => m.key === activeKey), [activeKey]);
  const trName = (n?: string) => (n && n.startsWith('pages.') ? t(n) : n);
  const strokeColor = activeMetric?.stroke || status?.cpu?.color || '#008771';
  const yFormatter = useMemo(
    () => unitFormatter(activeMetric?.unit ?? '', activeKey),
    [activeMetric, activeKey],
  );

  const tsLookup = useMemo(() => {
    const m = new Map<string, number>();
    for (let i = 0; i < labels.length; i++) {
      m.set(labels[i], timestamps[i]);
    }
    return m;
  }, [labels, timestamps]);

  const tooltipLabelFormatter = useCallback(
    (label: string) => {
      const ts = tsLookup.get(label);
      return ts ? formatFullTimestamp(ts) : label;
    },
    [tsLookup],
  );

  const fetchBucket = useCallback(async () => {
    if (!activeMetric || nodeId === null) return;
    const ticket = ++asked.current;
    const mine = () => ticket === asked.current;
    const seconds = span * 60;
    const bucket = bucketFor(span);
    const since = Math.floor(Date.now() / 1000) - seconds;
    try {
      const url = `/panel/api/nodes/${nodeId}/history/${activeMetric.key}/${bucket}?window=${seconds}`;
      const msg = await HttpUtil.get(url);
      if (!mine()) return;
      if (msg?.success && Array.isArray(msg.obj)) {
        const vals: number[] = [];
        const labs: string[] = [];
        const tss: number[] = [];
        for (const p of msg.obj) {
          const at = Number(p.t) || 0;
          if (at < since) continue;
          const d = new Date(at * 1000);
          const hh = String(d.getHours()).padStart(2, '0');
          const mm = String(d.getMinutes()).padStart(2, '0');
          const ss = String(d.getSeconds()).padStart(2, '0');
          labs.push(span >= 60 ? `${hh}:${mm}` : `${hh}:${mm}:${ss}`);
          vals.push(Number(p.v) || 0);
          tss.push(at);
        }
        setLabels(labs);
        setPoints(vals);
        setTimestamps(tss);

        const fetchAligned = async (key?: string): Promise<number[]> => {
          if (!key) return [];
          const m = await HttpUtil.get(
            `/panel/api/nodes/${nodeId}/history/${key}/${bucket}?window=${seconds}`,
          );
          if (m?.success && Array.isArray(m.obj)) {
            const byTs = new Map<number, number>();
            for (const p of m.obj) byTs.set(Number(p.t) || 0, Number(p.v) || 0);
            return tss.map((ts) => byTs.get(ts) ?? 0);
          }
          return [];
        };
        const second = await fetchAligned(activeMetric.key2);
        const third = await fetchAligned(activeMetric.key3);
        if (!mine()) return;
        setPoints2(second);
        setPoints3(third);
      } else {
        setLabels([]);
        setPoints([]);
        setPoints2([]);
        setPoints3([]);
        setTimestamps([]);
      }
    } catch (e) {
      console.error('Failed to fetch history bucket', e);
      if (!mine()) return;
      setLabels([]);
      setPoints([]);
      setPoints2([]);
      setPoints3([]);
      setTimestamps([]);
    }
  }, [activeMetric, span, nodeId]);

  useEffect(() => {
    if (nodeId === null && nodes.length > 0) setNodeId(nodes[0].id);
  }, [nodes, nodeId]);

  useEffect(() => {
    if (mode === 'charts') fetchBucket();
  }, [mode, activeKey, span, fetchBucket]);

  useEffect(() => {
    if (mode !== 'charts') return undefined;
    const ms = span <= 5 ? 2000 : 10000;
    const id = window.setInterval(() => fetchBucket(), ms);
    return () => window.clearInterval(id);
  }, [mode, span, fetchBucket]);

  const fetchLogs = useCallback(async () => {
    if (nodeId === null) return;
    const msg = await HttpUtil.post<string[]>(`/panel/api/nodes/${nodeId}/logs/${logRows}`, { level: logLevel });
    if (msg?.success) setLogLines(msg.obj || []);
  }, [nodeId, logRows, logLevel]);

  useEffect(() => {
    if (mode === 'logs') fetchLogs();
  }, [mode, fetchLogs]);

  const clearLevel = useCallback(async () => {
    if (nodeId === null) return;
    const msg = await HttpUtil.post(`/panel/api/nodes/${nodeId}/logs/clear`, { level: logLevel });
    if (msg?.success) await fetchLogs();
  }, [nodeId, logLevel, fetchLogs]);

  const downloadLog = useCallback(async () => {
    if (nodeId === null) return;
    const msg = await HttpUtil.get<string>(`/panel/api/nodes/${nodeId}/logs/download`);
    if (msg?.success && typeof msg.obj === 'string') {
      const name = nodes.find((n) => n.id === nodeId)?.name || String(nodeId);
      FileManager.downloadTextFile(msg.obj, `${name}.log`);
    }
  }, [nodeId, nodes]);

  const manualRefresh = useCallback(async () => {
    setRefreshing(true);
    try { await fetchBucket(); } finally { setRefreshing(false); }
  }, [fetchBucket]);

  async function downloadRaw() {
    if (nodeId === null) return;
    const msg = await HttpUtil.get<unknown>(`/panel/api/nodes/${nodeId}/history/export`, { window: span * 60 });
    if (!msg?.success) return;
    const name = nodes.find((n) => n.id === nodeId)?.name || String(nodeId);
    FileManager.downloadTextFile(JSON.stringify(msg.obj, null, 2), `${name}-history.json`);
  }

  const chart = (
    <div className="chp-chart">
      <Sparkline
        key={`${nodeId}-${activeKey}-${span}`}
        data={points}
        data2={activeMetric?.key2 ? points2 : undefined}
        data3={activeMetric?.key3 ? points3 : undefined}
        stroke2={activeMetric?.stroke2}
        stroke3={activeMetric?.stroke3}
        name1={trName(activeMetric?.name1)}
        name2={trName(activeMetric?.name2)}
        name3={trName(activeMetric?.name3)}
        labels={labels}
        height="100%"
        stroke={strokeColor}
        strokeWidth={2.2}
        showGrid
        showAxes
        tickCountX={5}
        maxPoints={points.length || 1}
        fillOpacity={0.18}
        markerRadius={3.2}
        showTooltip
        valueMin={0}
        valueMax={activeMetric?.valueMax ?? null}
        yFormatter={yFormatter}
        tooltipLabelFormatter={tooltipLabelFormatter}
        extrema={{ show: !activeMetric?.key2, formatter: yFormatter }}
      />
    </div>
  );

  return (
    <div className="chp">
      <div className="chp-head">
        <NodeSelect
          nodes={nodes}
          value={nodeId}
          onChange={setNodeId}
          empty={t('pages.index.noNodes')}
        />
        <div className="vertical-tabs-container chp-mode">
          <button
            type="button"
            className={`vtab-btn${mode === 'charts' ? ' is-active' : ''}`}
            onClick={() => setMode('charts')}
          >
            {t('pages.index.systemHistoryTitle')}
          </button>
          <button
            type="button"
            className={`vtab-btn${mode === 'logs' ? ' is-active' : ''}`}
            onClick={() => setMode('logs')}
          >
            {t('pages.index.logs')}
          </button>
        </div>

        <div className="chp-head__actions" key={mode}>
          {mode === 'charts' ? (
            <>
              <Button size="sm" icon={<DownloadOutlined />} disabled={nodeId === null} onClick={downloadRaw}>
                {t('pages.index.downloadRawHistory')}
              </Button>
              <Button size="sm" variant="text" onClick={manualRefresh}>
                <SyncOutlined spin={refreshing} />
              </Button>
            </>
          ) : (
            <>
              <Select
                value={logLevel}
                onChange={setLogLevel}
                options={[
                  { value: 'debug', label: 'All' },
                  { value: 'info', label: 'Info' },
                  { value: 'notice', label: 'Notice' },
                  { value: 'warning', label: 'Warning' },
                  { value: 'err', label: 'Error' },
                ]}
              />
              <Button size="sm" danger disabled={nodeId === null} onClick={clearLevel}>
                {t('pages.index.clearLogs', { defaultValue: 'Clear this level' })}
              </Button>
              <Button size="sm" icon={<DownloadOutlined />} disabled={nodeId === null} onClick={downloadLog}>
                {t('pages.index.downloadFullLog')}
              </Button>
              <Button size="sm" variant="text" onClick={fetchLogs}>
                <SyncOutlined />
              </Button>
            </>
          )}
        </div>
      </div>

      {mode === 'charts' ? (
        <Tabs
          activeKey={activeKey}
          onChange={setActiveKey}
          className="chp-tabs"
          items={METRICS.map((m) => {
            const tabLabel = m.tabKey ? t(m.tabKey) : m.tab;
            return {
              key: m.key,
              label: isMobile ? <span title={tabLabel} aria-label={tabLabel}>{m.icon}</span> : tabLabel,
              children: chart,
            };
          })}
        />
      ) : (
        <LogView lines={logLines} isMobile={isMobile} empty={t('pages.index.noRecord', { defaultValue: 'No Record...' })} />
      )}

      <div className="chp-window">
        {mode === 'charts'
          ? WINDOWS.map((b) => (
            <button
              key={b.minutes}
              type="button"
              className={`chp-win ${b.minutes === span ? 'is-active' : ''}`}
              onClick={() => setSpan(b.minutes)}
            >
              {b.label}
            </button>
          ))
          : LOG_ROWS.map((n) => (
            <button
              key={n}
              type="button"
              className={`chp-win ${n === logRows ? 'is-active' : ''}`}
              onClick={() => setLogRows(n)}
            >
              {n}
            </button>
          ))}
      </div>
    </div>
  );
}
