import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { MoonFilled, MoonOutlined, SunOutlined, TranslationOutlined } from '@ant-design/icons';

import { DropdownMenu } from '@/components/ds';
import type { MenuEntry } from '@/components/ds';
import { LanguageManager } from '@/utils';

export function ThemeCycleButton({ id, isDark, isUltra, onCycle, ariaLabel }: {
  id: string;
  isDark: boolean;
  isUltra: boolean;
  onCycle: () => void;
  ariaLabel: string;
}) {
  const icon = !isDark ? <SunOutlined /> : !isUltra ? <MoonOutlined /> : <MoonFilled />;
  return (
    <button
      id={id}
      type="button"
      className="sidebar-theme-cycle"
      aria-label={ariaLabel}
      title={ariaLabel}
      onClick={onCycle}
    >
      {icon}
    </button>
  );
}

export function LanguageSelector() {
  const { t } = useTranslation();
  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage());
  const items = useMemo<MenuEntry[]>(
    () => (LanguageManager.supportedLanguages as { value: string; name: string; icon: string }[]).map((l) => ({
      key: l.value,
      selected: l.value === lang,
      label: (
        <>
          <span className="ds-menu__flag" aria-hidden="true">{l.value.split('-')[0].toUpperCase()}</span>
          <span>{l.name}</span>
        </>
      ),
      onSelect: () => { setLang(l.value); LanguageManager.setLanguage(l.value); },
    })),
    [lang],
  );
  return (
    <DropdownMenu
      align="end"
      items={items}
      trigger={(
        <button
          type="button"
          className="sidebar-theme-cycle"
          aria-label={t('pages.settings.language')}
          title={t('pages.settings.language')}
        >
          <TranslationOutlined />
        </button>
      )}
    />
  );
}
