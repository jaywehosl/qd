import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { BellOutlined } from '@ant-design/icons';

import { useMetricsPanel } from '@/layouts/MetricsPanelContext';
import { useClientNotifications } from '@/layouts/useClientNotifications';

export default function ClientBell() {
  const { t } = useTranslation();
  const { notifyOpen, toggleNotify } = useMetricsPanel();
  const { unread, markRead } = useClientNotifications();

  const flip = useCallback(() => {
    if (notifyOpen && unread > 0) markRead.mutate();
    toggleNotify();
  }, [notifyOpen, unread, markRead, toggleNotify]);

  const label = t('client.notify.title');

  return (
    <button
      type="button"
      className={`sidebar-theme-cycle sidebar-bell ${notifyOpen ? 'is-active' : ''}`}
      aria-label={label}
      title={label}
      onClick={flip}
    >
      <BellOutlined />
      {unread > 0 && <span className="notif-badge">{unread > 99 ? '99+' : unread}</span>}
    </button>
  );
}
