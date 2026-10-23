import { useEffect } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import AppSidebar from '@/layouts/AppSidebar';
import { MetricsPanelProvider } from '@/layouts/MetricsPanelContext';
import { SettingsControllerProvider } from '@/layouts/SettingsController';
import { HeaderActionsProvider } from '@/layouts/header-actions-context';
import { BusyOverlayProvider } from '@/layouts/BusyOverlayProvider';
import MetricsPanel from '@/pages/index/MetricsPanel';
import NotificationsBar from '@/pages/index/NotificationsBar';
import SensorWatcher from '@/pages/index/SensorWatcher';
import ClientOfflineWatcher from '@/pages/index/ClientOfflineWatcher';
import MaintenanceWatcher from '@/pages/index/MaintenanceWatcher';
import { useWebSocketBridge } from '@/api/websocketBridge';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useTheme } from '@/hooks/useTheme';
import { warmModules } from '@/lib/warmup';

export default function PanelLayout() {
  useWebSocketBridge();
  usePageTitle();
  const { isDark, isUltra } = useTheme();
  const { pathname } = useLocation();

  useEffect(() => {
    warmModules('admin');
    warmModules('client');
  }, []);

  useEffect(() => {
    const reset = () => {
      document.getElementById('content-layout')?.scrollTo({ top: 0 });
      window.scrollTo({ top: 0 });
    };
    reset();
    const raf = requestAnimationFrame(reset);
    return () => cancelAnimationFrame(raf);
  }, [pathname]);
  
  const pageClass = `panel-app-wrapper ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <div className={pageClass}>
      <MetricsPanelProvider>
        <BusyOverlayProvider>
          <HeaderActionsProvider>
              <SettingsControllerProvider>
              
                <div className="topbar-shell">
                  <AppSidebar />
                </div>
                <MetricsPanel />
                <NotificationsBar />
                <SensorWatcher />
                <ClientOfflineWatcher />
                <MaintenanceWatcher />
                <div className="panel-main-content">
                  <Outlet />
                </div>
              
              </SettingsControllerProvider>
          </HeaderActionsProvider>
        </BusyOverlayProvider>
      </MetricsPanelProvider>
    </div>
  );
}
