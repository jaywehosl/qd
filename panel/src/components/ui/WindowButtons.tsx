import { inWindow, windowCommand } from '@/skin/titlebar';

export default function WindowButtons() {
  if (!inWindow()) return null;

  return (
    <div className="win-buttons" data-drag="off">
      <button
        type="button"
        className="win-button"
        aria-label="minimise"
        onClick={() => windowCommand('minimise')}
      >
        <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2 6h8" /></svg>
      </button>
      <button
        type="button"
        className="win-button"
        aria-label="maximise"
        onClick={() => windowCommand('maximise')}
      >
        <svg viewBox="0 0 12 12" aria-hidden="true"><rect x="2.5" y="2.5" width="7" height="7" rx="1" /></svg>
      </button>
      <button
        type="button"
        className="win-button win-button--close"
        aria-label="close"
        onClick={() => windowCommand('close')}
      >
        <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M3 3l6 6M9 3l-6 6" /></svg>
      </button>
    </div>
  );
}
