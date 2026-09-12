import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import PlanVerificationModal from '@/components/ui/PlanVerificationModal';
import DangerConfirmModal from '@/components/ui/DangerConfirmModal';
import { getMessage } from '@/utils/messageBus';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { AllSettingSchema } from '@/schemas/setting';
import { SettingsControllerContext, type SettingsControllerValue } from '@/layouts/settings-controller-context';

// The "Settings Implementation Plan" diff modal (PlanVerificationModal) is a
// bespoke frontend feature of ours (not from upstream 3x-ui). It's kept but
// OFF by default — Save applies directly. This flag will later be driven by a
// frontend-only settings store (planned: a layer of our own UI prefs on top of
// the backend settings). Flip to true to re-enable the pre-save diff review.
const PLAN_VERIFICATION_ENABLED = false;

// Nothing the panel exposes can lock the operator out any more — it serves no
// port of its own — so no save needs the danger-confirm gate.
const ACCESS_CRITICAL_FIELDS: { key: string; label: string }[] = [];

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

  const [showPlan, setShowPlan] = useState(false);
  const [showDanger, setShowDanger] = useState(false);

  const executeSave = useCallback(async () => {
    setShowPlan(false);
    setSpinning(true);
    try {
      await saveAll();
    } finally {
      setSpinning(false);
    }
  }, [saveAll, setSpinning]);

  // Access-critical fields whose draft value differs from the saved server value.
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
    // A Save that touches port/path/domain/cert (panel or sub) must clear the
    // hard confirmation gate first — those can lock the operator out behind the
    // reverse proxy.
    if (changedDangerFields.length > 0) {
      setShowDanger(true);
      return;
    }
    proceedSave();
  }, [allSetting, message, t, changedDangerFields, proceedSave]);

  // Panel preferences never leave this machine, so they persist as soon as they
  // settle. The header belongs to the network draft — the state that has to be
  // handed to the nodes deliberately.
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
    requestSave,
  }), [
    allSetting, originalSetting, updateSetting, commitSetting, fetched, spinning, setSpinning,
    saveDisabled, requestSave,
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
