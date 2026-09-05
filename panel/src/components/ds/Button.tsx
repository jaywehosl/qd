import { forwardRef } from 'react';
import type { ButtonHTMLAttributes, ReactNode } from 'react';

export type ButtonVariant = 'default' | 'primary' | 'text';
export type ButtonSize = 'sm' | 'md' | 'lg';

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'type'> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  danger?: boolean;
  block?: boolean;
  loading?: boolean;
  icon?: ReactNode;
  /** Native button type. Defaults to "button" to avoid accidental form submits. */
  htmlType?: 'button' | 'submit' | 'reset';
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'default', size = 'md', danger, block, loading, icon, htmlType = 'button', className = '', children, disabled, ...rest },
  ref,
) {
  const iconOnly = icon != null && (children == null || children === false);
  const cls = [
    'ds-btn',
    `ds-btn--${variant}`,
    size !== 'md' && `ds-btn--${size}`,
    danger && 'ds-btn--danger',
    block && 'ds-btn--block',
    iconOnly && 'ds-btn--icon',
    className,
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <button ref={ref} type={htmlType} className={cls} disabled={disabled || loading} {...rest}>
      {loading ? <span className="ds-btn__spinner" /> : icon}
      {children}
    </button>
  );
});
