import { createContext, useContext } from 'react';
import type { AllSetting } from '@/models/setting';

export interface SettingsControllerValue {
  allSetting: AllSetting;
  originalSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  /** persist a patch immediately (merge + POST), for discrete actions like 2FA */
  commitSetting: (patch: Partial<AllSetting>) => Promise<{ success?: boolean; msg?: string } | undefined>;
  fetched: boolean;
  spinning: boolean;
  setSpinning: (v: boolean) => void;
  /** true when the draft equals the server (nothing to save) */
  saveDisabled: boolean;
  /** convenience: there are unsaved edits */
  dirty: boolean;
  /** a save just succeeded → a panel restart is appropriate to apply it */
  restartNeeded: boolean;
  /** validate the draft, then save (or open the diff modal if enabled) */
  requestSave: () => void;
  /** restart the panel immediately (no confirm) */
  requestRestart: () => void;
}

// Kept in a component-free module on purpose: the context object must keep a
// stable identity across Fast Refresh / HMR patches. If this lived in the
// provider's .tsx (which also exports a component), editing that file could
// hand mounted consumers a stale/!== context reference → the spurious
// "useSettingsController must be used within a SettingsControllerProvider".
export const SettingsControllerContext = createContext<SettingsControllerValue | null>(null);

export function useSettingsController(): SettingsControllerValue {
  const ctx = useContext(SettingsControllerContext);
  if (!ctx) throw new Error('useSettingsController must be used within a SettingsControllerProvider');
  return ctx;
}
