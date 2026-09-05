import type { HTMLAttributes, ReactNode } from 'react';

export type TagTone = 'neutral' | 'primary' | 'success' | 'warning' | 'danger';

export interface TagProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: TagTone;
  icon?: ReactNode;
}

export function Tag({ tone = 'neutral', icon, className = '', children, ...rest }: TagProps) {
  const cls = ['ds-tag', tone !== 'neutral' && `ds-tag--${tone}`, className].filter(Boolean).join(' ');
  return (
    <span className={cls} {...rest}>
      {icon}
      {children}
    </span>
  );
}
