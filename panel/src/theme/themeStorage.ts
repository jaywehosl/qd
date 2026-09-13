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
  }
}

async function fetchServerTheme(): Promise<PanelTheme | null> {
  try {
    const res = await fetch(basePath() + 'theme.json', { cache: 'no-cache' });
    if (!res.ok) return null;
    const data: unknown = await res.json();
    return data && typeof data === 'object' && !Array.isArray(data) ? (data as PanelTheme) : null;
  } catch {
    return null;
  }
}

export async function saveTheme(theme: PanelTheme): Promise<boolean> {
  cacheLocal(theme);
  try {
    const msg = await HttpUtil.post('/panel/setting/theme', theme, {
      silent: true,
      headers: { 'Content-Type': 'application/json' },
      skipAuthRedirect: true,
    });
    return Boolean(msg && (msg as { success?: boolean }).success);
  } catch {
    return false;
  }
}

export function bootstrapTheme(): void {
  const injected = typeof window !== 'undefined' ? window.X_UI_THEME : undefined;
  const hasInjected = injected && Object.keys(injected).length > 0;
  const local = loadTheme();
  const hasLocal = Object.keys(local).length > 0;

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
    return;
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
