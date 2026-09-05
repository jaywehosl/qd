import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SnippetsOutlined } from '@ant-design/icons';

import { Button, Input } from '@/components/ds';
import { useTheme } from '@/hooks/useTheme';
import { ThemeCycleButton } from '@/layouts/HeaderButtons';
import BrandMark from '@/components/ui/BrandMark';
import WindowButtons from '@/components/ui/WindowButtons';

interface ImportPageProps {
  onImport: (uri: string) => Promise<{ ok: boolean; error?: string }>;
}

/**
 * What the operator meets before anything else exists: no account, no password,
 * just the link. Whether that link makes them an admin is the node's answer,
 * not something read out of the text here.
 */
export default function ImportPage({ onImport }: ImportPageProps) {
  const { t } = useTranslation();
  const { isDark, isUltra, cycleTheme } = useTheme();

  const [uri, setUri] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  const submit = useCallback(async (value: string) => {
    const trimmed = value.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError('');
    try {
      const result = await onImport(trimmed);
      if (!result.ok) setError(result.error || t('client.import.failed'));
    } finally {
      setBusy(false);
    }
  }, [busy, onImport, t]);

  // Pasting anywhere on the page is enough — no need to hit the field first,
  // and no need to press the button afterwards.
  useEffect(() => {
    function onPaste(e: ClipboardEvent) {
      const text = e.clipboardData?.getData('text') ?? '';
      if (!text.trim()) return;
      e.preventDefault();
      setUri(text.trim());
      void submit(text);
    }
    window.addEventListener('paste', onPaste);
    return () => window.removeEventListener('paste', onPaste);
  }, [submit]);

  useEffect(() => { inputRef.current?.focus(); }, []);

  const fromClipboard = useCallback(async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (!text.trim()) return;
      setUri(text.trim());
      void submit(text);
    } catch {
      setError(t('client.import.clipboardDenied'));
    }
  }, [submit, t]);

  return (
    <div className="imp">
      {/* The window has no chrome of its own, so this screen carries the bar
          that drags it and the buttons that close it. */}
      <div className="topbar-shell">
        <header className="antigravity-header client-header">
          <div className="header-container">
            <div className="header-left">
              <div className="brand-block" data-drag="off">
                <BrandMark state="off" />
              </div>
            </div>
            <div className="header-right">
              <div className="win-tray" data-drag="off">
                <ThemeCycleButton
                  id="import-theme-cycle"
                  isDark={isDark}
                  isUltra={isUltra}
                  onCycle={() => cycleTheme('import-theme-cycle')}
                  ariaLabel={t('menu.theme')}
                />
                <WindowButtons />
              </div>
            </div>
          </div>
        </header>
      </div>

      <main className="imp-main">
        <div className="imp-lede">
          <h1 className="imp-title">{t('client.import.title')}</h1>
          <p className="imp-sub">{t('client.import.subtitle')}</p>
        </div>

        <div className="imp-card">
          <Input
            ref={inputRef}
            value={uri}
            placeholder="qd://…"
            spellCheck={false}
            onChange={(e) => { setUri(e.target.value); setError(''); }}
            onKeyDown={(e) => { if (e.key === 'Enter') void submit(uri); }}
          />

          {error && <div className="imp-error">{error}</div>}

          <div className="imp-actions">
            <Button block icon={<SnippetsOutlined />} disabled={busy} onClick={fromClipboard}>
              {t('client.import.fromClipboard')}
            </Button>
            <Button block variant="primary" loading={busy} onClick={() => void submit(uri)}>
              {t('client.import.action')}
            </Button>
          </div>

          <p className="imp-hint">{t('client.import.hint')}</p>
        </div>
      </main>
    </div>
  );
}
