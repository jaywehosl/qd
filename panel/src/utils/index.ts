import axios from 'axios';
import type { AxiosError, AxiosRequestConfig, AxiosResponse } from 'axios';
import i18next from 'i18next';
import { getMessage } from './messageBus';

type RespEnvelope = { success?: unknown; msg?: unknown; obj?: unknown };

export class Msg<T = unknown> {
  success: boolean;
  msg: string;
  obj: T | null;

  constructor(success: boolean = false, msg: string = '', obj: T | null = null) {
    this.success = success;
    this.msg = msg;
    this.obj = obj;
  }
}

export interface HttpOptions extends AxiosRequestConfig {
  silent?: boolean;
  skipAuthRedirect?: boolean;
}

export interface HttpModal {
  loading: (state: boolean) => void;
  close: () => void;
}

export class HttpUtil {
  static _handleMsg(msg: unknown): void {
    if (!(msg instanceof Msg) || msg.msg === '') {
      return;
    }
    const messageType = msg.success ? 'success' : 'error';
    getMessage()[messageType](msg.msg);
    if (
      msg.success &&
      msg.obj &&
      typeof msg.obj === 'object' &&
      (msg.obj as { nodePending?: unknown }).nodePending === true
    ) {
      getMessage().warning(i18next.t('pages.inbounds.toasts.savedNodeOfflineWillSync'));
    }
  }

  static _respToMsg(resp: AxiosResponse | undefined): Msg {
    if (!resp || !resp.data) {
      return new Msg(false, 'No response data');
    }
    const { data } = resp;
    if (data == null) {
      return new Msg(true);
    }
    if (typeof data === 'object' && 'success' in (data as object)) {
      const d = data as RespEnvelope;
      return new Msg(Boolean(d.success), typeof d.msg === 'string' ? d.msg : '', d.obj ?? null);
    }
    return typeof data === 'object' ? (data as Msg) : new Msg(false, 'unknown data:', data);
  }

  static async get<T = unknown>(url: string, params?: unknown, options: HttpOptions = {}): Promise<Msg<T>> {
    const { silent, ...axiosOpts } = options;
    try {
      const resp = await axios.get(url, { params, ...axiosOpts });
      const msg = this._respToMsg(resp) as Msg<T>;
      if (!silent) this._handleMsg(msg);
      return msg;
    } catch (error) {
      console.error('GET request failed:', error);
      const err = error as AxiosError<{ message?: string }>;
      const errorMsg = new Msg<T>(false, err.response?.data?.message || err.message || 'Request failed');
      if (!silent) this._handleMsg(errorMsg);
      return errorMsg;
    }
  }

  static async post<T = unknown>(url: string, data?: unknown, options: HttpOptions = {}): Promise<Msg<T>> {
    const { silent, ...axiosOpts } = options;
    try {
      const resp = await axios.post(url, data, axiosOpts);
      const msg = this._respToMsg(resp) as Msg<T>;
      if (!silent) this._handleMsg(msg);
      return msg;
    } catch (error) {
      console.error('POST request failed:', error);
      const err = error as AxiosError<{ message?: string }>;
      const errorMsg = new Msg<T>(false, err.response?.data?.message || err.message || 'Request failed');
      if (!silent) this._handleMsg(errorMsg);
      return errorMsg;
    }
  }

  static async postWithModal<T = unknown>(url: string, data?: unknown, modal?: HttpModal | null): Promise<Msg<T>> {
    if (modal) {
      modal.loading(true);
    }
    const msg = await this.post<T>(url, data);
    if (modal) {
      modal.loading(false);
      if (msg instanceof Msg && msg.success) {
        modal.close();
      }
    }
    return msg;
  }
}

export class PromiseUtil {
  static async sleep(timeout: number): Promise<void> {
    await new Promise<void>((resolve) => {
      setTimeout(resolve, timeout);
    });
  }
}

export interface RandomSeqOptions {
  type?: 'default' | 'hex';
  hasNumbers?: boolean;
  hasLowercase?: boolean;
  hasUppercase?: boolean;
}

