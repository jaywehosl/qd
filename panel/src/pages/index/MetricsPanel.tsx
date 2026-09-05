import { Suspense, lazy, useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CloudServerOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import { message } from '@/components/ui';
import { LazyMount } from '@/components/utility';
import { useMetricsPanel } from '@/layouts/MetricsPanelContext';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { setMessageInstance } from '@/utils/messageBus';
import { HttpUtil } from '@/utils';
const BackupModal = lazy(() => import('./BackupModal'));
const SystemHistoryPanel = lazy(() => import('./SystemHistoryModal'));

export default function MetricsPanel() {
  const { t } = useTranslation();
  const { open } = useMetricsPanel();
  const { status, refresh } = useStatusQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => { setMessageInstance(messageApi); }, [messageApi]);

  const [backupOpen, setBackupOpen] = useState(false);
  const [chartsSeen, setChartsSeen] = useState(false);
  useEffect(() => {
    if (open) { setChartsSeen(true); return undefined; }
    const idle = window.requestIdleCallback;
    if (!idle) {
      const timer = window.setTimeout(() => setChartsSeen(true), 1200);
      return () => window.clearTimeout(timer);
    }
    const id = idle(() => setChartsSeen(true), { timeout: 4000 });
    return () => window.cancelIdleCallback?.(id);
  }, [open]);

  useEffect(() => {
    if (!open) return undefined;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = prev; };
  }, [open]);

  const basePath = window.X_UI_BASE_PATH || '';

  // Signals every node the panel is connected to to restart its datapath.
  const restartNetwork = useCallback(async () => {
    await HttpUtil.post('/panel/api/nodes/restartAll');
    await refresh();
  }, [refresh]);

  return (
    <>
      {messageContextHolder}

      <div className={`metrics-bar ${open ? 'is-open' : ''}`} aria-hidden={!open}>
        <div className="mb-container">
          {/* ---- CENTER: control buttons (under the nav) ---- */}
          <div className="mb-center">
            <div className="vertical-tabs-container mb-controls">
              <button type="button" className="vtab-btn" style={{ '--i': 0 } as React.CSSProperties} onClick={restartNetwork}>
                <span className="vtab-icon"><ReloadOutlined /></span>
                {t('pages.index.restartNetwork')}
              </button>
              <button type="button" className="vtab-btn" style={{ '--i': 1 } as React.CSSProperties} onClick={() => setBackupOpen(true)}>
                <span className="vtab-icon"><CloudServerOutlined /></span>
                {t('pages.index.backupTitle')}
              </button>
            </div>
          </div>

          <div className="mb-history">
            <Suspense fallback={null}>
              {chartsSeen && <SystemHistoryPanel status={status} />}
            </Suspense>
          </div>
        </div>
      </div>

      <LazyMount when={backupOpen}>
        <BackupModal open={backupOpen} basePath={basePath} onClose={() => setBackupOpen(false)} />
      </LazyMount>
    </>
  );
}
