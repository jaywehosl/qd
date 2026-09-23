export function readLocalToken(): string | undefined {
  if (typeof window === 'undefined') return undefined;
  if (window.QD_TOKEN) return window.QD_TOKEN;
  try {
    return sessionStorage.getItem('qd.token') || undefined;
  } catch {
    return undefined;
  }
}
