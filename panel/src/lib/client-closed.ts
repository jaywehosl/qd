const OVERLAY_ID = 'qd-client-closed';

export function showClientClosed(): void {
  showOverlay(
    'qd is closed',
    'The client is no longer running, so this page has nothing to talk to. Start qd again and open it from the tray icon.',
  );
}

export function showSessionGone(): void {
  showOverlay(
    'This page lost its key',
    'The key that let this page talk to the client is gone. Open the client again from the tray icon to get a fresh one.',
  );
}

function showOverlay(heading: string, body: string): void {
  if (typeof document === 'undefined' || document.getElementById(OVERLAY_ID)) return;

  const root = document.documentElement;
  root.style.cssText = 'height:100%;margin:0;padding:0;filter:none;transform:none';
  document.body.replaceChildren();
  document.body.style.cssText =
    'height:100%;min-height:100vh;margin:0;padding:0;filter:none;transform:none;overflow:auto;background:#0f1115';

  const overlay = document.createElement('div');
  overlay.id = OVERLAY_ID;
  overlay.setAttribute('role', 'alert');
  overlay.style.cssText = [
    'box-sizing:border-box',
    'min-height:100vh',
    'width:100%',
    'display:flex',
    'align-items:center',
    'justify-content:center',
    'padding:24px',
    'background:#0f1115',
    'color:#e6e8ec',
    'font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif',
  ].join(';');

  const card = document.createElement('div');
  card.style.cssText = 'max-width:420px;width:100%;text-align:center';

  const dot = document.createElement('div');
  dot.style.cssText =
    'width:12px;height:12px;border-radius:50%;background:#8a8a8a;margin:0 auto 20px';

  const title = document.createElement('h1');
  title.textContent = heading;
  title.style.cssText = 'margin:0 0 12px;font-size:20px;font-weight:600;letter-spacing:-0.01em';

  const text = document.createElement('p');
  text.textContent = body;
  text.style.cssText = 'margin:0 0 24px;font-size:14px;line-height:1.6;color:#9aa0aa';

  const retry = document.createElement('button');
  retry.type = 'button';
  retry.textContent = 'Try again';
  retry.style.cssText = [
    'appearance:none',
    'border:1px solid #2a2f3a',
    'background:#171a21',
    'color:#e6e8ec',
    'border-radius:8px',
    'padding:9px 18px',
    'font-size:14px',
    'font-family:inherit',
    'cursor:pointer',
  ].join(';');
  retry.addEventListener('click', () => window.location.reload());

  card.append(dot, title, text, retry);
  overlay.append(card);
  document.body.append(overlay);
}

export function looksLikeClientGone(error: unknown): boolean {
  if (!error) return false;
  const message = String((error as { message?: unknown })?.message ?? error);
  return (
    /failed to fetch dynamically imported module/i.test(message) ||
    /unable to preload css/i.test(message) ||
    /loading chunk .* failed/i.test(message) ||
    /importing a module script failed/i.test(message) ||
    /dynamically imported module/i.test(message)
  );
}
