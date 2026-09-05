import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  KeyOutlined,
  LockOutlined,
  MoonOutlined,
  SunOutlined,
  TranslationOutlined,
  UserOutlined,
} from '@ant-design/icons';

import { Button, DropdownMenu, Field, Input } from '@/components/ds';
import type { MenuEntry } from '@/components/ds';
import { Spin } from '@/components/ui';
import { HttpUtil, LanguageManager } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import { LoginFormSchema, TwoFactorCodeSchema } from '@/schemas/login';
const basePath = window.X_UI_BASE_PATH || '';

interface LoginErrors {
  username?: string;
  password?: string;
  twoFactorCode?: string;
}

export default function LoginPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, cycleTheme } = useTheme();

  const [fetched, setFetched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [twoFactorEnable, setTwoFactorEnable] = useState(false);
  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage());

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [twoFactorCode, setTwoFactorCode] = useState('');
  const [errors, setErrors] = useState<LoginErrors>({});

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const msg = await HttpUtil.post('/getTwoFactorEnable');
      if (cancelled) return;
      if (msg.success) setTwoFactorEnable(!!msg.obj);
      setFetched(true);
    })();
    return () => { cancelled = true; };
  }, []);

  const validate = useCallback((): boolean => {
    const next: LoginErrors = {};
    const u = LoginFormSchema.shape.username.safeParse(username);
    if (!u.success) next.username = t(u.error.issues[0]?.message ?? 'somethingWentWrong');
    const p = LoginFormSchema.shape.password.safeParse(password);
    if (!p.success) next.password = t(p.error.issues[0]?.message ?? 'somethingWentWrong');
    if (twoFactorEnable) {
      const c = TwoFactorCodeSchema.safeParse(twoFactorCode);
      if (!c.success) next.twoFactorCode = t(c.error.issues[0]?.message ?? 'somethingWentWrong');
    }
    setErrors(next);
    return Object.keys(next).length === 0;
  }, [username, password, twoFactorCode, twoFactorEnable, t]);

  const onSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    try {
      const payload = twoFactorEnable ? { username, password, twoFactorCode } : { username, password };
      const msg = await HttpUtil.post('/login', payload);
      if (msg.success) window.location.href = basePath + 'panel/';
    } finally {
      setSubmitting(false);
    }
  }, [validate, username, password, twoFactorCode, twoFactorEnable]);

  const onLangChange = useCallback((nextLang: string) => {
    setLang(nextLang);
    LanguageManager.setLanguage(nextLang);
  }, []);



  const pageClass = useMemo(() => {
    const classes = ['login-app'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const langMenuItems = useMemo<MenuEntry[]>(
    () => (LanguageManager.supportedLanguages as { value: string; name: string; icon: string }[]).map((l) => ({
      key: l.value,
      selected: l.value === lang,
      label: (
        <span style={{ display: 'inline-flex', gap: 8, alignItems: 'center' }}>
          <span aria-hidden="true">{l.icon}</span>
          <span>{l.name}</span>
        </span>
      ),
      onSelect: () => onLangChange(l.value),
    })),
    [onLangChange, lang],
  );

  const themeIcon = isDark ? <MoonOutlined /> : <SunOutlined />;

  return (
    <div className={pageClass}>
      <div className="login-content">
        <div className="login-header">
          <div className="brand-block">
            <svg className="antigravity-logo-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" style={{ width: 24, height: 24, marginRight: 8 }}>
              <path d="M12 2L2 22h20L12 2z" fill="var(--color-primary)" />
              <path d="M12 6l7 13H5l7-13z" fill="#FFFFFF" opacity="0.3" />
            </svg>
            <span className="brand-text">Community</span>
          </div>
          <div className="login-header-right">
            <Button
              id="login-theme-cycle"
              className="toolbar-btn"
              aria-label={t('menu.theme')}
              title={t('menu.theme')}
              icon={themeIcon}
              onClick={() => cycleTheme('login-theme-cycle')}
            />
            <DropdownMenu
              align="end"
              items={langMenuItems}
              trigger={(
                <Button
                  className="toolbar-btn"
                  aria-label={t('pages.settings.language')}
                  icon={<TranslationOutlined />}
                />
              )}
            />
          </div>
        </div>

        <div className="login-hero-container">
          <h1 className="login-hero-title">
            Your network at a glance
          </h1>
          <p className="login-hero-subtitle">
            A clean, spacious control panel for the QUIC Diver network.
          </p>
        </div>

        <div className="login-wrapper">
          {!fetched ? (
            <div className="login-loading">
              <Spin spinning size="large" />
            </div>
          ) : (
            <div className="login-card">
              <form className="login-form" onSubmit={onSubmit} noValidate>
                <Field label={t('username')} error={errors.username}>
                  <div className="login-field">
                    <span className="login-field-icon"><UserOutlined /></span>
                    <Input
                      className="has-icon"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoComplete="username"
                      placeholder={t('username')}
                      autoFocus
                    />
                  </div>
                </Field>

                <Field label={t('password')} error={errors.password}>
                  <div className="login-field">
                    <span className="login-field-icon"><LockOutlined /></span>
                    <Input
                      className="has-icon"
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="current-password"
                      placeholder={t('password')}
                    />
                  </div>
                </Field>

                {twoFactorEnable && (
                  <Field label={t('twoFactorCode')} error={errors.twoFactorCode}>
                    <div className="login-field">
                      <span className="login-field-icon"><KeyOutlined /></span>
                      <Input
                        className="has-icon"
                        value={twoFactorCode}
                        onChange={(e) => setTwoFactorCode(e.target.value)}
                        autoComplete="one-time-code"
                        placeholder={t('twoFactorCode')}
                      />
                    </div>
                  </Field>
                )}

                <div className="submit-row">
                  <Button
                    variant="primary"
                    htmlType="submit"
                    loading={submitting}
                    size="lg"
                    block
                    className="login-submit-btn"
                  >
                    {t('login')}
                  </Button>
                </div>
              </form>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
