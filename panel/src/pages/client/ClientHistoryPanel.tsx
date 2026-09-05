import { useCallback, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  ApiOutlined,
  CloudUploadOutlined,
  DeploymentUnitOutlined,
  GlobalOutlined,
  SearchOutlined,
  StopOutlined,
} from '@ant-design/icons';

import { Tabs } from '@/components/ds';
import { Sparkline } from '@/components/viz';
import { HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { bitsPerSec } from '@/lib/rate';
interface HistPoint {
  t: number;
  [key: string]: number;
}

type Unit = 'bits' | 'rate' | 'count';

interface MetricDef {
  key: string;
  label: string;
  icon: ReactNode;
  unit: Unit;
  series: { field: string; name: string; stroke: string }[];
}

// One tab per line the tunnel already prints, split by where the answer lies:
// 'link' is the network between here and the node, 'sending' is this machine
// failing to push. Reading them the other way round sends you fixing the
// wrong end.
const METRICS: MetricDef[] = [
  {
    key: 'throughput', label: 'client.history.tab.throughput', icon: <GlobalOutlined />, unit: 'bits',
    series: [
      { field: 'down', name: 'client.history.down', stroke: '#13c2c2' },
      { field: 'up', name: 'client.history.up', stroke: '#1890ff' },
    ],
  },
  {
    key: 'packets', label: 'client.history.tab.packets', icon: <DeploymentUnitOutlined />, unit: 'rate',
    series: [
      { field: 'pktIn', name: 'client.history.in', stroke: '#36cfc9' },
      { field: 'pktOut', name: 'client.history.out', stroke: '#2f54eb' },
    ],
  },
  {
    key: 'link', label: 'client.history.tab.link', icon: <ApiOutlined />, unit: 'rate',
    series: [
      { field: 'lost', name: 'client.history.lost', stroke: '#f5222d' },
      { field: 'drops', name: 'client.history.drops', stroke: '#fa8c16' },
      { field: 'reorder', name: 'client.history.reorder', stroke: '#faad14' },
    ],
  },
  {
    key: 'sending', label: 'client.history.tab.sending', icon: <CloudUploadOutlined />, unit: 'rate',
    series: [
      { field: 'retries', name: 'client.history.retries', stroke: '#fa8c16' },
      { field: 'sendDrop', name: 'client.history.gaveUp', stroke: '#f5222d' },
      { field: 'sendErr', name: 'client.history.sendErr', stroke: '#eb2f96' },
    ],
  },
  {
    key: 'dns', label: 'client.history.tab.dns', icon: <SearchOutlined />, unit: 'rate',
    series: [
      { field: 'dnsQueries', name: 'client.history.queries', stroke: '#597ef7' },
      { field: 'dnsCached', name: 'client.history.cached', stroke: '#52c41a' },
      { field: 'dnsUpstream', name: 'client.history.upstream', stroke: '#722ed1' },
    ],
  },
  {
    key: 'adblock', label: 'client.history.tab.adblock', icon: <StopOutlined />, unit: 'rate',
    series: [
      { field: 'adblock', name: 'client.history.blocked', stroke: '#52c41a' },
    ],
  },
];

const WINDOWS = [1, 5, 15, 60];

function formatter(unit: Unit): (v: number) => string {
  if (unit === 'bits') return bitsPerSec;
  if (unit === 'rate') {
    return (v) => `${Math.max(0, v).toFixed(v < 10 ? 1 : 0)}/s`;
  }
  return (v) => Math.round(Math.max(0, v)).toLocaleString();
}

function clockLabel(unixSec: number, window: number): string {
  const d = new Date(unixSec * 1000);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  if (window >= 60) return `${hh}:${mm}`;
  return `${hh}:${mm}:${String(d.getSeconds()).padStart(2, '0')}`;
}

export default function ClientHistoryPanel() {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [activeKey, setActiveKey] = useState('throughput');
  const [window_, setWindow] = useState(5);

  const { data: points = [] } = useQuery<HistPoint[]>({
    queryKey: ['client', 'history', window_],
    queryFn: async () => {
      const msg = await HttpUtil.get<{ points?: HistPoint[] }>(
        `/client/api/history/${window_}`, undefined, { silent: true },
      );
      return msg?.success ? (msg.obj?.points ?? []) : [];
    },
    refetchInterval: window_ <= 5 ? 2000 : 10000,
    refetchIntervalInBackground: true,
  });

  const metric = useMemo(
    () => METRICS.find((m) => m.key === activeKey) ?? METRICS[0],
    [activeKey],
  );

  const labels = useMemo(
    () => points.map((p) => clockLabel(p.t, window_)),
    [points, window_],
  );

  const series = useMemo(
    () => metric.series.map((s) => points.map((p) => Number(p[s.field]) || 0)),
    [metric, points],
  );

  const yFormatter = useMemo(() => formatter(metric.unit), [metric.unit]);

  const chart = useCallback(() => (
    <Sparkline
      data={series[0] ?? []}
      data2={series[1]}
      data3={series[2]}
      stroke={metric.series[0]?.stroke}
      stroke2={metric.series[1]?.stroke}
      stroke3={metric.series[2]?.stroke}
      name1={t(metric.series[0]?.name ?? '')}
      name2={metric.series[1] ? t(metric.series[1].name) : undefined}
      name3={metric.series[2] ? t(metric.series[2].name) : undefined}
      labels={labels}
      height={210}
      strokeWidth={2.2}
      showGrid
      showAxes
      tickCountX={5}
      maxPoints={labels.length || 1}
      fillOpacity={0.16}
      markerRadius={2.6}
      showTooltip
      valueMin={0}
      valueMax={null}
      yFormatter={yFormatter}
      extrema={{ show: metric.series.length === 1, formatter: yFormatter }}
    />
  ), [series, metric, labels, yFormatter, t]);

  return (
    <div className="chp">
      <Tabs
        activeKey={activeKey}
        onChange={setActiveKey}
        className="chp-tabs"
        items={METRICS.map((m) => ({
          key: m.key,
          label: isMobile
            ? <span title={t(m.label)} aria-label={t(m.label)}>{m.icon}</span>
            : t(m.label),
          children: chart(),
        }))}
      />

      <div className="chp-window">
        {WINDOWS.map((w) => (
          <button
            key={w}
            type="button"
            className={`chp-win ${w === window_ ? 'is-active' : ''}`}
            onClick={() => setWindow(w)}
          >
            {w >= 60 ? `${w / 60}h` : `${w}m`}
          </button>
        ))}
      </div>

      {points.length === 0 && (
        <div className="chp-empty">{t('client.history.empty')}</div>
      )}
    </div>
  );
}
