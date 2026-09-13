import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Modal,
  Tag,
  Tooltip,
} from '@/components/ui';
import { Divider, Tabs } from '@/components/ds';
import { CopyOutlined } from '@ant-design/icons';

import { IntlUtil, SizeFormatter, ColorUtils } from '@/utils';
import { InfinityIcon } from '@/components/ui';
import { useDatepicker } from '@/hooks/useDatepicker';

import { buildInboundInfo, copyText, statsColor } from './helpers';
import type { ClientSetting, ClientStats, InboundInfo, InboundInfoModalProps } from './types';

export default function InboundInfoModal({
  open,
  onClose,
  dbInbound,
  clientIndex = 0,
  expireDiff = 0,
  trafficDiff = 0,
  ipLimitEnable = false,
  tgBotEnable = false,
  subSettings,
  lastOnlineMap = {},
}: InboundInfoModalProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();

  const [inbound, setInbound] = useState<InboundInfo | null>(null);
  const [clientSettings, setClientSettings] = useState<ClientSetting | null>(null);
  const [clientStats, setClientStats] = useState<ClientStats | null>(null);
  const [subLink, setSubLink] = useState('');
  const [subJsonLink, setSubJsonLink] = useState('');
  const [activeTab, setActiveTab] = useState('client');

  useEffect(() => {
    if (!open || !dbInbound) return;
    const info = buildInboundInfo(dbInbound);
    setInbound(info);
    setActiveTab(info.clients.length > 0 ? 'client' : 'inbound');

    const idx = clientIndex ?? 0;
    const clientSet = info.clients.length > 0 ? (info.clients[idx] || null) : null;
    setClientSettings(clientSet);
    setClientStats(
      clientSet
        ? (dbInbound.clientStats || []).find((s) => s.email === clientSet.email) || null
        : null,
    );

    if (clientSet?.subId) {
      setSubLink((subSettings?.subURI || '') + clientSet.subId);
      setSubJsonLink(
        subSettings?.subJsonEnable ? (subSettings?.subJsonURI || '') + clientSet.subId : '',
      );
    } else {
      setSubLink('');
      setSubJsonLink('');
    }
  }, [open, dbInbound, clientIndex, subSettings]);

  const isEnable = useMemo(() => {
    if (clientSettings) return !!clientSettings.enable;
    return dbInbound?.enable ?? true;
  }, [clientSettings, dbInbound]);

  const isDepleted = useMemo(() => {
    if (!clientStats || !clientSettings) return false;
    const total = clientStats.total ?? 0;
    const used = (clientStats.up ?? 0) + (clientStats.down ?? 0);
    if (total > 0 && used >= total) return true;
    const expiry = clientSettings.expiryTime ?? 0;
    if (expiry > 0 && Date.now() >= expiry) return true;
    return false;
  }, [clientStats, clientSettings]);

  const remainingStats = useMemo(() => {
    if (!clientStats || !clientSettings) return '-';
    const remained = clientStats.total - clientStats.up - clientStats.down;
    return remained > 0 ? SizeFormatter.sizeFormat(remained) : '-';
  }, [clientStats, clientSettings]);

  const formatLastOnline = useCallback(
    (email: string) => {
      const ts = lastOnlineMap[email];
      if (!ts) return '-';
      return IntlUtil.formatDate(ts, datepicker);
    },
    [lastOnlineMap, datepicker],
  );

  const showClientTab = !!clientSettings;
  const showSubscriptionTab = !!(subSettings?.enable && clientSettings?.subId);

  if (!dbInbound || !inbound) {
    return (
      <Modal open={open} onCancel={onClose} title={t('pages.inbounds.inboundInfo')} footer={null} width={640} />
    );
  }

  const clientTab = (
    <>
      <table className="info-table block">
        <tbody>
          <tr>
            <td>{t('pages.inbounds.email')}</td>
            <td>
              {clientSettings?.email ? (
                <Tag color="green">{clientSettings.email}</Tag>
              ) : (
                <Tag color="red">{t('none')}</Tag>
              )}
            </td>
          </tr>
          {clientSettings?.id && (
            <tr><td>ID</td><td><Tag>{clientSettings.id}</Tag></td></tr>
          )}
          {clientSettings?.password && (
            <tr>
              <td>{t('password')}</td>
              <td><Tag className="info-large-tag">{clientSettings.password}</Tag></td>
            </tr>
          )}
          <tr>
            <td>{t('status')}</td>
            <td>
              {isDepleted ? (
                <Tag color="red">{t('depleted')}</Tag>
              ) : isEnable ? (
                <Tag color="green">{t('enabled')}</Tag>
              ) : (
                <Tag>{t('disabled')}</Tag>
              )}
            </td>
          </tr>
          {clientStats && (
            <tr>
              <td>{t('usage')}</td>
              <td>
                <Tag color="green">{SizeFormatter.sizeFormat(clientStats.up + clientStats.down)}</Tag>
                <Tag>
                  ↑ {SizeFormatter.sizeFormat(clientStats.up)} /
                  {' '}{SizeFormatter.sizeFormat(clientStats.down)} ↓
                </Tag>
              </td>
            </tr>
          )}
          <tr>
            <td>{t('pages.inbounds.createdAt')}</td>
            <td>
              {clientSettings?.created_at ? (
                <Tag>{IntlUtil.formatDate(clientSettings.created_at, datepicker)}</Tag>
              ) : <Tag>-</Tag>}
            </td>
          </tr>
          <tr>
            <td>{t('pages.inbounds.updatedAt')}</td>
            <td>
              {clientSettings?.updated_at ? (
                <Tag>{IntlUtil.formatDate(clientSettings.updated_at, datepicker)}</Tag>
              ) : <Tag>-</Tag>}
            </td>
          </tr>
          <tr>
            <td>{t('lastOnline')}</td>
            <td><Tag>{formatLastOnline(clientSettings?.email || '')}</Tag></td>
          </tr>
          {clientSettings?.comment && (
            <tr><td>{t('comment')}</td><td><Tag className="info-large-tag">{clientSettings.comment}</Tag></td></tr>
          )}
          {ipLimitEnable && (
            <tr><td>{t('pages.inbounds.IPLimit')}</td><td><Tag>{clientSettings?.limitIp ?? 0}</Tag></td></tr>
          )}
        </tbody>
      </table>

      <table className="info-table summary-table">
        <thead>
          <tr>
            <th>{t('remained')}</th>
            <th>{t('pages.inbounds.totalUsage')}</th>
            <th>{t('pages.inbounds.expireDate')}</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>
              {clientStats && (clientSettings?.totalGB ?? 0) > 0 ? (
                <Tag color={statsColor(clientStats, trafficDiff)}>{remainingStats}</Tag>
              ) : !clientSettings?.totalGB || clientSettings.totalGB <= 0 ? (
                <Tag color="purple"><InfinityIcon /></Tag>
              ) : null}
            </td>
            <td>
              {(clientSettings?.totalGB ?? 0) > 0 ? (
                <Tag color={clientStats ? statsColor(clientStats, trafficDiff) : 'default'}>
                  {SizeFormatter.sizeFormat(clientSettings!.totalGB!)}
                </Tag>
              ) : (
                <Tag color="purple"><InfinityIcon /></Tag>
              )}
            </td>
            <td>
              {(clientSettings?.expiryTime ?? 0) > 0 ? (
                <Tag color={ColorUtils.usageColor(Date.now(), expireDiff, clientSettings!.expiryTime!)}>
                  {IntlUtil.formatDate(clientSettings!.expiryTime!, datepicker)}
                </Tag>
              ) : (clientSettings?.expiryTime ?? 0) < 0 ? (
                <Tag color="green">{clientSettings!.expiryTime! / -86400000} {t('day')}</Tag>
              ) : (
                <Tag color="purple"><InfinityIcon /></Tag>
              )}
            </td>
          </tr>
        </tbody>
      </table>

      {tgBotEnable && clientSettings?.tgId && (
        <>
          <Divider>Telegram</Divider>
          <div className="tg-row">
            <Tag color="blue">{clientSettings.tgId}</Tag>
            <Tooltip title={t('copy')}>
              <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(clientSettings.tgId, t)} />
            </Tooltip>
          </div>
        </>
      )}

      {showSubscriptionTab && (
        <>
          <Divider>{t('subscription.title')}</Divider>
          <div className="link-panel">
            <div className="link-panel-header">
              <Tag color="green">{t('subscription.title')}</Tag>
              <Tooltip title={t('copy')}>
                <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(subLink, t)} />
              </Tooltip>
            </div>
            <a href={subLink} target="_blank" rel="noopener noreferrer" className="link-panel-anchor">{subLink}</a>
          </div>
          {subSettings?.subJsonEnable && subJsonLink && (
            <div className="link-panel">
              <div className="link-panel-header">
                <Tag color="green">JSON</Tag>
                <Tooltip title={t('copy')}>
                  <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(subJsonLink, t)} />
                </Tooltip>
              </div>
              <a href={subJsonLink} target="_blank" rel="noopener noreferrer" className="link-panel-anchor">{subJsonLink}</a>
            </div>
          )}
        </>
      )}
    </>
  );

  const inboundTab = (
    <dl className="info-list">
      <div className="info-row">
        <dt>{t('pages.inbounds.protocol')}</dt>
        <dd><Tag color="purple">{dbInbound.protocol}</Tag></dd>
      </div>
      <div className="info-row">
        <dt>{t('pages.inbounds.address')}</dt>
        <dd><Tag className="value-tag">{dbInbound.address}</Tag></dd>
      </div>
      <div className="info-row">
        <dt>{t('pages.inbounds.port')}</dt>
        <dd><Tag>{dbInbound.port}</Tag></dd>
      </div>
    </dl>
  );

  const tabItems = [];
  if (showClientTab) {
    tabItems.push({ key: 'client', label: t('pages.inbounds.client'), children: clientTab });
  }
  tabItems.push({ key: 'inbound', label: t('pages.inbounds.inbound'), children: inboundTab });

  return (
    <Modal open={open} onCancel={onClose} title={t('pages.inbounds.inboundInfo')} footer={null} width={640} destroyOnHidden>
      <Tabs activeKey={activeTab} onChange={setActiveTab} items={tabItems} />
    </Modal>
  );
}
