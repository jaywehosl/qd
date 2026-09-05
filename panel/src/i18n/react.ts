import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';

import { LanguageManager } from '@/utils';
import enUS from '../../../web/translation/en-US.json';

const FALLBACK = 'en-US';

export async function readyI18n() {
  await i18next.use(initReactI18next).init({
    lng: LanguageManager.getLanguage(),
    fallbackLng: FALLBACK,
    resources: { [FALLBACK]: { translation: enUS } },
    interpolation: { escapeValue: false, prefix: '{', suffix: '}' },
    returnNull: false,
  });
  return i18next;
}

export { i18next as i18n };
