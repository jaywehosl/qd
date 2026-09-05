import { useEffect } from 'react';

export const APP_TITLE = 'QUIC Diver Client';

export function usePageTitle() {
  useEffect(() => {
    document.title = APP_TITLE;
  }, []);
}