export class RandomUtil {
  static getSeq({ type = 'default', hasNumbers = true, hasLowercase = true, hasUppercase = true }: RandomSeqOptions = {}): string {
    let seq = '';

    switch (type) {
      case 'hex':
        seq += '0123456789abcdef';
        break;
      default:
        if (hasNumbers) seq += '0123456789';
        if (hasLowercase) seq += 'abcdefghijklmnopqrstuvwxyz';
        if (hasUppercase) seq += 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
        break;
    }

    return seq;
  }

  static randomInteger(min: number, max: number): number {
    const range = max - min + 1;
    const randomBuffer = new Uint32Array(1);
    window.crypto.getRandomValues(randomBuffer);
    return Math.floor((randomBuffer[0] / (0xFFFFFFFF + 1)) * range) + min;
  }

  static randomSeq(count: number, options: RandomSeqOptions = {}): string {
    const seq = this.getSeq(options);
    const seqLength = seq.length;
    const randomValues = new Uint32Array(count);
    window.crypto.getRandomValues(randomValues);
    return Array.from(randomValues, (v) => seq[v % seqLength]).join('');
  }

  static randomLowerAndNum(len: number): string {
    return this.randomSeq(len, { hasUppercase: false });
  }

  static randomUUID(): string {
    if (window.location.protocol === 'https:') {
      return window.crypto.randomUUID();
    }
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const randomValues = new Uint8Array(1);
      window.crypto.getRandomValues(randomValues);
      const randomValue = randomValues[0] % 16;
      const calculatedValue = c === 'x' ? randomValue : (randomValue & 0x3) | 0x8;
      return calculatedValue.toString(16);
    });
  }

}

type AnyRecord = Record<string, unknown>;

export class ObjectUtil {
  static getPropIgnoreCase(obj: AnyRecord, prop: string): unknown {
    for (const name in obj) {
      if (!Object.prototype.hasOwnProperty.call(obj, name)) continue;
      if (name.toLowerCase() === prop.toLowerCase()) {
        return obj[name];
      }
    }
    return undefined;
  }

  static deepSearch(obj: unknown, key: string): boolean {
    if (obj instanceof Array) {
      for (let i = 0; i < obj.length; ++i) {
        if (this.deepSearch(obj[i], key)) return true;
      }
    } else if (obj instanceof Object) {
      const rec = obj as AnyRecord;
      for (const name in rec) {
        if (!Object.prototype.hasOwnProperty.call(rec, name)) continue;
        if (this.deepSearch(rec[name], key)) return true;
      }
    } else {
      return this.isEmpty(obj) ? false : String(obj).toLowerCase().indexOf(key.toLowerCase()) >= 0;
    }
    return false;
  }

  static isEmpty(obj: unknown): boolean {
    return obj === null || obj === undefined || obj === '';
  }

  static isArrEmpty(arr: unknown): boolean {
    return !Array.isArray(arr) || arr.length === 0;
  }

  static copyArr<T>(dest: T[], src: T[]): void {
    dest.splice(0);
    for (const item of src) {
      dest.push(item);
    }
  }

  static clone<T>(obj: T): T {
    if (obj instanceof Array) {
      const newArr: unknown[] = [];
      this.copyArr(newArr, obj);
      return newArr as unknown as T;
    }
    if (obj instanceof Object) {
      const newObj: AnyRecord = {};
      const rec = obj as unknown as AnyRecord;
      for (const key of Object.keys(rec)) {
        newObj[key] = rec[key];
      }
      return newObj as unknown as T;
    }
    return obj;
  }

  static deepClone<T>(obj: T): T {
    if (obj instanceof Array) {
      const newArr: unknown[] = [];
      for (const item of obj) {
        newArr.push(this.deepClone(item));
      }
      return newArr as unknown as T;
    }
    if (obj instanceof Object) {
      const newObj: AnyRecord = {};
      const rec = obj as unknown as AnyRecord;
      for (const key of Object.keys(rec)) {
        newObj[key] = this.deepClone(rec[key]);
      }
      return newObj as unknown as T;
    }
    return obj;
  }

