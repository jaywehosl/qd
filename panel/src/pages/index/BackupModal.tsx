import { useTranslation } from 'react-i18next';
import { Button, Dialog, toast } from '@/components/ds';
import { DownloadOutlined, UploadOutlined } from '@ant-design/icons';

import { HttpUtil, PromiseUtil } from '@/utils';
import { markBackupDone } from '@/stores/notificationStore';
import { useBusyOverlay, BOOT_BUSY_KEY } from '@/layouts/busy-overlay-context';
async function waitForPanelBack(timeoutMs = 90000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await PromiseUtil.sleep(2500);
    const res = await HttpUtil.get('/panel/api/server/status', undefined, {
      silent: true,
      skipAuthRedirect: true,
    });
    if (res?.success) return true;
  }
  return false;
}

interface BackupModalProps {
  open: boolean;
  basePath: string;
  onClose: () => void;
}

export default function BackupModal({ open, basePath: _basePath, onClose }: BackupModalProps) {
  const { t } = useTranslation();
  const busyOverlay = useBusyOverlay();
  const isPostgres = window.X_UI_DB_TYPE === 'postgres';

  async function exportDb() {
    markBackupDone();
    const res = await fetch('/panel/api/server/getDb', {
      headers: { 'X-QD-Token': sessionStorage.getItem('qd.token') || '' },
    });
    if (!res.ok) {
      toast.error(t('pages.index.exportDatabaseError', { defaultValue: 'Could not read the database' }));
      return;
    }
    const url = URL.createObjectURL(await res.blob());
    const link = document.createElement('a');
    link.href = url;
    link.download = 'qd-network.db';
    link.click();
    URL.revokeObjectURL(url);
  }

  function importDb() {
    const fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = isPostgres ? '.dump' : '.db';
    fileInput.addEventListener('change', async (e) => {
      const dbFile = (e.target as HTMLInputElement).files?.[0];
      if (!dbFile) return;

      const formData = new FormData();
      formData.append('db', dbFile);

      onClose();
      const overlay = {
        title: t('pages.index.restoringBackup'),
        subtitle: t('pages.settings.restartingDesc'),
      };
      busyOverlay.show(overlay);
      try { localStorage.setItem(BOOT_BUSY_KEY, JSON.stringify(overlay)); } catch { /* ignore */ }

      const importP = HttpUtil.post('/panel/api/server/importDB', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        skipAuthRedirect: true,
        silent: true,
      }).then((r) => ({ kind: 'resp' as const, r })).catch(() => ({ kind: 'err' as const }));

      const race = await Promise.race([
        importP,
        PromiseUtil.sleep(8000).then(() => ({ kind: 'pending' as const })),
      ]);

      if (race.kind === 'resp' && !race.r?.success) {
        try { localStorage.removeItem(BOOT_BUSY_KEY); } catch { /* ignore */ }
        busyOverlay.hide();
        toast.error(race.r?.msg || t('pages.index.importDatabaseError'));
        return;
      }

      const back = await waitForPanelBack();
      try { localStorage.removeItem(BOOT_BUSY_KEY); } catch { /* ignore */ }
      busyOverlay.hide();

      if (back) {
        toast.success(t('pages.index.importDatabaseSuccess', { defaultValue: 'Database restored — panel is back online' }));
        await PromiseUtil.sleep(1500);
        window.location.reload();
      } else {
        toast.error(t('pages.index.importDatabaseTimeout', { defaultValue: 'Restore sent, but the panel did not come back in time — check the server.' }));
      }
    });
    fileInput.click();
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => { if (!o) onClose(); }}
      title={t('pages.index.backupTitle')}
      footer={null}
    >
      {isPostgres && (
        <div className="backup-description" style={{ marginBottom: 16 }}>
          {t('pages.index.backupPostgresNote')}
        </div>
      )}
      <div className="backup-list">
        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.exportDatabase')}</div>
            <div className="backup-description">
              {isPostgres ? t('pages.index.exportDatabasePgDesc') : t('pages.index.exportDatabaseDesc')}
            </div>
          </div>
          <Button variant="primary" onClick={exportDb} icon={<DownloadOutlined />} />
        </div>

        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.importDatabase')}</div>
            <div className="backup-description">
              {isPostgres ? t('pages.index.importDatabasePgDesc') : t('pages.index.importDatabaseDesc')}
            </div>
          </div>
          <Button variant="primary" onClick={importDb} icon={<UploadOutlined />} />
        </div>
      </div>
    </Dialog>
  );
}
