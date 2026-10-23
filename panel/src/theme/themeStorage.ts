import { applyTheme, applyThemeMode, DEFAULT_THEME, type PanelTheme } from './themeApply';
import { HttpUtil } from '@/utils';

const STORAGE_KEY = 'uup.panelTheme';

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
    const res = await fetch('/theme.json', { cache: 'no-cache' });
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
    });
    return Boolean(msg && (msg as { success?: boolean }).success);
  } catch {
    return false;
  }
}

export function bootstrapTheme(): void {
  const local = loadTheme();
  const hasLocal = Object.keys(local).length > 0;

  const theme = hasLocal ? { ...local } : { ...DEFAULT_THEME };
  if (local.mode) {
    theme.mode = local.mode;
  }

  applyTheme(theme);
  if (theme.mode) {
    applyThemeMode(theme.mode);
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
