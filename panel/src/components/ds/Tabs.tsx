import * as RTabs from '@radix-ui/react-tabs';
import type { ReactNode } from 'react';

export interface TabItem {
  key: string;
  label: ReactNode;
  children: ReactNode;
  disabled?: boolean;
}

export interface TabsProps {
  items: TabItem[];
  /** Uncontrolled initial tab. */
  defaultActiveKey?: string;
  /** Controlled active tab. */
  activeKey?: string;
  onChange?: (key: string) => void;
  className?: string;
}

export function Tabs({ items, defaultActiveKey, activeKey, onChange, className = '' }: TabsProps) {
  return (
    <RTabs.Root
      className={['ds-tabs', className].filter(Boolean).join(' ')}
      defaultValue={defaultActiveKey ?? items[0]?.key}
      value={activeKey}
      onValueChange={onChange}
    >
      <RTabs.List className="ds-tabs__list">
        {items.map((it) => (
          <RTabs.Trigger key={it.key} value={it.key} disabled={it.disabled} className="ds-tabs__trigger">
            {it.label}
          </RTabs.Trigger>
        ))}
      </RTabs.List>
      {items.map((it) => (
        <RTabs.Content key={it.key} value={it.key} className="ds-tabs__content">
          {it.children}
        </RTabs.Content>
      ))}
    </RTabs.Root>
  );
}
