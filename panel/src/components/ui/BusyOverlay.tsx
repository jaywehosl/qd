import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
interface BusyOverlayProps {
  open: boolean;
  title: string;
  subtitle?: string;
}

const EXIT_MS = 360;

export default function BusyOverlay({ open, title, subtitle }: BusyOverlayProps) {
  const [mounted, setMounted] = useState(open);
  const [closing, setClosing] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (open) {
      window.clearTimeout(timer.current);
      setClosing(false);
      setMounted(true);
    } else {
      setMounted((wasMounted) => {
        if (!wasMounted) return false;
        setClosing(true);
        timer.current = window.setTimeout(() => {
          setMounted(false);
          setClosing(false);
        }, EXIT_MS);
        return true;
      });
    }
    return () => window.clearTimeout(timer.current);
  }, [open]);

  if (!mounted) return null;

  return createPortal(
    <div
      className={`busy-overlay ${closing ? 'is-closing' : ''}`}
      role="alertdialog"
      aria-live="assertive"
      aria-busy="true"
    >
      <div className="busy-overlay__card">
        <div className="busy-overlay__mark">
          <span className="busy-overlay__ring" aria-hidden="true" />
          <svg className="busy-overlay__logo" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 2L2 22h20L12 2z" fill="var(--color-primary)" />
            <path d="M12 6l7 13H5l7-13z" fill="#FFFFFF" opacity="0.3" />
          </svg>
        </div>
        <div className="busy-overlay__text">
          <div className="busy-overlay__title">{title}</div>
          {subtitle && <div className="busy-overlay__subtitle">{subtitle}</div>}
        </div>
      </div>
    </div>,
    document.body,
  );
}
