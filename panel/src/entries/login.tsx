import { createRoot } from 'react-dom/client';
import '@/styles/reset.css';
import '@/skin/tokens.css';
import '@/skin/base.css';
import '@/skin/surface.css';
import '@/skin/controls.css';
import '@/skin/overlay.css';
import '@/skin/pages-rest.css';
import '@/skin/widgets.css';
import '@/skin/scroller.css';
import { setupAxios } from '@/api/axios-init';
import { applyDocumentTitle } from '@/utils';
import { readyI18n } from '@/i18n/react';
import { ThemeProvider } from '@/hooks/useTheme';
import { QueryProvider } from '@/api/QueryProvider';
import { ToastViewport } from '@/components/ds';
import { bootstrapTheme } from '@/theme/themeStorage';
import LoginPage from '@/pages/login/LoginPage';
import { mountScroller } from '@/skin/scroller';

setupAxios();
bootstrapTheme();
applyDocumentTitle();
mountScroller();

readyI18n().then(() => {
  const root = document.getElementById('app');
  if (root) {
    createRoot(root).render(
      <ThemeProvider>
        <QueryProvider>
          <LoginPage />
          <ToastViewport />
        </QueryProvider>
      </ThemeProvider>,
    );
  }
});
