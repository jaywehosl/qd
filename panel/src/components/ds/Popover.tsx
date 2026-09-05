import * as RPopover from '@radix-ui/react-popover';
import type { ReactNode } from 'react';

export interface PopoverProps {
  trigger: ReactNode;
  content: ReactNode;
  side?: 'top' | 'right' | 'bottom' | 'left';
  align?: 'start' | 'center' | 'end';
  padded?: boolean;
  onOpenChange?: (open: boolean) => void;
}

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
