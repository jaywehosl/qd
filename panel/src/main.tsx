import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router-dom';
import '@/styles/reset.css';
import '@/skin/tokens.css';
import '@/skin/base.css';
import '@/skin/surface.css';
import '@/skin/shell.css';
import '@/skin/controls.css';
import '@/skin/overlay.css';
import '@/skin/table.css';
import '@/skin/client-pages.css';
import '@/skin/panel-pages.css';
import '@/skin/pages-rest.css';
import '@/skin/widgets.css';
import '@/skin/bits.css';
import '@/skin/panel-detail.css';
import '@/skin/phone-admin.css';
import '@/skin/scroller.css';
import '@/skin/motion.css';

import { setupAxios } from '@/api/axios-init';
import { readyI18n } from '@/i18n/react';
import { ThemeProvider } from '@/hooks/useTheme';
import { QueryProvider } from '@/api/QueryProvider';
import { ToastViewport } from '@/components/ds';
import { router } from '@/routes';
import { bootstrapTheme } from '@/theme/themeStorage';
import { mountScroller } from '@/skin/scroller';
import { watchTitleBar } from '@/skin/titlebar';
import { mountTips } from '@/lib/tips';
import { mountPhone } from '@/lib/phone';

setupAxios();
bootstrapTheme();
mountScroller();
watchTitleBar();
mountTips();
mountPhone();

readyI18n().then(() => {
  const root = document.getElementById('app');
  if (root) {
    createRoot(root).render(
      <ThemeProvider>
        <QueryProvider>
          <RouterProvider router={router} />
          <ToastViewport />
        </QueryProvider>
      </ThemeProvider>,
    );
  }

});
