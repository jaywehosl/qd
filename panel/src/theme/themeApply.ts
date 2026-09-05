/**
 * Theme override core — the spine of the customization system.
 *
 * A "theme" is a small object of overrides on top of the canonical design
 * tokens (frontend/src/styles/tokens.css). applyTheme() writes them into a
 * single <style id="uup-theme-overrides"> as `:root { --token: value }`, so the
 * whole DS repaints instantly (live preview is free). The light/dark base lives
 * in tokens.css and is selected by the mode class on <html>; this layer only
 * carries user overrides on top of the chosen base.
 *
 * Pure where possible: themeToCss() is a pure function (unit-tested); applyTheme()
 * is the thin DOM side-effect around it.
 */

export type ThemeMode = 'light' | 'dark' | 'ultra-dark';

export interface ThemeBackground {
  type?: 'aura' | 'color' | 'image';
  color?: string;
  assetId?: string;
  dim?: number;   // 0..1 darken overlay over a background image
  blur?: string;  // e.g. "0px" | "8px"
}

export interface ThemeFonts {
  sans?: string;
  display?: string;
  mono?: string;
}

export interface ThemeParticles {
  on?: boolean;
  density?: number;
  speed?: number; // maps to intensity in WebGL
  color?: 'primary' | 'monochrome' | 'palette';
  preset?: 'pucks' | 'neural' | 'nebula';
}

export interface ThemeEffects {
  particles?: ThemeParticles;
  hoverGlow?: boolean;
}

export interface PanelTheme {
  mode?: ThemeMode;
  /** Raw token overrides, e.g. { "--color-primary": "#3279f9" }. Written
   *  verbatim, EXCEPT --radius-scale which is expanded across the radius ramp.
   *  (--shadow-intensity is written raw — tokens.css multiplies the elevation
   *  alphas by it directly via calc(), in both light and dark.) */
  tokens?: Record<string, string | number>;
  background?: ThemeBackground;
  fonts?: ThemeFonts;
  effects?: ThemeEffects;
}

export const THEME_STYLE_ID = 'uup-theme-overrides';

/** Base px values of the radius ramp in tokens.css, scaled by --radius-scale. */
const RADIUS_RAMP: Record<string, number> = {
  '--radius-xs': 8,
  '--radius-sm': 12,
  '--radius-md': 16,
  '--radius-lg': 20,
  '--radius-xl': 24,
  '--radius-2xl': 28,
  '--radius-control': 14,
  '--radius-control-sm': 10,
};

const SYNTHETIC_KEYS = new Set(['--radius-scale']);

function normalizeKey(k: string): string {
  return k.startsWith('--') ? k : `--${k}`;
}