  static cloneProps(dest: object, src: object, ...ignoreProps: string[]): void {
    if (dest == null || src == null) return;
    const ignoreEmpty = this.isArrEmpty(ignoreProps);
    const d = dest as AnyRecord;
    const s = src as AnyRecord;
    for (const key of Object.keys(s)) {
      if (!Object.prototype.hasOwnProperty.call(s, key)) continue;
      if (!Object.prototype.hasOwnProperty.call(d, key)) continue;
      if (s[key] === undefined) continue;
      if (ignoreEmpty) {
        d[key] = s[key];
      } else {
        let ignore = false;
        for (let i = 0; i < ignoreProps.length; ++i) {
          if (key === ignoreProps[i]) {
            ignore = true;
            break;
          }
        }
        if (!ignore) {
          d[key] = s[key];
        }
      }
    }
  }

  static delProps(obj: object, ...props: string[]): void {
    const o = obj as AnyRecord;
    for (const prop of props) {
      if (prop in o) {
        delete o[prop];
      }
    }
  }

  static execute(func: unknown, ...args: unknown[]): void {
    if (!this.isEmpty(func) && typeof func === 'function') {
      (func as (...a: unknown[]) => unknown)(...args);
    }
  }

  static orDefault<T>(obj: T | null | undefined, defaultValue: T): T {
    if (obj == null) return defaultValue;
    return obj;
  }

  static equals(a: unknown, b: unknown): boolean {
    if (a == null || b == null || typeof a !== 'object' || typeof b !== 'object') {
      return a === b;
    }
    const ra = a as AnyRecord;
    const rb = b as AnyRecord;
    const aKeys = Object.keys(ra);
    const bKeys = Object.keys(rb);
    if (aKeys.length !== bKeys.length) return false;
    for (const key of aKeys) {
      if (!Object.prototype.hasOwnProperty.call(rb, key)) return false;
      if (ra[key] !== rb[key]) return false;
    }
    return true;
  }
}

export class ClipboardManager {
  static async copyText(content: unknown = ''): Promise<boolean> {
    const text = String(content ?? '');
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(text);
        return true;
      } catch {}
    }
    return ClipboardManager._legacyCopy(text);
  }

  static _legacyCopy(text: string): boolean {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.setAttribute('readonly', '');
    textarea.setAttribute('aria-hidden', 'true');
    textarea.style.position = 'absolute';
    textarea.style.left = '-9999px';
    textarea.style.top = '0';
    textarea.style.opacity = '1';

    const active = document.activeElement as HTMLElement | null;
    const host = (active && active !== document.body && active.parentElement)
      ? active.parentElement
      : document.body;
    host.appendChild(textarea);

    const sel0 = document.getSelection();
    const prevSelection = sel0 && sel0.rangeCount ? sel0.getRangeAt(0) : null;

    let ok = false;
    try {
      textarea.focus({ preventScroll: true });
      textarea.select();
      textarea.setSelectionRange(0, text.length);
      const exec = (document as unknown as Record<string, unknown>)['execCommand'];
      if (typeof exec === 'function') {
        ok = (exec as (cmd: string) => boolean).call(document, 'copy');
      }
    } catch {}

    host.removeChild(textarea);
    if (active && typeof active.focus === 'function') {
      try { active.focus({ preventScroll: true }); } catch {}
    }
    if (prevSelection) {
      const sel = document.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(prevSelection);
    }
    return ok;
  }
}

export class SizeFormatter {
  static readonly ONE_KB = 1024;
  static readonly ONE_MB = SizeFormatter.ONE_KB * 1024;
  static readonly ONE_GB = SizeFormatter.ONE_MB * 1024;
  static readonly ONE_TB = SizeFormatter.ONE_GB * 1024;
  static readonly ONE_PB = SizeFormatter.ONE_TB * 1024;

  static sizeFormat(size: number | null | undefined): string {
    if (size == null || size <= 0) return '0 B';
    if (size < SizeFormatter.ONE_KB) return size.toFixed(0) + ' B';
    if (size < SizeFormatter.ONE_MB) return (size / SizeFormatter.ONE_KB).toFixed(2) + ' KB';
    if (size < SizeFormatter.ONE_GB) return (size / SizeFormatter.ONE_MB).toFixed(2) + ' MB';
    if (size < SizeFormatter.ONE_TB) return (size / SizeFormatter.ONE_GB).toFixed(2) + ' GB';
    if (size < SizeFormatter.ONE_PB) return (size / SizeFormatter.ONE_TB).toFixed(2) + ' TB';
    return (size / SizeFormatter.ONE_PB).toFixed(2) + ' PB';
  }
}

