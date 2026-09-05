import { useEffect } from 'react';

export const APP_TITLE = 'qd';

export function usePageTitle() {
  useEffect(() => {
    document.title = APP_TITLE;
  }, []);
}
