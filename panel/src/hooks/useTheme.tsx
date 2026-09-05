import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { flushSync } from 'react-dom';
import type { ReactNode } from 'react';
import { loadTheme, saveTheme } from '@/theme/themeStorage';
import { applyThemeMode, type ThemeMode } from '@/theme/themeApply';

const STORAGE_DARK = 'dark-mode';
const STORAGE_ULTRA = 'isUltraDarkThemeEnabled';

function applyDom(isDark: boolean) {
  const root = document.documentElement;
  root.classList.toggle('is-dark', isDark);
  root.classList.remove('is-ultra');
  root.removeAttribute('data-theme');
  document.body.classList.toggle('dark', isDark);
  document.body.classList.toggle('light', !isDark);
  const msg = document.getElementById('message');
  if (msg) msg.className = isDark ? 'dark' : 'light';
}

const CACHE_RESET_KEY = 'antigravity-reset-v3';
if (localStorage.getItem(CACHE_RESET_KEY) !== 'true') {
  localStorage.setItem(STORAGE_DARK, 'false');
  localStorage.setItem(STORAGE_ULTRA, 'false');
  localStorage.setItem(CACHE_RESET_KEY, 'true');
}

const getInitialMode = () => {
  if (typeof window === 'undefined') return { dark: false, ultra: false };
  const injected = (window as any).X_UI_THEME;
  const local = loadTheme();
  const mode = local.mode ?? injected?.mode ?? 'light';
  return {
    dark: mode === 'dark' || mode === 'ultra-dark',
    ultra: mode === 'ultra-dark',
  };
};

const initialMode = getInitialMode();
applyDom(initialMode.dark);

function crossfade(swap: () => void): void {
  const start = document.startViewTransition?.bind(document);
  if (!start) {
    swap();
    return;
  }
  const root = document.documentElement;
  root.setAttribute('data-vt', 'theme');
  const run = start(() => flushSync(swap));
  void run.finished.finally(() => root.removeAttribute('data-vt'));
}

export function pauseAnimationsUntilLeave(elementId: string): void {
  document.documentElement.setAttribute('data-theme-animations', 'off');
  const el = document.getElementById(elementId);
  if (!el) return;
  const restore = () => {
    document.documentElement.removeAttribute('data-theme-animations');
    el.removeEventListener('mouseleave', restore);
    el.removeEventListener('touchend', restore);
  };
  el.addEventListener('mouseleave', restore);
  el.addEventListener('touchend', restore);
}

interface ThemeContextValue {
  isDark: boolean;
  isUltra: boolean;
  toggleTheme: () => void;
  toggleUltra: () => void;
  cycleTheme: (elementId?: string) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [isDark, setIsDark] = useState<boolean>(initialMode.dark);
  const [isUltra, setIsUltra] = useState<boolean>(initialMode.ultra);

  useEffect(() => {
    applyDom(isDark);
    localStorage.setItem(STORAGE_DARK, String(isDark));
    localStorage.setItem(STORAGE_ULTRA, String(isUltra));
  }, [isDark, isUltra]);

  useEffect(() => {
    const handleModeChange = (e: Event) => {
      const mode = (e as CustomEvent<ThemeMode>).detail;
      setIsDark(mode === 'dark' || mode === 'ultra-dark');
      setIsUltra(mode === 'ultra-dark');
    };
    window.addEventListener('uup-theme-mode-changed', handleModeChange);
    return () => window.removeEventListener('uup-theme-mode-changed', handleModeChange);
  }, []);

  const toggleTheme = useCallback(() => {
    crossfade(() => setIsDark((prevDark) => {
      const nextDark = !prevDark;
      setIsUltra(false);
      
      const theme = loadTheme();
      const nextMode: ThemeMode = nextDark ? 'dark' : 'light';
      theme.mode = nextMode;
      
      applyThemeMode(nextMode);
      void saveTheme(theme);
      
      return nextDark;
    }));
  }, []);

  const toggleUltra = useCallback(() => {
    crossfade(() => setIsUltra((prevUltra) => {
      const nextUltra = !prevUltra;
      setIsDark(true);
      
      const theme = loadTheme();
      const nextMode: ThemeMode = nextUltra ? 'ultra-dark' : 'dark';
      theme.mode = nextMode;
      
      applyThemeMode(nextMode);
      void saveTheme(theme);
      
      return nextUltra;
    }));
  }, []);

  const cycleTheme = useCallback((elementId?: string) => {
    if (elementId) {
      pauseAnimationsUntilLeave(elementId);
    }

    const nextMode: ThemeMode = isDark ? 'light' : 'dark';

    crossfade(() => {
      setIsDark(nextMode === 'dark');
      setIsUltra(false);

      const theme = loadTheme();
      theme.mode = nextMode;
      applyThemeMode(nextMode);
      void saveTheme(theme);
    });
  }, [isDark, isUltra]);

  const value = useMemo<ThemeContextValue>(
    () => ({ isDark, isUltra, toggleTheme, toggleUltra, cycleTheme }),
    [isDark, isUltra, toggleTheme, toggleUltra, cycleTheme],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}
