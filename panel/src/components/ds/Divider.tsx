import type { ReactNode } from 'react';

export interface DividerProps {
  children?: ReactNode;
  className?: string;
}

export function Divider({ children, className = '' }: DividerProps) {
  const cls = ['ds-divider', children != null && 'ds-divider--labelled', className].filter(Boolean).join(' ');
  return (
    <div className={cls} role="separator">
      {children != null && <span className="ds-divider__label">{children}</span>}
    </div>
  );
}
