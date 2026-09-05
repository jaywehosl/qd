import { createContext, useContext } from 'react';
import type { AllSetting } from '@/models/setting';

export interface SettingsControllerValue {
  allSetting: AllSetting;
  originalSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  commitSetting: (patch: Partial<AllSetting>) => Promise<{ success?: boolean; msg?: string } | undefined>;
  fetched: boolean;
  spinning: boolean;
  setSpinning: (v: boolean) => void;
  saveDisabled: boolean;
  dirty: boolean;
  restartNeeded: boolean;
  requestSave: () => void;
  requestRestart: () => void;
}

export const SettingsControllerContext = createContext<SettingsControllerValue | null>(null);

export function useSettingsController(): SettingsControllerValue {
  const ctx = useContext(SettingsControllerContext);
  if (!ctx) throw new Error('useSettingsController must be used within a SettingsControllerProvider');
  return ctx;
}
