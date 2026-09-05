import type { HTMLAttributes, ReactNode } from 'react';

export interface CardProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  title?: ReactNode;
  extra?: ReactNode;
  /** Remove body padding (e.g. when embedding a table). */
  flush?: boolean;
}

export function Card({ title, extra, flush, className = '', children, ...rest }: CardProps) {
  const cls = ['ds-card', flush && 'ds-card--pad-0', className].filter(Boolean).join(' ');
  return (
    <div className={cls} {...rest}>
      {(title != null || extra != null) && (
        <div className="ds-card__header">
          {title != null && <div className="ds-card__title">{title}</div>}
          {extra != null && <div className="ds-card__extra">{extra}</div>}
        </div>
      )}
      <div className="ds-card__body">{children}</div>
    </div>
  );
}
