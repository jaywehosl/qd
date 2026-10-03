import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button, Dialog } from '@/components/ds';
import { HttpUtil, IntlUtil } from '@/utils';
import { useDatepicker } from '@/hooks/useDatepicker';
import type { ClientDevice, ClientRecord } from '@/hooks/useClients';

type Device = ClientDevice & { blocked?: boolean; model?: string; kind?: string; ip?: string };
type Blocked = { email: string; device: Device };

interface BlockedDevicesModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChanged: () => void;
}

export default function BlockedDevicesModal({ open, onOpenChange, onChanged }: BlockedDevicesModalProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const [rows, setRows] = useState<Blocked[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(async () => {
    const msg = await HttpUtil.get<ClientRecord[]>('/panel/api/clients/list', undefined, { silent: true });
    const list = Array.isArray(msg?.obj) ? msg.obj : [];
    setRows(list.flatMap((c) => ((c.devices ?? []) as Device[])
      .filter((d) => d.blocked)
      .map((device) => ({ email: c.email, device }))));
  }, []);

  useEffect(() => {
    if (open) void load();
  }, [open, load]);

  const unblock = async ({ email, device }: Blocked) => {
    setBusy(`${email}:${device.fingerprint}`);
    await HttpUtil.post('/panel/api/clients/devices/block',
      { email, fingerprint: device.fingerprint, blocked: false },
      { headers: { 'Content-Type': 'application/json' } });
    await load();
    setBusy(null);
    onChanged();
  };

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('pages.clients.blockedDevices')}
      autoHeight
    >
      <div className="ms">
        {rows !== null && rows.length === 0 && <div className="ms-empty">{t('pages.clients.noBlockedDevices')}</div>}
        {rows !== null && rows.length > 0 && (
          <div className="ms-items">
            {rows.map((row) => {
              const { device: d } = row;
              const key = `${row.email}:${d.fingerprint}`;
              return (
                <div key={key} className="ms-item">
                  <div className="ms-item__top">
                    <span className="ms-item__name">{d.model || d.platform || d.fingerprint.slice(0, 12)}</span>
                    <span className="ms-item__owner">{row.email}</span>
                  </div>
                  <span className="ms-item__line">
                    {[d.platform, d.kind, d.version || t('pages.clients.noVersion')].filter(Boolean).join(' · ')}
                  </span>
                  {d.ip && <span className="ms-item__line is-mono">{d.ip}</span>}
                  <span className="ms-item__line">
                    {t('pages.clients.lastSeen')}: {d.lastSeen ? IntlUtil.formatDate(d.lastSeen, datepicker) : '—'}
                  </span>
                  <div className="ms-item__acts">
                    <Button size="sm" loading={busy === key} onClick={() => unblock(row)}>
                      {t('pages.clients.unblockDevice', { defaultValue: 'Unblock' })}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </Dialog>
  );
}
