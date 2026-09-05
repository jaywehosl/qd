import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import type { ComponentType } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  BellOutlined,
} from '@ant-design/icons';
import { Button } from '@/components/ds';

import { useMetricsPanel } from '@/layouts/MetricsPanelContext';
import { prefetchRoute } from '@/routes';
import { useNotifications } from '@/pages/index/useNotifications';
import { subscribe as notifSubscribe, getSnapshot as notifSnapshot } from '@/stores/notificationStore';
import { useHeaderActions } from '@/layouts/header-actions-context';
import { LanguageSelector, ThemeCycleButton } from '@/layouts/HeaderButtons';
import { useTheme } from '@/hooks/useTheme';
import WindowButtons from '@/components/ui/WindowButtons';
import BrandMark from '@/components/ui/BrandMark';
import { useClientState } from '@/hooks/useClientState';
import {
  InboundsIcon,
  ClientsIcon,
  GroupsIcon,
  NodesIcon,
  SettingsIcon,

  ApiDocsIcon,
} from '@/components/ui';
type IconName = 'inbound' | 'team' | 'groups' | 'setting' | 'cluster' | 'apidocs';

const iconByName: Record<IconName, ComponentType> = {
  inbound: InboundsIcon,
  team: ClientsIcon,
  groups: GroupsIcon,
  setting: SettingsIcon,

  cluster: NodesIcon,
  apidocs: ApiDocsIcon,
};

export default function AppSidebar() {
  const { t } = useTranslation();
  const { state: link } = useClientState();
  const { isDark, isUltra, cycleTheme } = useTheme();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { open: metricsOpen, toggle: toggleMetrics, notifyOpen, toggleNotify } = useMetricsPanel();
  const headerActions = useHeaderActions();

  // Real unread count, shared with the NotificationsBar strip: live-condition
  // alerts + active event notifications (sensors / log).
  const notifyActive = useSyncExternalStore(notifSubscribe, notifSnapshot, notifSnapshot).active;
  const notifyCount = useNotifications().length + notifyActive.length;

  // Active nav highlight. Hash-section clicks (/#clients …) use history.pushState
  // (so the page can smooth-scroll without a router nav) which does NOT update
  // react-router's `hash` — so we track the clicked key explicitly and fall back
  // to a route-derived key for cross-page navigation. clickedKey resets whenever
  // the actual route (pathname) changes.
  const [clickedKey, setClickedKey] = useState<string | null>(null);
  useEffect(() => { setClickedKey(null); }, [pathname]);
  const routeKey = useMemo(() => (pathname === '/panel' ? '/panel/inbounds' : pathname), [pathname]);
  const activeKey = clickedKey ?? routeKey;

  // The brand logo is the (slightly hidden) metrics status-bar toggle — it only
  // opens/closes the bar and never navigates.
  const onLogoClick = useCallback(() => {
    toggleMetrics();
  }, [toggleMetrics]);

  const tabs = useMemo<{ key: string; icon: IconName; title: string }[]>(() => [
    { key: '/panel/inbounds', icon: 'inbound', title: t('menu.inbounds') },
    { key: '/panel/clients', icon: 'team', title: t('menu.clients') },
    { key: '/panel/groups', icon: 'groups', title: t('menu.groups') },
    { key: '/panel/nodes', icon: 'cluster', title: t('menu.nodes') },
    { key: '/panel/settings', icon: 'setting', title: t('menu.settings') },
    // API Docs temporarily hidden from the header (the row was getting
    // crowded); the /api-docs route still works.
  ], [t]);

  const navItems = tabs;

  const openLink = useCallback((key: string) => {
    setClickedKey(key);
    navigate(key, { viewTransition: true });
  }, [navigate]);



  return (
    <header className={`antigravity-header ${metricsOpen ? 'metrics-open' : ''}`}>
      <div className="header-container">
        <div className="header-left">
          <div className="brand-block" data-drag="off" onClick={onLogoClick} style={{ cursor: 'pointer' }} title={t('menu.dashboard')}>
            <BrandMark
              state={link?.connected ? 'on' : link && link.nodes.reachable === 0 ? 'error' : 'off'}
              exit={!!link?.egress && link?.allowExit !== false}
            />
          </div>
        </div>

        <div className="header-center">
          <nav className="header-nav-list-container">
            <ul className="header-nav-list">
              {navItems.map((tab) => {
                const Icon = iconByName[tab.icon];
                const isActive = tab.key === activeKey;
                return (
                  <li key={tab.key} className="header-nav-item-wrapper">
                    <button
                      type="button"
                      className={`nav-menu-item ${isActive ? 'is-active' : ''}`}
                      onPointerEnter={() => prefetchRoute(tab.key)}
                      onClick={() => openLink(tab.key)}
                    >
                      <Icon />
                      <span>{tab.title}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </nav>
        </div>

        <div className="header-right">
          {headerActions && (headerActions.dirty || headerActions.restartNeeded) && (
            <div className="header-save-actions">
              {headerActions.dirty && headerActions.onDiscard && (
                <Button danger disabled={headerActions.busy} onClick={headerActions.onDiscard}>
                  {headerActions.discardText}
                </Button>
              )}
              {headerActions.dirty && (
                <Button variant="primary" loading={headerActions.busy} onClick={headerActions.onSave}>
                  {headerActions.saveText}
                </Button>
              )}
              {headerActions.restartNeeded && !headerActions.dirty && (
                <Button variant="primary" danger loading={headerActions.busy} onClick={headerActions.onRestart}>
                  {headerActions.restartText}
                </Button>
              )}
            </div>
          )}
          <div className="win-tray" data-drag="off">
          <Button
            className="client-admin-btn"
            onPointerEnter={() => prefetchRoute('/client')}
            onClick={() => navigate('/client', { viewTransition: true })}
          >
            {t('client.menu.back')}
          </Button>
          <button
            type="button"
            className={`sidebar-theme-cycle sidebar-bell ${notifyOpen ? 'is-active' : ''}`}
            aria-label={t('menu.notifications', { defaultValue: 'Notifications' })}
            title={t('menu.notifications', { defaultValue: 'Notifications' })}
            onClick={toggleNotify}
          >
            <BellOutlined />
            {notifyCount > 0 && (
              <span className="notif-badge">{notifyCount > 99 ? '99+' : notifyCount}</span>
            )}
          </button>
          <ThemeCycleButton
            id="theme-cycle"
            isDark={isDark}
            isUltra={isUltra}
            onCycle={() => cycleTheme('theme-cycle')}
            ariaLabel={t('menu.theme')}
          />
          <LanguageSelector />
          <WindowButtons />
          </div>
        </div>
      </div>

    </header>
  );
}
