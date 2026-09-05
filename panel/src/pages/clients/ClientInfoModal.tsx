import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { CopyOutlined, DownOutlined } from '@ant-design/icons';

import { Button, Dialog, Tag, Tooltip, TooltipProvider } from '@/components/ds';
import { getMessage } from '@/utils/messageBus';
import { ClipboardManager, IntlUtil } from '@/utils';
import { useDatepicker } from '@/hooks/useDatepicker';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import type { ClientRecord } from '@/hooks/useClients';
interface ClientInfoModalProps {
  open: boolean;
  client: ClientRecord | null;
  isOnline: boolean;
  onDevice?: (action: 'block' | 'forget', fingerprint: string, blocked: boolean) => void;
  onAddress?: (ip: string) => void;
  onExit?: (nodeId: number) => void;
  onOpenChange: (open: boolean) => void;
}

export default function ClientInfoModal({
  open,
  client,
  isOnline,
  onDevice,
  onAddress,
  onExit,
  onOpenChange,
}: ClientInfoModalProps) {
  const { datepicker } = useDatepicker();
  const { t } = useTranslation();
  const message = getMessage();
  const { nodes } = useNodesQuery();

  const nodeName = useMemo(() => {
    const byId = new Map<number, string>();
    for (const n of nodes) byId.set(n.id, n.name || n.address || String(n.id));
    return (id?: number) => (id ? byId.get(id) ?? String(id) : '—');
  }, [nodes]);

  const dateLabel = (ts?: number) => (!ts || ts <= 0 ? '—' : IntlUtil.formatDate(ts, datepicker));

  const spanText = (ms: number): string => {
    const minutes = Math.max(0, Math.floor(ms / 60000));
    const d = Math.floor(minutes / 1440);
    const h = Math.floor((minutes % 1440) / 60);
    const m = minutes % 60;
    const parts: string[] = [];
    if (d > 0) parts.push(`${d} ${t(d === 1 ? 'pages.clients.unitDay' : 'pages.clients.unitDays')}`);
    if (h > 0) parts.push(`${h} ${t(h === 1 ? 'pages.clients.unitHour' : 'pages.clients.unitHours')}`);
    if (m > 0 || parts.length === 0) parts.push(`${m} ${t(m === 1 ? 'pages.clients.unitMinute' : 'pages.clients.unitMinutes')}`);
    return parts.join(' ');
  };

  const expiryLabel = (ts?: number) => {
    if (!ts) return '∞';
    if (ts < 0) { const days = Math.round(ts / -86400000); return `${t('pages.clients.delayedStart')}: ${days}d`; }
    return IntlUtil.formatDate(ts, datepicker);
  };

  async function copyValue(text: string) {
    if (!text) return;
    const ok = await ClipboardManager.copyText(String(text));
    if (ok) message.success(t('copied'));
  }

  function presenceTag(row: ClientRecord) {
    const now = Date.now();
    if (row.enable && isOnline) {
      const since = row.onlineSince || row.lastConnect || 0;
      const label = since > 0
        ? t('pages.clients.onlineFor', { value: spanText(now - since) })
        : t('pages.clients.online');
      return { tone: 'success' as const, label, title: dateLabel(since) };
    }
    const last = row.lastConnect || 0;
    const label = last > 0
      ? t('pages.clients.offlineFor', { value: spanText(now - last) })
      : t('pages.clients.neverConnected');
    return { tone: 'neutral' as const, label, title: dateLabel(last) };
  }

  const devices = client?.devices ?? [];
  const ipLog = client?.ipLog ?? [];
  const exitLog = client?.exitLog ?? [];

  const deviceLabel = useMemo(() => {
    const byPrint = new Map<string, string>();
    for (const d of devices) {
      const model = (d as { model?: string }).model || d.platform || '';
      byPrint.set(d.fingerprint, model);
    }
    return (print?: string) => (print ? byPrint.get(print) ?? print.slice(0, 12) : '—');
  }, [devices]);

  const field = (label: string, value: ReactNode) => (
    <div className="ci-field">
      <span className="ci-field__label">{label}</span>
      <span className="ci-field__value">{value}</span>
    </div>
  );

  const [shown, setShown] = useState<'devices' | 'ips' | 'exits' | null>(null);

  const [asking, setAsking] = useState<string | null>(null);

  const confirmDevice = (action: 'block' | 'forget', print: string, blocked: boolean) => {
    const key = `${action}:${print}`;
    if (asking !== key) {
      setAsking(key);
      window.setTimeout(() => setAsking((held) => (held === key ? null : held)), 4000);
      return;
    }
    setAsking(null);
    onDevice?.(action, print, blocked);
  };

  const confirmAddress = (ip: string) => {
    const key = `ip:${ip}`;
    if (asking !== key) {
      setAsking(key);
      window.setTimeout(() => setAsking((held) => (held === key ? null : held)), 4000);
      return;
    }
    setAsking(null);
    onAddress?.(ip);
  };

  const confirmExit = (nodeId: number) => {
    const key = `exit:${nodeId}`;
    if (asking !== key) {
      setAsking(key);
      window.setTimeout(() => setAsking((held) => (held === key ? null : held)), 4000);
      return;
    }
    setAsking(null);
    onExit?.(nodeId);
  };

  const sectionTitle = (key: 'devices' | 'ips' | 'exits', label: string, count: number) => (
    <button
      type="button"
      className={`ci-section-title${shown === key ? ' is-open' : ''}`}
      aria-expanded={shown === key}
      onClick={() => setShown(shown === key ? null : key)}
    >
      <DownOutlined className="ci-section-title__caret" />
      {label}
      <Tag tone={count > 0 ? 'primary' : 'neutral'} className="ci-count">{count}</Tag>
    </button>
  );

  return (
    <TooltipProvider>
      <Dialog
        open={open}
        onOpenChange={(o) => !o && onOpenChange(false)}
        title={client ? `${t('pages.clients.clientInfo')} — ${client.email}` : t('pages.clients.clientInfo')}
        width={760}
        autoHeight
        footer={null}
      >
        {client && (
          <div className="ci">
            {/* Same shape as the edit dialog: label above value, two halves per
                row, one rhythm down the whole panel. */}
            <div className="ci-pair">
              {field(t('pages.clients.connectionUri'), (
                client.uri ? (
                  <span className="ci-copyable">
                    <code title={client.uri}>{client.uri}</code>
                    <Tooltip title={t('copy')}>
                      <Button size="sm" variant="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(client.uri!)} />
                    </Tooltip>
                  </span>
                ) : <span className="hint">{t('pages.clients.noConnectionUri')}</span>
              ))}
              {field(t('pages.clients.uuid'), (
                <span className="ci-copyable">
                  <code title={client.uuid || ''}>{client.uuid || '—'}</code>
                  <Tooltip title={t('copy')}>
                    <Button size="sm" variant="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(client.uuid || '')} />
                  </Tooltip>
                </span>
              ))}
            </div>

            <div className="ci-pair">
              {field(t('pages.clients.duration'), (
                <span className="ci-value-row">
                  {!client.expiryTime
                    ? <Tag tone="primary" className="ci-duration">∞</Tag>
                    : <Tag tone={client.expiryTime < 0 ? 'primary' : 'neutral'} className="ci-duration">{expiryLabel(client.expiryTime)}</Tag>}
                  {(client.expiryTime ?? 0) > 0 && <span className="hint">{IntlUtil.formatRelativeTime(client.expiryTime)}</span>}
                </span>
              ))}
              {field(t('pages.clients.attachedGroup'), (
                client.group ? <Tag tone="primary">{client.group}</Tag> : <span className="hint">—</span>
              ))}
            </div>

            <div className="ci-pair">
              {field(t('pages.clients.comment'), (
                client.comment
                  ? <span className="ci-comment">{client.comment}</span>
                  : <span className="hint">—</span>
              ))}
              {field(t('pages.clients.state', { defaultValue: 'State' }), (
                <span className="ci-value-row">
                  {(() => {
                    const p = presenceTag(client);
                    return <Tooltip title={p.title}><Tag tone={p.tone}>{p.label}</Tag></Tooltip>;
                  })()}
                  <Tag tone={client.enable ? 'success' : 'neutral'}>{client.enable ? t('enabled') : t('disabled')}</Tag>
                </span>
              ))}
            </div>

            <div className="ci-pair">
              {field(t('pages.clients.createdAt', { defaultValue: 'Created' }), (
                <span className="ci-plain">{dateLabel(client.createdAt)}</span>
              ))}
              {field(t('pages.clients.updatedAt', { defaultValue: 'Updated' }), (
                <span className="ci-plain">{dateLabel(client.updatedAt)}</span>
              ))}
            </div>

            {sectionTitle('devices', t('pages.clients.devices'), devices.length)}
            {shown !== 'devices' ? null : devices.length === 0 ? (
              <div className="ci-empty">{t('pages.clients.noDevices')}</div>
            ) : (
              <div className="ci-rows">
                {devices.map((d) => (
                  <div key={d.fingerprint} className="ci-row">
                    <span className="ci-row__facts">
                    <Tag tone={(d as { blocked?: boolean }).blocked ? 'danger' : 'primary'}>
                      {(d as { blocked?: boolean }).blocked
                        ? t('pages.clients.deviceBlocked', { defaultValue: 'blocked' })
                        : (d as { kind?: string }).kind || '—'}
                    </Tag>
                    <Tag tone="success">{nodeName(d.nodeId)}</Tag>
                    {(d as { ip?: string }).ip && (
                      <Tag className="ci-row__mono">{(d as { ip?: string }).ip}</Tag>
                    )}
                    <Tag className="ci-row__mono">{(d.fingerprint || '').slice(0, 12)}</Tag>
                    <Tag>{(d as { model?: string }).model || '—'}</Tag>
                    <Tag>{d.platform || '—'}</Tag>
                    <Tag>{t('pages.clients.firstSeen')}: {dateLabel(d.firstSeen)}</Tag>
                    <Tag>{t('pages.clients.lastSeen')}: {dateLabel(d.lastSeen)}</Tag>
                    </span>
                    <span className="ci-row__actions">
                      <Button
                        onClick={() => confirmDevice(
                          'block',
                          d.fingerprint,
                          !(d as { blocked?: boolean }).blocked,
                        )}
                      >
                        {asking === `block:${d.fingerprint}`
                          ? t('pages.clients.confirmShort', { defaultValue: 'Sure?' })
                          : (d as { blocked?: boolean }).blocked
                            ? t('pages.clients.unblockDevice', { defaultValue: 'Unblock' })
                            : t('pages.clients.blockDevice', { defaultValue: 'Block' })}
                      </Button>
                      <Button danger onClick={() => confirmDevice('forget', d.fingerprint, false)}>
                        {asking === `forget:${d.fingerprint}`
                          ? t('pages.clients.confirmShort', { defaultValue: 'Sure?' })
                          : t('delete')}
                      </Button>
                    </span>
                  </div>
                ))}
              </div>
            )}

            {sectionTitle('ips', t('pages.clients.ipAddressLog'), ipLog.length)}
            {shown !== 'ips' ? null : ipLog.length === 0 ? (
              <div className="ci-empty">{t('pages.clients.noIpAddresses')}</div>
            ) : (
              <div className="ci-rows">
                <div className="ci-row ci-row--tools">
                  <span className="ci-row__facts">
                    <Tag>{t('pages.clients.clearIpLogHint', {
                      defaultValue: 'Addresses are remembered per node',
                    })}</Tag>
                  </span>
                  <span className="ci-row__actions">
                    <Button danger onClick={() => confirmAddress('')}>
                      {asking === 'ip:'
                        ? t('pages.clients.confirmShort', { defaultValue: 'Sure?' })
                        : t('pages.clients.clearIpLog', { defaultValue: 'Clear the log' })}
                    </Button>
                  </span>
                </div>
                {ipLog.map((e, idx) => (
                  <div key={`${e.ip}-${idx}`} className="ci-row">
                    <span className="ci-row__facts">
                      <Tag tone="primary" className="ci-row__mono">{e.ip}</Tag>
                      <Tag tone="success">{nodeName(e.nodeId)}</Tag>
                      <Tag>{deviceLabel(e.fingerprint)}</Tag>
                      <Tag>{t('pages.clients.lastSeen')}: {dateLabel(e.lastOnline)}</Tag>
                    </span>
                    <span className="ci-row__actions">
                      <Button danger onClick={() => confirmAddress(e.ip)}>
                        {asking === `ip:${e.ip}`
                          ? t('pages.clients.confirmShort', { defaultValue: 'Sure?' })
                          : t('delete')}
                      </Button>
                    </span>
                  </div>
                ))}
              </div>
            )}

            {sectionTitle('exits', t('pages.clients.exitLog', { defaultValue: 'Exit nodes used' }), exitLog.length)}
            {shown !== 'exits' ? null : exitLog.length === 0 ? (
              <div className="ci-empty">
                {t('pages.clients.noExits', { defaultValue: 'This client has not taken an exit yet' })}
              </div>
            ) : (
              <div className="ci-rows">
                {exitLog.map((e) => (
                  <div key={e.nodeId} className="ci-row">
                    <span className="ci-row__facts">
                      <Tag tone="success">{nodeName(e.nodeId)}</Tag>
                      <Tag>{t('pages.clients.firstSeen')}: {dateLabel(e.firstSeen)}</Tag>
                      <Tag>{t('pages.clients.lastSeen')}: {dateLabel(e.lastOnline)}</Tag>
                    </span>
                    <span className="ci-row__actions">
                      <Button danger onClick={() => confirmExit(e.nodeId)}>
                        {asking === `exit:${e.nodeId}`
                          ? t('pages.clients.confirmShort', { defaultValue: 'Sure?' })
                          : t('delete')}
                      </Button>
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </Dialog>
    </TooltipProvider>
  );
}
