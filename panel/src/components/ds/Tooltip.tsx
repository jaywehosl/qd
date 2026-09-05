import * as RTooltip from '@radix-ui/react-tooltip';
import type { ReactNode } from 'react';

export interface TooltipProps {
  title: ReactNode;
  children: ReactNode;
  side?: 'top' | 'right' | 'bottom' | 'left';
}

/** Wrap once near the app root for shared timing; safe to nest per-tooltip too. */
export function TooltipProvider({ children }: { children: ReactNode }) {
  return <RTooltip.Provider delayDuration={300}>{children}</RTooltip.Provider>;
}

export function Tooltip({ title, children, side = 'top' }: TooltipProps) {
  if (title == null || title === '') return <>{children}</>;
  return (
    <RTooltip.Root>
      <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
      <RTooltip.Portal>
        <RTooltip.Content className="ds-tooltip" side={side} sideOffset={6}>
          {title}
        </RTooltip.Content>
      </RTooltip.Portal>
    </RTooltip.Root>
  );
}
