import { toast, type ToastApi } from '@/components/ds/Toast';

// The DS toaster is the single message backend. getMessage() returns it; the
// setter stays for API compatibility (callers used to register an antd
// message instance here — now a no-op-ish override hook).
let current: ToastApi = toast;

export function setMessageInstance(instance: ToastApi) {
  current = instance;
}

export function getMessage(): ToastApi {
  return current;
}
