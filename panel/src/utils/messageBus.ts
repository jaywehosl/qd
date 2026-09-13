import { toast, type ToastApi } from '@/components/ds/Toast';

let current: ToastApi = toast;

export function setMessageInstance(instance: ToastApi) {
  current = instance;
}

export function getMessage(): ToastApi {
  return current;
}
