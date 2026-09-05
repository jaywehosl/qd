import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import PlanVerificationModal from '@/components/ui/PlanVerificationModal';
import DangerConfirmModal from '@/components/ui/DangerConfirmModal';
import { HttpUtil, PromiseUtil } from '@/utils';
import { getMessage } from '@/utils/messageBus';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { AllSettingSchema } from '@/schemas/setting';
import { SettingsControllerContext, type SettingsControllerValue } from '@/layouts/settings-controller-context';
import { useBusyOverlay, BOOT_BUSY_KEY } from '@/layouts/busy-overlay-context';

interface ApiMsg { success?: boolean }

const PLAN_VERIFICATION_ENABLED = false;

const ACCESS_CRITICAL_FIELDS: { key: string; label: string }[] = [];

const RESTART_NEEDED_KEY = 'uup.restartNeeded';
function loadRestartNeeded(): boolean {
  try { return localStorage.getItem(RESTART_NEEDED_KEY) === '1'; } catch { return false; }
}
function persistRestartNeeded(v: boolean): void {
  try {
    if (v) localStorage.setItem(RESTART_NEEDED_KEY, '1');
    else localStorage.removeItem(RESTART_NEEDED_KEY);
  } catch { /* localStorage unavailable — degrade to in-memory only */ }
}

export function SettingsControllerProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const message = getMessage();
  const {
    allSetting,
    originalSetting,
    updateSetting,
    commitSetting,
    fetched,
    spinning,
    setSpinning,
    saveDisabled,
    saveAll,
  } = useAllSettings();

  const busyOverlay = useBusyOverlay();
  const [showPlan, setShowPlan] = useState(false);
  const [showDanger, setShowDanger] = useState(false);
  const [restartNeeded, setRestartNeededState] = useState<boolean>(loadRestartNeeded);
  const setRestartNeeded = useCallback((v: boolean) => {
    persistRestartNeeded(v);
    setRestartNeededState(v);
  }, []);


  const rebuildUrlAfterRestart = useCallback(
    (): string => window.location.href,
    [],
  );

  const executeSave = useCallback(async () => {
    setShowPlan(false);
    setSpinning(true);
    try {
      const msg = await saveAll();
      if (msg?.success) setRestartNeeded(true);
    } finally {
      setSpinning(false);
    }
  }, [saveAll, setSpinning, setRestartNeeded]);

  const changedDangerFields = useMemo(() => {
    if (!originalSetting) return [] as string[];
    const orig = originalSetting as unknown as Record<string, unknown>;
    const draft = allSetting as unknown as Record<string, unknown>;
    return ACCESS_CRITICAL_FIELDS.filter((f) => draft[f.key] !== orig[f.key]).map((f) => f.label);
  }, [allSetting, originalSetting]);

  const proceedSave = useCallback(() => {
    if (PLAN_VERIFICATION_ENABLED) setShowPlan(true);
    else void executeSave();
  }, [executeSave]);

  const requestSave = useCallback(() => {
    const result = AllSettingSchema.safeParse(allSetting);
    if (!result.success) {
      const issue = result.error.issues[0];
      const fieldPath = issue?.path.join('.') ?? 'value';
      const msgKey = issue?.message ?? 'somethingWentWrong';
      message.error(`${fieldPath}: ${t(msgKey, { defaultValue: msgKey })}`);
      return;
    }
    if (changedDangerFields.length > 0) {
      setShowDanger(true);
      return;
    }
    proceedSave();
  }, [allSetting, message, t, changedDangerFields, proceedSave]);

  const requestRestart = useCallback(async () => {
    setSpinning(true);
    try {
      const msg = await HttpUtil.post('/panel/setting/restartPanel') as ApiMsg;
      if (!msg?.success) return;
      const overlay = {
        title: t('pages.settings.restartingTitle'),
        subtitle: t('pages.settings.restartingDesc'),
      };
      busyOverlay.show(overlay);
      try { localStorage.setItem(BOOT_BUSY_KEY, JSON.stringify(overlay)); } catch { /* ignore */ }
      setRestartNeeded(false);
      await PromiseUtil.sleep(5000);
      const target = new URL(import.meta.env.DEV ? window.location.href : rebuildUrlAfterRestart());
      const cur = new URL(window.location.href);
      const sameDocument =
        target.origin === cur.origin
        && target.pathname === cur.pathname
        && target.search === cur.search;
      if (sameDocument) {
        if (target.hash !== cur.hash) window.location.hash = target.hash;
        window.location.reload();
      } else {
        window.location.replace(target.toString());
      }
    } finally {
      setSpinning(false);
    }
  }, [rebuildUrlAfterRestart, setSpinning, setRestartNeeded, busyOverlay, t]);

  useEffect(() => {
    if (saveDisabled) return undefined;
    const id = window.setTimeout(() => { requestSave(); }, 600);
    return () => window.clearTimeout(id);
  }, [saveDisabled, requestSave]);

  const value = useMemo<SettingsControllerValue>(() => ({
    allSetting,
    originalSetting,
    updateSetting,
    commitSetting,
    fetched,
    spinning,
    setSpinning,
    saveDisabled,
    dirty: !saveDisabled,
    restartNeeded,
    requestSave,
    requestRestart,
  }), [
    allSetting, originalSetting, updateSetting, commitSetting, fetched, spinning, setSpinning,
    saveDisabled, restartNeeded, requestSave, requestRestart,
  ]);

  return (
    <SettingsControllerContext.Provider value={value}>
      {children}

      <PlanVerificationModal
        open={showPlan}
        title="Settings Implementation Plan"
        original={originalSetting}
        modified={allSetting}
        confirmLoading={spinning}
        onConfirm={executeSave}
        onCancel={() => setShowPlan(false)}
      />

      <DangerConfirmModal
        open={showDanger}
        fields={changedDangerFields}
        onConfirm={() => { setShowDanger(false); proceedSave(); }}
        onCancel={() => setShowDanger(false)}
        onBackup={() => { window.location.href = (window.X_UI_BASE_PATH || '') + 'panel/api/server/getDb'; }}
      />
    </SettingsControllerContext.Provider>
  );
}
