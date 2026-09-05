import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  SettingOutlined,
  BellOutlined,
  GlobalOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import { Card } from '@/components/ds';
import { Spin, VerticalTabs } from '@/components/ui';
import BackToTop from '@/components/ui/BackToTop';
import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useSettingsController } from '@/layouts/settings-controller-context';
import PreferencesTab from './PreferencesTab';
import DnsTab from './DnsTab';
import DatapathTab from './DatapathTab';
import NotificationsTab from '@/pages/appearance/NotificationsTab';
const tabSlugs = ['preferences', 'dns', 'datapath', 'notifications'];

function scrollTarget() {
  return document.getElementById('content-layout') as HTMLElement;
}

export default function SettingsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra } = useTheme();
  const { isMobile } = useMediaQuery();
  const navigate = useNavigate();

  const {
    allSetting,
    updateSetting,
    fetched,
  } = useSettingsController();

  const tabItems = useMemo(() => [
    { key: 'preferences', label: t('pages.settings.preferences'), icon: <SettingOutlined /> },
    { key: 'dns', label: t('pages.settings.dns', { defaultValue: 'DNS' }), icon: <GlobalOutlined /> },
    { key: 'datapath', label: t('pages.settings.datapath'), icon: <ThunderboltOutlined /> },
    { key: 'notifications', label: t('pages.settings.notifications'), icon: <BellOutlined /> },
  ], [t]);

  const location = useLocation();
  const slug = location.hash.replace(/^#/, '');
  const activeSlug = tabSlugs.includes(slug) ? slug : 'preferences';

  const pageClass = useMemo(
    () => ['settings-page', isDark && 'is-dark', isUltra && 'is-ultra'].filter(Boolean).join(' '),
    [isDark, isUltra],
  );

  const categoryBody = useMemo(() => {
    switch (activeSlug) {
      case 'notifications': return <NotificationsTab />;
      case 'dns': return <DnsTab allSetting={allSetting} updateSetting={updateSetting} />;
      case 'datapath': return <DatapathTab allSetting={allSetting} updateSetting={updateSetting} />;
      default: return <PreferencesTab allSetting={allSetting} updateSetting={updateSetting} />;
    }
  }, [activeSlug, allSetting, updateSetting]);

  return (
    <div className={pageClass}>
      <div className="content-shell">
        <div id="content-layout" className="content-area">
          <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
            {!fetched ? (
              <div className="loading-spacer" />
            ) : (
              <div className="panel-page">
                {/* The security-warning and "needs restart" alerts now live in
                    the global status-bar notification strip, not on-page. */}
                <BackToTop target={scrollTarget} visibilityHeight={200} />

                <div className="section-header">
                  <h2>{t('menu.panelSettings', { defaultValue: 'Panel Settings' })}</h2>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: isMobile ? 8 : 12 }}>
                  <div className="inb-roles">
                    <VerticalTabs
                      items={tabItems}
                      activeKey={activeSlug}
                      onChange={(key) => navigate(`#${key}`)}
                    />
                  </div>

                  <Card style={{ minHeight: 450 }}>
                    <div className="qd-page-swap" key={activeSlug}>
                      {categoryBody}
                    </div>
                  </Card>
                </div>
              </div>
            )}
          </Spin>
        </div>
      </div>
    </div>
  );
}