export class NumberFormatter {
  static addZero(num: number): string | number {
    return num < 10 ? '0' + num : num;
  }

  static toFixed(num: number, n: number): number {
    const m = Math.pow(10, n);
    return Math.floor(num * m) / m;
  }
}

const COLORS = {
  success: '#389e0a',
  warning: '#faad14',
  danger: '#ff4d4f',
  purple: '#722ed1',
} as const;

export type UsageColor = 'purple' | 'green' | 'orange' | 'red';

export interface ClientUsageStats {
  total: number;
  up: number;
  down: number;
}

export interface ExpiryClient {
  enable: boolean;
  expiryTime: number | null;
}

export class ColorUtils {
  static usageColor(
    data: number | null | undefined,
    threshold: number,
    total: number | { valueOf(): number } | null | undefined,
  ): UsageColor {
    const t = Number(total ?? 0);
    const d = Number(data);
    switch (true) {
      case data === null || data === undefined: return 'purple';
      case t < 0: return 'green';
      case t == 0: return 'purple';
      case d < t - threshold: return 'green';
      case d < t: return 'orange';
      default: return 'red';
    }
  }

  static clientUsageColor(clientStats: ClientUsageStats | null | undefined, trafficDiff: number): string {
    switch (true) {
      case !clientStats || clientStats.total == 0: return COLORS.purple;
      case clientStats!.up + clientStats!.down < clientStats!.total - trafficDiff: return COLORS.success;
      case clientStats!.up + clientStats!.down < clientStats!.total: return COLORS.warning;
      default: return COLORS.danger;
    }
  }

  static userExpiryColor(threshold: number, client: ExpiryClient, isDark: boolean = false): string {
    if (!client.enable) return isDark ? '#2c3950' : '#bcbcbc';
    const now = new Date().getTime();
    const expiry = client.expiryTime;
    switch (true) {
      case expiry === null: return COLORS.purple;
      case (expiry as number) < 0: return COLORS.success;
      case (expiry as number) == 0: return COLORS.purple;
      case now < (expiry as number) - threshold: return COLORS.success;
      case now < (expiry as number): return COLORS.warning;
      default: return COLORS.danger;
    }
  }
}

export interface SupportedLanguage {
  name: string;
  value: string;
  icon: string;
}

export class LanguageManager {
  static readonly supportedLanguages: readonly SupportedLanguage[] = [
    { name: 'English', value: 'en-US', icon: '🇺🇸' },
  ];

  static getLanguage(): string {
    return 'en-US';
  }
}

export class FileManager {
  static downloadTextFile(content: BlobPart, filename: string = 'file.txt', options: BlobPropertyBag = { type: 'text/plain' }): void {
    const link = window.document.createElement('a');
    link.download = filename;
    link.style.border = '0';
    link.style.padding = '0';
    link.style.margin = '0';
    link.style.position = 'absolute';
    link.style.left = '-9999px';
    link.style.top = `${window.pageYOffset || window.document.documentElement.scrollTop}px`;
    link.href = URL.createObjectURL(new Blob([content], options));
    link.click();
    URL.revokeObjectURL(link.href);
    link.remove();
  }
}

export type CalendarKind = 'gregorian' | 'jalalian';

export class IntlUtil {
  static formatDate(date: string | number | Date | null | undefined, calendar: CalendarKind = 'gregorian'): string {
    if (date == null) return '';
    const language = LanguageManager.getLanguage();
    const locale = calendar === 'jalalian' ? 'fa-IR' : language;

    const intlOptions: Intl.DateTimeFormatOptions = {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    };

    const intl = new Intl.DateTimeFormat(locale, intlOptions);
    return intl.format(new Date(date));
  }

  static formatRelativeTime(date: number | null | undefined): string {
    if (date == null) return '';
    const language = LanguageManager.getLanguage();
    const now = new Date();
    const diff = date < 0
      ? Math.round(date / (1000 * 60 * 60 * 24))
      : Math.round((date - now.getTime()) / (1000 * 60 * 60 * 24));
    const formatter = new Intl.RelativeTimeFormat(language, { numeric: 'auto' });
    return formatter.format(diff, 'day');
  }
}
