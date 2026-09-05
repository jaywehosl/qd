import { useCallback, useEffect, useMemo, lazy, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  ApiOutlined,
  ControlOutlined,
  SettingOutlined,
} from '@ant-design/icons';

import { Button } from '@/components/ds';
import { useTheme } from '@/hooks/useTheme';
import { useClientState } from '@/hooks/useClientState';
import { HeaderActionsProvider, useHeaderActions } from '@/layouts/header-actions-context';
import { ClientSettingsControllerProvider } from '@/layouts/ClientSettingsController';
import { MetricsPanelProvider } from '@/layouts/MetricsPanelContext';
import { LanguageSelector, ThemeCycleButton } from '@/layouts/HeaderButtons';
import ClientBell from '@/layouts/ClientBell';
import WindowButtons from '@/components/ui/WindowButtons';
import BrandMark from '@/components/ui/BrandMark';
import { useMetricsPanel } from '@/layouts/MetricsPanelContext';
import { prefetchRoute } from '@/routes';
const ClientHistoryPanel = lazy(() => import('@/pages/client/ClientHistoryPanel'));
import ClientNotificationsBar from '@/layouts/ClientNotificationsBar';
const NAV = [
  { key: '/client', icon: ApiOutlined, label: 'client.menu.connect' },
  { key: '/client/routing', icon: ControlOutlined, label: 'client.menu.routing' },
  { key: '/client/settings', icon: SettingOutlined, label: 'client.menu.settings' },
];

/**
 * The client's own shell. It deliberately does not reuse the panel header:
 * a user who is not an admin has no business seeing entries for nodes,
 * groups or a publish button.
 */
function ClientShell() {
  const { t } = useTranslation();
  const { open: metricsOpen, toggle: toggleMetrics } = useMetricsPanel();
  const [chartsSeen, setChartsSeen] = useState(false);
  useEffect(() => {
    if (metricsOpen) { setChartsSeen(true); return undefined; }
    const idle = window.requestIdleCallback;
    if (!idle) {
      const timer = window.setTimeout(() => setChartsSeen(true), 1200);
      return () => window.clearTimeout(timer);
    }
    const id = idle(() => setChartsSeen(true), { timeout: 4000 });
    return () => window.cancelIdleCallback?.(id);
  }, [metricsOpen]);
  const { isDark, isUltra, cycleTheme } = useTheme();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { state } = useClientState();
  const headerActions = useHeaderActions();

  const activeKey = useMemo(() => {
    if (pathname.startsWith('/client/settings')) return '/client/settings';
    if (pathname.startsWith('/client/routing')) return '/client/routing';
    return '/client';
  }, [pathname]);

  const go = useCallback((key: string) => {
    if (pathname === key) return;
    navigate(key, { viewTransition: true });
    window.scrollTo({ top: 0 });
  }, [navigate, pathname]);

  const shellClass = ['client-shell', isDark && 'is-dark', isUltra && 'is-ultra']
    .filter(Boolean).join(' ');

  // With nothing imported the shell would frame an import screen that already
  // draws its own chrome, so it steps aside entirely.
  if (!state?.imported) return <Outlet />;

  return (
    <div className={shellClass}>

      <div className="topbar-shell">
        <header className="antigravity-header client-header">
        <div className="header-container">
          <div className="header-left">
            <div className="brand-block" data-drag="off" style={{ cursor: 'pointer' }} onClick={toggleMetrics}>
              <BrandMark
                state={state.connected ? 'on' : state.nodes.reachable === 0 ? 'error' : 'off'}
                exit={!!state.egress && state.allowExit !== false}
              />
            </div>
          </div>

          <div className="header-center">
            <nav className="header-nav-list-container">
              <ul className="header-nav-list">
                {NAV.map((item) => {
                  const Icon = item.icon;
                  return (
                    <li key={item.key} className="header-nav-item-wrapper">
                      <button
                        type="button"
                        className={`nav-menu-item ${item.key === activeKey ? 'is-active' : ''}`}
                        onPointerEnter={() => prefetchRoute(item.key)}
                        onClick={() => go(item.key)}
                      >
                        <Icon />
                        <span>{t(item.label)}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </nav>
          </div>

          <div className="header-right">
            {headerActions?.dirty && (
              <div className="header-save-actions">
                {headerActions.onDiscard && (
                  <Button danger disabled={headerActions.busy} onClick={headerActions.onDiscard}>
                    {headerActions.discardText}
                  </Button>
                )}
                <Button variant="primary" loading={headerActions.busy} onClick={headerActions.onSave}>
                  {headerActions.saveText}
                </Button>
              </div>
            )}
            <div className="win-tray" data-drag="off">
              {state.admin && (
                <Button
                  className="client-admin-btn"
                  onPointerEnter={() => prefetchRoute('/panel/inbounds')}
                  onClick={() => navigate('/panel', { viewTransition: true })}
                >
                  {t('client.menu.admin')}
                </Button>
              )}
              <ClientBell />
              <ThemeCycleButton
                id="client-theme-cycle"
                isDark={isDark}
                isUltra={isUltra}
                onCycle={() => cycleTheme('client-theme-cycle')}
                ariaLabel={t('menu.theme')}
              />
              <LanguageSelector />
              <WindowButtons />
            </div>
          </div>
        </div>
        </header>
        <div className={`metrics-bar is-client ${metricsOpen ? 'is-open' : ''}`} aria-hidden={!metricsOpen}>
          <div className="mb-container">
            <Suspense fallback={null}>
              {chartsSeen && <ClientHistoryPanel />}
            </Suspense>
          </div>
        </div>
        <ClientNotificationsBar />
      </div>

      <div className="client-main">
        <Outlet />
      </div>
    </div>
  );
}

export default function ClientLayout() {
  return (
    <MetricsPanelProvider>
      <HeaderActionsProvider>
        <ClientSettingsControllerProvider>
          <ClientShell />
        </ClientSettingsControllerProvider>
      </HeaderActionsProvider>
    </MetricsPanelProvider>
  );
}
