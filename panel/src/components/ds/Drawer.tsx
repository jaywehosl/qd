import * as RDialog from '@radix-ui/react-dialog';
import type { ReactNode } from 'react';
import { useClosing } from './Dialog';

export interface DrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  width?: number | string;
  side?: 'left' | 'right';
}

/** Side panel built on Radix Dialog (focus-trap, ESC, scroll-lock, a11y). */
export function Drawer({ open, onOpenChange, title, children, footer, width = 420, side = 'right' }: DrawerProps) {
  const closing = useClosing(open);
  if (!open && !closing) return null;

  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal forceMount>
        <RDialog.Overlay className={`ds-dialog__overlay${closing ? ' is-closing' : ''}`} />
        <RDialog.Content
          className={`ds-drawer__content ds-drawer__content--${side}${closing ? ' is-closing' : ''}`}
          style={{ width: typeof width === 'number' ? `${width}px` : width }}
          aria-describedby={undefined}
        >
          <div className="ds-drawer__header">
            <RDialog.Title className="ds-dialog__title">{title}</RDialog.Title>
            <RDialog.Close asChild>
              <button className="ds-dialog__close" aria-label="Close">&times;</button>
            </RDialog.Close>
          </div>
          <div className="ds-drawer__body">{children}</div>
          {footer != null && <div className="ds-drawer__footer">{footer}</div>}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
