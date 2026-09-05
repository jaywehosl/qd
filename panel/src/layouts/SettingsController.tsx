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

// The "Settings Implementation Plan" diff modal (PlanVerificationModal) is a
// bespoke frontend feature of ours (not from upstream 3x-ui). It's kept but
// OFF by default — Save applies directly. This flag will later be driven by a
// frontend-only settings store (planned: a layer of our own UI prefs on top of
// the backend settings). Flip to true to re-enable the pre-save diff review.
const PLAN_VERIFICATION_ENABLED = false;

// Nothing the panel exposes can lock the operator out any more — it serves no
// port of its own — so no save needs the danger-confirm gate.
const ACCESS_CRITICAL_FIELDS: { key: string; label: string }[] = [];

// "Restart panel" reminder must survive a full page reload (e.g. switching the
// language, which calls window.location.reload()). A save persists settings to
// the DB, but settings that need a panel restart (port/basePath/cert/listen…)
// only take effect after an actual /panel/setting/restartPanel — NOT a frontend
// reload. So we persist the pending-restart flag and clear it only on a real
// restart.
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

  // A fresh server payload (e.g. after save invalidates the query) means the
  // draft is back in sync — clear the dirty flag's restart prompt only when the
  // user resets, not here; restartNeeded persists until an actual restart.

  // The panel no longer serves itself on a configurable host/port/base path, so
  // a restart always comes back on the very same URL.
  const rebuildUrlAfterRestart = useCallback(
    (): string => window.location.href,
    [],
  );

  const executeSave = useCallback(async () => {
    setShowPlan(false);
    setSpinning(true);
    try {
      const msg = await saveAll();
      // Only flag a pending restart when the save actually succeeded. The
      // success/error toast itself is emitted by HttpUtil (non-silent POST).
      if (msg?.success) setRestartNeeded(true);
    } finally {
      setSpinning(false);
    }
  }, [saveAll, setSpinning, setRestartNeeded]);

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

  // Restart immediately — no confirm modal. A panel restart takes ~half a
  // second (along with the core); re-triggering it (e.g. after a language
  // switch already restarted) is harmless.
  const requestRestart = useCallback(async () => {
    setSpinning(true);
    try {
      const msg = await HttpUtil.post('/panel/setting/restartPanel') as ApiMsg;
      if (!msg?.success) return;
      // The restart is happening for real now → raise the full-screen takeover,
      // and clear the pending reminder so it doesn't linger after the reload.
      const overlay = {
        title: t('pages.settings.restartingTitle'),
        subtitle: t('pages.settings.restartingDesc'),
      };
      busyOverlay.show(overlay);
      // A panel restart reloads the whole frontend → persist a one-shot request
      // so the freshly-booted app keeps the frost up while it pre-renders,
      // instead of visibly assembling the UI on the fly.
      try { localStorage.setItem(BOOT_BUSY_KEY, JSON.stringify(overlay)); } catch { /* ignore */ }
      setRestartNeeded(false);
      await PromiseUtil.sleep(5000);
      // In prod the panel may come back on a different port/protocol/basePath
      // (per the just-saved settings) → rebuild the URL. In dev the Vite server
      // is fixed on http://localhost:5173 regardless of the backend's cert
      // settings, so rebuilding (which would pick https from webCertFile and
      // jump to /panel/settings) just breaks — reload the current URL as-is.
      const target = new URL(import.meta.env.DEV ? window.location.href : rebuildUrlAfterRestart());
      const cur = new URL(window.location.href);
      const sameDocument =
        target.origin === cur.origin
        && target.pathname === cur.pathname
        && target.search === cur.search;
      if (sameDocument) {
        // CRITICAL: location.replace() does NOT reload when the URL is identical
        // or differs only by #hash (e.g. restarting from /panel#groups). Align
        // the hash, then force a real reload — otherwise the busy overlay spins
        // forever because the page never reloads to clear it.
        if (target.hash !== cur.hash) window.location.hash = target.hash;
        window.location.reload();
      } else {
        window.location.replace(target.toString());
      }
    } finally {
      setSpinning(false);
    }
  }, [rebuildUrlAfterRestart, setSpinning, setRestartNeeded, busyOverlay, t]);

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
