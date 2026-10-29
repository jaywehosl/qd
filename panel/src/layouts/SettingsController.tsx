import { useCallback, useEffect, useMemo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { getMessage } from '@/utils/messageBus';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { AllSettingSchema } from '@/schemas/setting';
import { SettingsControllerContext, type SettingsControllerValue } from '@/layouts/settings-controller-context';

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

  const executeSave = useCallback(async () => {
    setSpinning(true);
    try {
      await saveAll();
    } finally {
      setSpinning(false);
    }
  }, [saveAll, setSpinning]);

  const requestSave = useCallback(() => {
    const result = AllSettingSchema.safeParse(allSetting);
    if (!result.success) {
      const issue = result.error.issues[0];
      const fieldPath = issue?.path.join('.') ?? 'value';
      const msgKey = issue?.message ?? 'somethingWentWrong';
      message.error(`${fieldPath}: ${t(msgKey, { defaultValue: msgKey })}`);
      return;
    }
    void executeSave();
  }, [allSetting, message, t, executeSave]);

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
    </SettingsControllerContext.Provider>
  );
}
