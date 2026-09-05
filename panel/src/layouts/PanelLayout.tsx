import { useEffect } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import AppSidebar from '@/layouts/AppSidebar';
import { MetricsPanelProvider } from '@/layouts/MetricsPanelContext';
import { SettingsControllerProvider } from '@/layouts/SettingsController';
import { PublishControllerProvider } from '@/layouts/PublishController';
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

export default function PanelLayout() {
  useWebSocketBridge();
  usePageTitle();
  const { isDark, isUltra } = useTheme();
  const { pathname } = useLocation();

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
            {/* Always-mounted editor controllers: their drafts (and thus the
                global Save/Restart) survive navigating away from their pages. */}
            <PublishControllerProvider>
              <SettingsControllerProvider>
              
                {/* Header + metrics bar share ONE fixed glass shell (single
                    backdrop-filter) so there's no seam between the two surfaces. */}
                <div className="topbar-shell">
                  <AppSidebar />
                  <MetricsPanel />
                  <NotificationsBar />
                </div>
                <SensorWatcher />
                <ClientOfflineWatcher />
                <MaintenanceWatcher />
                <div className="panel-main-content">
                  <Outlet />
                </div>
              
              </SettingsControllerProvider>
              
            </PublishControllerProvider>
          </HeaderActionsProvider>
        </BusyOverlayProvider>
      </MetricsPanelProvider>
    </div>
  );
}
