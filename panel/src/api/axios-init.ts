import axios from 'axios';
import type { AxiosError, AxiosResponse, InternalAxiosRequestConfig } from 'axios';

import { clientUnreachable, showSessionGone } from '@/lib/client-closed';
import { restarting } from '@/lib/handover';
import { readLocalToken } from './localToken';

export function setupAxios(): void {
  const localToken = readLocalToken();

  axios.interceptors.request.use((config: InternalAxiosRequestConfig) => {
    if (localToken) config.headers.set('X-QD-Token', localToken);
    return config;
  });

  axios.interceptors.response.use(
    (response: AxiosResponse) => response,
    (error: AxiosError) => {
      if (!error.response && error.code !== 'ERR_CANCELED') {
        if (!restarting()) clientUnreachable();
        return Promise.reject(error);
      }
      if (error.response?.status === 401) {
        showSessionGone();
        return new Promise(() => {});
      }
      return Promise.reject(error);
    },
  );
}
