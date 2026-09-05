import * as RMenu from '@radix-ui/react-dropdown-menu';
import type { ReactNode } from 'react';

export interface MenuItem {
  key: string;
  label: ReactNode;
  icon?: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  /** Marks the currently-chosen entry — highlighted when the menu opens. */
  selected?: boolean;
  onSelect?: () => void;
}

export type MenuEntry = MenuItem | { type: 'divider'; key?: string };

export interface DropdownMenuProps {
  /** The element that opens the menu. */
  trigger: ReactNode;
  items: MenuEntry[];
  align?: 'start' | 'center' | 'end';
}

function isDivider(e: MenuEntry): e is { type: 'divider'; key?: string } {
  return (e as { type?: string }).type === 'divider';
}

export function DropdownMenu({ trigger, items, align = 'end' }: DropdownMenuProps) {
  return (
    <RMenu.Root>
      <RMenu.Trigger asChild>{trigger}</RMenu.Trigger>
      <RMenu.Portal>
        <RMenu.Content className="ds-menu" align={align} sideOffset={6} collisionPadding={8}>
          {items.map((entry, i) =>
            isDivider(entry) ? (
              <RMenu.Separator key={entry.key ?? `sep-${i}`} className="ds-menu__sep" />
            ) : (
              <RMenu.Item
                key={entry.key}
                className={[
                  'ds-menu__item',
                  entry.danger && 'ds-menu__item--danger',
                  entry.selected && 'ds-menu__item--selected',
                ].filter(Boolean).join(' ')}
                disabled={entry.disabled}
                onSelect={entry.onSelect}
              >
                {entry.icon}
                <span className="ds-menu__label">{entry.label}</span>
                {entry.selected && <span className="ds-menu__check" aria-hidden="true">✓</span>}
              </RMenu.Item>
            ),
          )}
        </RMenu.Content>
      </RMenu.Portal>
    </RMenu.Root>
  );
}
