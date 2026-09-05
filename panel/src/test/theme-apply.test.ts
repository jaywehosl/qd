import { describe, it, expect } from 'vitest';
import { themeToCss, type PanelTheme } from '@/theme/themeApply';

describe('themeToCss', () => {
  it('returns empty string for an empty theme', () => {
    expect(themeToCss({})).toBe('');
  });

  it('writes raw token overrides under :root, normalizing bare keys', () => {
    const css = themeToCss({ tokens: { '--color-primary': '#abc', 'glass-blur': '40px' } });
    expect(css).toContain(':root{');
    expect(css).toContain('--color-primary: #abc;');
    expect(css).toContain('--glass-blur: 40px;');
  });

  it('skips null/undefined/empty values', () => {
    const theme: PanelTheme = { tokens: { '--a': '', '--b': '1px' } };
    const css = themeToCss(theme);
    expect(css).not.toContain('--a:');
    expect(css).toContain('--b: 1px;');
  });

  it('expands --radius-scale across the radius ramp instead of writing it raw', () => {
    const css = themeToCss({ tokens: { '--radius-scale': 1.5 } });
    expect(css).not.toContain('--radius-scale');
    expect(css).toContain('--radius-xs: 12px;');
    expect(css).toContain('--radius-md: 24px;');
    expect(css).toContain('--radius-2xl: 42px;');
  });

  it('ignores a non-positive radius scale', () => {
    expect(themeToCss({ tokens: { '--radius-scale': 0 } })).toBe('');
    expect(themeToCss({ tokens: { '--radius-scale': -1 } })).toBe('');
  });

  it('maps fonts to the family tokens', () => {
    const css = themeToCss({ fonts: { sans: 'Inter', display: 'Outfit', mono: 'Fira Code' } });
    expect(css).toContain('--font-sans: Inter;');
    expect(css).toContain('--font-display: Outfit;');
    expect(css).toContain('--font-mono: Fira Code;');
  });

  it('handles color and image backgrounds', () => {
    expect(themeToCss({ background: { type: 'color', color: '#101010' } }))
      .toContain('--bg-page: #101010;');
    const img = themeToCss({ background: { type: 'image', assetId: 'bg_x', dim: 0.4, blur: '6px' } });
    expect(img).toContain('--bg-image: url("/theme/asset/bg_x");');
    expect(img).toContain('--bg-image-dim: 0.4;');
    expect(img).toContain('--bg-image-blur: 6px;');
  });

  it('emits @font-face for an uploaded (asset:) font and points the token at it', () => {
    const css = themeToCss({ fonts: { sans: 'asset:abc123.woff2' } });
    expect(css).toContain('@font-face{');
    expect(css).toContain('src:url("/theme/asset/abc123.woff2")');
    expect(css).toContain('--font-sans: "uup-font-abc123woff2", sans-serif;');
  });
});
