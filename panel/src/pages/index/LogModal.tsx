import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Dialog, Select } from '@/components/ds';
import { DownloadOutlined, SyncOutlined } from '@ant-design/icons';

import { HttpUtil, FileManager, PromiseUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import NodeSelect from './NodeSelect';
interface LogModalProps {
  open: boolean;
  onClose: () => void;
}

interface ParsedLog {
  date: string;
  time: string;
  stamp: string;
  levelText: string;
  levelClass: string;
  service: string;
  body: string;
}

const LEVELS = ['DEBUG', 'INFO', 'NOTICE', 'WARNING', 'ERROR'];
const LEVEL_CLASSES = ['level-debug', 'level-info', 'level-notice', 'level-warning', 'level-error'];

function parseLogLine(line: string): ParsedLog {
  const [head, ...rest] = (line || '').split(' - ');
  const message = rest.join(' - ');
  const parts = head.split(' ');

  let date = '';
  let time = '';
  let levelText: string;
  if (parts.length >= 3) {
    [date, time, levelText] = parts;
  } else {
    levelText = head;
  }

  const li = LEVELS.indexOf(levelText);
  const levelClass = li >= 0 ? LEVEL_CLASSES[li] : 'level-unknown';

  let service = '';
  let body = message || '';
  if (body.startsWith('XRAY:')) {
    service = 'XRAY:';
    body = body.slice('XRAY:'.length).trimStart();
  } else if (body) {
    service = 'QD:';
  }

  const stamp = [date, time].filter(Boolean).join(' ');

  return { date, time, stamp, levelText, levelClass, service, body };
}

export default function LogModal({ open, onClose }: LogModalProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const { nodes } = useNodesQuery();
  const [nodeId, setNodeId] = useState<number | null>(null);
  const [rows, setRows] = useState('20');
  const [level, setLevel] = useState('info');
  const [loading, setLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);
  const openRef = useRef(open);

  useEffect(() => {
    if (nodeId === null && nodes.length > 0) setNodeId(nodes[0].id);
  }, [nodes, nodeId]);

  const refresh = useCallback(async () => {
    if (nodeId === null) return;
    setLoading(true);
    try {
      const msg = await HttpUtil.post<string[]>(`/panel/api/nodes/${nodeId}/logs/${rows}`, { level });
      if (msg?.success) {
        setLogs(msg.obj || []);
      }
      await PromiseUtil.sleep(300);
    } finally {
      setLoading(false);
    }
  }, [nodeId, rows, level]);

  useEffect(() => {
    openRef.current = open;
    if (open) refresh();
  }, [open, refresh]);

  useEffect(() => {
    if (openRef.current) refresh();
  }, [nodeId, rows, level, refresh]);

  const parsedLogs = useMemo(() => logs.map(parseLogLine), [logs]);
  const nodeName = nodes.find((n) => n.id === nodeId)?.name || String(nodeId ?? '');

  async function clearLevel() {
    if (nodeId === null) return;
    const msg = await HttpUtil.post(`/panel/api/nodes/${nodeId}/logs/clear`, { level });
    if (msg?.success) await refresh();
  }

  async function downloadAll() {
    if (nodeId === null) return;
    const msg = await HttpUtil.get<string>(`/panel/api/nodes/${nodeId}/logs/download`);
    if (msg?.success && typeof msg.obj === 'string') {
      FileManager.downloadTextFile(msg.obj, `${nodeName}.log`);
    }
  }

  const titleNode = (
    <>
      {t('pages.index.logs')}
      <SyncOutlined spin={loading} className="reload-icon" onClick={refresh} />
    </>
  );

  return (
    <Dialog
      open={open}
      footer={null}
      width={isMobile ? 360 : 800}
      onOpenChange={(o) => { if (!o) onClose(); }}
      title={titleNode}
    >
      <NodeSelect
        nodes={nodes}
        value={nodeId}
        onChange={setNodeId}
        empty={t('pages.index.noNodes')}
      />

      <div className="log-toolbar">
        <div style={{ width: 80 }}>
          <Select
            value={rows}
            onChange={setRows}
            options={[
              { value: '10', label: '10' },
              { value: '20', label: '20' },
              { value: '50', label: '50' },
              { value: '100', label: '100' },
            ]}
          />
        </div>

        <div className="log-toolbar__mid">
          <Button danger disabled={nodeId === null} onClick={clearLevel}>
            {t('pages.index.clearLogs', { defaultValue: 'Clear this level' })}
          </Button>
          <Button icon={<DownloadOutlined />} disabled={nodeId === null} onClick={downloadAll}>
            {t('pages.index.downloadFullLog')}
          </Button>
        </div>

        <div style={{ width: 110 }}>
          <Select
            value={level}
            onChange={setLevel}
            options={[
              { value: 'debug', label: 'All' },
              { value: 'info', label: 'Info' },
              { value: 'notice', label: 'Notice' },
              { value: 'warning', label: 'Warning' },
              { value: 'err', label: 'Error' },
            ]}
          />
        </div>
      </div>

      <div className={`log-container ${isMobile ? 'log-container-mobile' : ''}`}>
        {parsedLogs.length === 0 ? (
          <div className="log-empty">No Record...</div>
        ) : isMobile ? (
          parsedLogs.map((log, idx) => (
            <div key={idx} className="log-card">
              <div className="log-card-head">
                {log.stamp && (
                  <span className="log-time">
                    {log.time && <span>{log.time}</span>}
                    {log.time && log.date ? ' ' : ''}
                    {log.date && <span className="log-date">{log.date}</span>}
                  </span>
                )}
                {log.levelText && (
                  <span className={`log-level-badge ${log.levelClass}`}>{log.levelText}</span>
                )}
              </div>
              {(log.body || log.service) && (
                <div className="log-body">
                  {log.service && <b>{log.service}</b>}
                  {log.service && log.body ? ' ' : ''}
                  {log.body && <span className="log-body-text">{log.body}</span>}
                </div>
              )}
            </div>
          ))
        ) : (
          parsedLogs.map((log, idx) => (
            <div key={idx} className="log-line">
              {log.stamp && <span className="log-stamp">{log.stamp}</span>}
              {log.stamp && log.levelText ? ' ' : ''}
              {log.levelText && <span className={`log-level ${log.levelClass}`}>{log.levelText}</span>}
              {(log.body || log.service) && (
                <>
                  <span> - </span>
                  {log.service && <b>{log.service}</b>}
                  {log.service && log.body ? ' ' : ''}
                  <span>{log.body}</span>
                </>
              )}
            </div>
          ))
        )}
      </div>
    </Dialog>
  );
}
