import * as RDialog from '@radix-ui/react-dialog';
import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { CloseOutlined } from '@ant-design/icons';
import { Button } from './Button';

export const CLOSE_MS = 150;

export function useClosing(open: boolean) {
  const [closing, setClosing] = useState(false);
  const was = useRef(open);

  useEffect(() => {
    const leaving = was.current && !open;
    was.current = open;
    if (open) {
      setClosing(false);
      return undefined;
    }
    if (!leaving) return undefined;
    setClosing(true);
    const id = window.setTimeout(() => setClosing(false), CLOSE_MS);
    return () => window.clearTimeout(id);
  }, [open]);

  return closing;
}

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  /** Convenience confirm/cancel footer. Ignored when `footer` is provided. */
  okText?: ReactNode;
  cancelText?: ReactNode;
  onOk?: () => void;
  okDanger?: boolean;
  okDisabled?: boolean;
  confirmLoading?: boolean;
  width?: number | string;
  /** Skip the fixed-height layout that wide dialogs get by default. */
  autoHeight?: boolean;
  /** Hide the default header close button. */
  hideClose?: boolean;
}

/**
 * Accessible modal dialog (Radix). Focus-trap, ESC, scroll-lock and overlay
 * click-to-close are handled by Radix; we only style it.
 */
export function Dialog({
  open,
  onOpenChange,
  title,
  children,
  footer,
  okText = 'OK',
  cancelText = 'Cancel',
  onOk,
  okDanger,
  okDisabled,
  confirmLoading,
  width,
  autoHeight,
  hideClose,
}: DialogProps) {
  const resolvedFooter =
    footer !== undefined
      ? footer
      : onOk
        ? (
            <>
              <Button onClick={() => onOpenChange(false)}>{cancelText}</Button>
              <Button variant="primary" danger={okDanger} disabled={okDisabled} loading={confirmLoading} onClick={onOk}>
                {okText}
              </Button>
            </>
          )
        : null;

  // Form/content dialogs (inbound/outbound/rule/…) are unified to one standard
  // size — the "Add Inbound" reference (780 × fixed height) — so every form
  // modal is identical and never resizes/jumps between tabs; the body is the
  // single scroll region. Narrow confirms (<520) keep their own width + auto
  // height.
  const isTall = !autoHeight && typeof width === 'number' && width >= 520;
  // A confirm that names no width used to be held in by a 560px cap in the
  // stylesheet; without that cap it stretched the full page, so the default
  // lives here now — where the other size decisions already are.
  const resolvedWidth = isTall ? 780 : (width ?? 460);

  const closing = useClosing(open);
  if (!open && !closing) return null;

  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal forceMount>
        {/* fixed flex-centring viewport → integer position, no translate() layer
            that settles and nudges text on open (see REDESIGN_TODO). */}
        <RDialog.Overlay className={`ds-dialog__overlay ds-dialog__viewport${closing ? ' is-closing' : ''}`}>
          <RDialog.Content
            className={`ds-dialog__content${isTall ? ' ds-dialog__content--tall' : ''}${closing ? ' is-closing' : ''}`}
            style={resolvedWidth ? { width: typeof resolvedWidth === 'number' ? `${resolvedWidth}px` : resolvedWidth } : undefined}
            aria-describedby={undefined}
          >
            <div className="ds-dialog__header">
              <RDialog.Title className="ds-dialog__title">{title}</RDialog.Title>
              {!hideClose && (
                <RDialog.Close asChild>
                  <button className="ds-dialog__close" aria-label="Close">
                    <CloseOutlined />
                  </button>
                </RDialog.Close>
              )}
            </div>
            <div className="ds-dialog__body">{children}</div>
            {resolvedFooter != null && <div className="ds-dialog__footer">{resolvedFooter}</div>}
          </RDialog.Content>
        </RDialog.Overlay>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
