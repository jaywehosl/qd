/**
 * Theme persistence — server-canonical with a localStorage fallback.
 *
 * Canonical store is the server (`GET /theme.json`, `POST /panel/setting/theme`).
 * localStorage is kept as a fast cache (no-flash on boot) AND as a graceful
 * fallback when the backend doesn't expose the endpoints yet (e.g. dev proxying
 * to an older panel) — so the Appearance page stays usable everywhere.
 */
import { applyTheme, applyThemeMode, DEFAULT_THEME, type PanelTheme } from './themeApply';
import { HttpUtil } from '@/utils';

const STORAGE_KEY = 'uup.panelTheme';
const basePath = () => (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';

export function loadTheme(): PanelTheme {
  if (typeof localStorage === 'undefined') return {};
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as PanelTheme) : {};
  } catch {
    return {};
  }
}

function cacheLocal(theme: PanelTheme): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(theme));
  } catch {
    /* ignore quota / disabled storage */
  }
}

export function clearTheme(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

/** Read the server theme. Returns null when unreachable / not a theme object. */
export async function fetchServerTheme(): Promise<PanelTheme | null> {
  try {
    const res = await fetch(basePath() + 'theme.json', { cache: 'no-cache' });
    if (!res.ok) return null;
    const data: unknown = await res.json();
    return data && typeof data === 'object' && !Array.isArray(data) ? (data as PanelTheme) : null;
  } catch {
    return null;
  }
}

/** Persist a theme: cache locally immediately, then push to the server. Returns
 *  true when the server accepted it (false → kept locally only). */
export async function saveTheme(theme: PanelTheme): Promise<boolean> {
  cacheLocal(theme);
  try {
    // MUST send as JSON: the axios interceptor qs.stringify()s any non-JSON
    // body into form-urlencoded, but the server reads this endpoint as raw JSON
    // (ShouldBindBodyWith(binding.JSON)). Without this header the nested theme
    // object arrived as `mode=…&radius=…`, failed JSON validation, and the save
    // silently fell back to localStorage-only (panelTheme stayed "{}").
    const msg = await HttpUtil.post('/panel/setting/theme', theme, {
      silent: true,
      headers: { 'Content-Type': 'application/json' },
      // On the login screen the user isn't authenticated yet, so this 401s —
      // that must NOT trigger the global session-expired page reload (it caused
      // the login "flash/reload on theme switch"). The theme still applies +
      // caches locally; it persists to the server once they're logged in.
      skipAuthRedirect: true,
    });
    return Boolean(msg && (msg as { success?: boolean }).success);
  } catch {
    return false;
  }
}

/** Upload a custom background image / font; returns its asset id (referenced
 *  from the theme and served via /theme/asset/<id>), or null on failure. */
export async function uploadThemeAsset(kind: 'image' | 'font', file: File): Promise<string | null> {
  const fd = new FormData();
  fd.append('kind', kind);
  fd.append('file', file);
  try {
    const msg = await HttpUtil.post('/panel/setting/theme/asset', fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
      silent: true,
    });
    const obj = (msg as { obj?: { assetId?: string } } | undefined)?.obj;
    return obj?.assetId ?? null;
  } catch {
    return null;
  }
}

/** Apply the theme at boot with no flash. Prefers window.X_UI_THEME — the
 *  server copy the Go handler inlines into the HTML before any JS runs — then
 *  the local cache, then (only if neither) a server fetch. */
export function bootstrapTheme(): void {
  const injected = typeof window !== 'undefined' ? window.X_UI_THEME : undefined;
  const hasInjected = injected && Object.keys(injected).length > 0;
  const local = loadTheme();
  const hasLocal = Object.keys(local).length > 0;

  // Fresh install (no server-injected and no cached theme) → Community Panel
  // factory defaults instead of the bare token baseline.
  const theme = hasInjected ? { ...injected } : (hasLocal ? { ...local } : { ...DEFAULT_THEME });
  if (local.mode) {
    theme.mode = local.mode;
  }

  applyTheme(theme);
  if (theme.mode) {
    applyThemeMode(theme.mode);
  }

  if (hasInjected) {
    cacheLocal(theme);
    return; // the inlined theme IS the server copy — no fetch needed
  }

  void fetchServerTheme().then((srv) => {
    if (srv && Object.keys(srv).length) {
      const mergedSrv = { ...srv };
      if (local.mode) {
        mergedSrv.mode = local.mode;
      }
      cacheLocal(mergedSrv);
      applyTheme(mergedSrv);
      if (mergedSrv.mode) applyThemeMode(mergedSrv.mode);
    }
  });
}
