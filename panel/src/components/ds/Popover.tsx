import * as RPopover from '@radix-ui/react-popover';
import type { ReactNode } from 'react';

export interface PopoverProps {
  trigger: ReactNode;
  content: ReactNode;
  side?: 'top' | 'right' | 'bottom' | 'left';
  align?: 'start' | 'center' | 'end';
  /** Padding inside the floating panel (default 12). */
  padded?: boolean;
  /** Fires on open and on close; the panel stays uncontrolled either way. */
  onOpenChange?: (open: boolean) => void;
}

/** Click-triggered floating panel with arbitrary content (Radix Popover). */
export function Popover({
  trigger, content, side = 'bottom', align = 'center', padded = true, onOpenChange,
}: PopoverProps) {
  return (
    <RPopover.Root onOpenChange={onOpenChange}>
      <RPopover.Trigger asChild>{trigger}</RPopover.Trigger>
      <RPopover.Portal>
        <RPopover.Content
          className={`ds-popover${padded ? ' ds-popover--padded' : ''}`}
          side={side}
          align={align}
          sideOffset={8}
          collisionPadding={8}
        >
          {content}
        </RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}
