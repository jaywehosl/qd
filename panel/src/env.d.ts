/// <reference types="vite/client" />

interface SubPageData {
  sId?: string;
  enabled?: boolean;
  download?: string;
  upload?: string;
  total?: string;
  used?: string;
  remained?: string;
  totalByte?: string | number;
  expire?: string | number;
  lastOnline?: string | number;
  subUrl?: string;
  subJsonUrl?: string;
  subClashUrl?: string;
  subTitle?: string;
  links?: string[];
  emails?: string[];
  datepicker?: 'gregorian' | 'jalalian';
  downloadByte?: string | number;
  uploadByte?: string | number;
  usedByte?: string | number;
}

interface Window {
  X_UI_BASE_PATH?: string;
  QD_TOKEN?: string;
  X_UI_CUR_VER?: string;
  X_UI_DB_TYPE?: string;
  X_UI_THEME?: import('@/theme/themeApply').PanelTheme;
  __SUB_PAGE_DATA__?: SubPageData;
}

declare module 'qs' {
  interface StringifyOptions {
    arrayFormat?: 'indices' | 'brackets' | 'repeat' | 'comma';
    encode?: boolean;
    encoder?: (str: unknown, defaultEncoder: (s: unknown) => string, charset: string, type: 'key' | 'value') => string;
    allowDots?: boolean;
    skipNulls?: boolean;
    addQueryPrefix?: boolean;
  }
  interface ParseOptions {
    depth?: number;
    arrayLimit?: number;
    allowDots?: boolean;
    parseArrays?: boolean;
    ignoreQueryPrefix?: boolean;
  }
  export function stringify(obj: unknown, options?: StringifyOptions): string;
  export function parse(str: string, options?: ParseOptions): Record<string, unknown>;
  const qs: { stringify: typeof stringify; parse: typeof parse };
  export default qs;
}

