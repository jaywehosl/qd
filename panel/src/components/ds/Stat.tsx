import type { ReactNode } from 'react';

export interface StatProps {
  title: ReactNode;
  value: ReactNode;
  prefix?: ReactNode;
  className?: string;
}

export function Stat({ title, value, prefix, className = '' }: StatProps) {
  return (
    <div className={['ds-stat', className].filter(Boolean).join(' ')}>
      <span className="ds-stat__title">
        {prefix}
        {title}
      </span>
      <span className="ds-stat__value">{value}</span>
    </div>
  );
}