export function hexToRgb(hex: string): string | null {
  const clean = hex.trim().replace(/^#/, '');
  if (clean.length === 3) {
    const r = parseInt(clean[0] + clean[0], 16);
    const g = parseInt(clean[1] + clean[1], 16);
    const b = parseInt(clean[2] + clean[2], 16);
    if (!isNaN(r) && !isNaN(g) && !isNaN(b)) {
      return `${r}, ${g}, ${b}`;
    }
  } else if (clean.length === 6) {
    const r = parseInt(clean.substring(0, 2), 16);
    const g = parseInt(clean.substring(2, 4), 16);
    const b = parseInt(clean.substring(4, 6), 16);
    if (!isNaN(r) && !isNaN(g) && !isNaN(b)) {
      return `${r}, ${g}, ${b}`;
    }
  }
  return null;
}

/** A font value is either a CSS font-family list ("'Inter', sans-serif") or
 *  "asset:<id>" pointing at an uploaded font. The latter emits an @font-face. */
function resolveFont(raw: string | undefined, fallback: string, faces: string[]): string | undefined {
  if (!raw) return undefined;
  if (!raw.startsWith('asset:')) return raw;
  const id = raw.slice('asset:'.length);
  const family = `uup-font-${id.replace(/[^a-zA-Z0-9]/g, '')}`;
  faces.push(`@font-face{font-family:"${family}";font-display:swap;src:url("/theme/asset/${id}")}`);
  return `"${family}", ${fallback}`;
}

/** Build the CSS (optional @font-face rules + `:root { … }`) for a theme. Pure. */
export function themeToCss(theme: PanelTheme): string {
  const decls: string[] = [];
  const fontFaces: string[] = [];
  const push = (k: string, v: string | number) => decls.push(`${k}: ${v};`);

  if (theme.tokens) {
    const computedRgb: Record<string, string> = {};
    for (const [rawKey, value] of Object.entries(theme.tokens)) {
      const key = normalizeKey(rawKey);
      if (SYNTHETIC_KEYS.has(key)) continue;
      if (key === '--fx-particles' && theme.effects?.particles?.on !== undefined) continue;
      if (key === '--fx-hover-glow' && theme.effects?.hoverGlow !== undefined) continue;
      if (value === null || value === undefined || value === '') continue;
      push(key, value);

      // Automatically generate -rgb counterpart for hex colors
      if (key.startsWith('--color-') && !key.endsWith('-rgb') && typeof value === 'string') {
        const rgb = hexToRgb(value);
        if (rgb) {
          computedRgb[`${key}-rgb`] = rgb;
        }
      }
    }

    // Append generated -rgb tokens if not explicitly overridden
    for (const [rgbKey, rgbValue] of Object.entries(computedRgb)) {
      const explicitKey = Object.keys(theme.tokens).find(k => normalizeKey(k) === rgbKey);
      if (!explicitKey) {
        push(rgbKey, rgbValue);
      }
    }

    const scale = Number(theme.tokens['--radius-scale']);
    if (Number.isFinite(scale) && scale > 0) {
      for (const [key, base] of Object.entries(RADIUS_RAMP)) {
        push(key, `${Math.round(base * scale)}px`);
      }
    }
  }

  const sans = resolveFont(theme.fonts?.sans, 'sans-serif', fontFaces);
  if (sans) push('--font-sans', sans);
  const display = resolveFont(theme.fonts?.display, 'var(--font-sans)', fontFaces);
  if (display) push('--font-display', display);
  const mono = resolveFont(theme.fonts?.mono, 'monospace', fontFaces);
  if (mono) push('--font-mono', mono);

  const bg = theme.background;
  if (bg) {
    if (bg.type === 'color' && bg.color) {
      push('--bg-page', bg.color);
      push('--bg-page-2', bg.color);
    }
    if (bg.type === 'image' && bg.assetId) {
      push('--bg-image', `url("/theme/asset/${bg.assetId}")`);
    }
    if (typeof bg.dim === 'number') push('--bg-image-dim', String(bg.dim));
    if (bg.blur) push('--bg-image-blur', bg.blur);
  }

  const fx = theme.effects;
  if (fx) {
    if (fx.particles) {
      push('--fx-particles', fx.particles.on !== false ? 'on' : 'off');
      if (typeof fx.particles.density === 'number') push('--fx-particles-density', fx.particles.density);
      if (typeof fx.particles.speed === 'number') push('--fx-particles-speed', fx.particles.speed);
      if (fx.particles.color) push('--fx-particles-color', fx.particles.color);
      if (fx.particles.preset) push('--fx-particles-preset', fx.particles.preset);
    }
    if (fx.hoverGlow === false) {
      push('--hover-glow', 'none');
      push('--fx-hover-glow', 'off');
    } else if (fx.hoverGlow === true) {
      push('--fx-hover-glow', 'on');
    }
  }

  const root = decls.length ? `:root{${decls.join('')}}` : '';
  return fontFaces.join('') + root;
}

/** Select the light/dark/ultra-dark base by toggling the <html> mode classes
 *  (the same convention tokens.css and useTheme already use). */
export function applyThemeMode(mode: ThemeMode | undefined): void {
  if (typeof document === 'undefined' || !mode) return;
  const html = document.documentElement;
  const isDark = mode === 'dark' || mode === 'ultra-dark';
  const isUltra = mode === 'ultra-dark';
  html.classList.toggle('is-dark', isDark);
  html.classList.toggle('is-ultra', isUltra);

  if (isDark && isUltra) {
    html.setAttribute('data-theme', 'ultra-dark');
  } else {
    html.removeAttribute('data-theme');
  }

  document.body.classList.toggle('dark', isDark);
  document.body.classList.toggle('light', !isDark);
  const msg = document.getElementById('message');
  if (msg) msg.className = isDark ? 'dark' : 'light';

  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('uup-theme-mode-changed', { detail: mode }));
    const dress = (window as unknown as { qdWindowTheme?: (dark: boolean) => void }).qdWindowTheme;
    if (dress) void dress(isDark);
  }
}

/** Apply token/font/background overrides into the single managed <style>. The
 *  mode class is handled separately (applyThemeMode) so it can stay coordinated
 *  with the existing theme toggle. */
export function applyTheme(theme: PanelTheme): void {
  if (typeof document === 'undefined') return;
  let el = document.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!el) {
    el = document.createElement('style');
    el.id = THEME_STYLE_ID;
    document.head.appendChild(el);
  }
  el.textContent = themeToCss(theme);

  // Make the wrapper gradient transparent so a custom background image shows.
  const hasBgImage = theme.background?.type === 'image' && Boolean(theme.background.assetId);
  document.body?.classList.toggle('has-theme-bg', hasBgImage);

  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('uup-theme-changed', { detail: theme }));
  }
}

export const EMPTY_THEME: PanelTheme = {};

/** Community Panel factory theme — what the panel starts with on a fresh install
 *  (no stored/server theme) and what "Reset" restores. Only the knobs that differ
 *  from the tokens.css baseline are set; everything else falls through to the
 *  token defaults. Mirrored by the "Community Panel" preset in AppearancePage. */
export const DEFAULT_THEME: PanelTheme = {
  mode: 'light',
  tokens: {
    '--glass-blur': '60px',
    '--glass-saturate': '200%',
    '--radius-scale': 1.3,
    '--shadow-intensity': 1.0,
  },
};
