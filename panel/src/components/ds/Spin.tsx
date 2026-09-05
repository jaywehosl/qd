import type { ReactNode } from 'react';

export interface SpinProps {
  spinning?: boolean;
  description?: ReactNode;
  size?: 'small' | 'default' | 'large';
  children?: ReactNode;
}

export function Spin({
  spinning = true,
  description,
  size = 'default',
  children,
}: SpinProps) {
  if (!spinning) return <>{children}</>;

  return (
    <div className={`ds-spin-container ds-spin-container--${size}`}>
      {children && <div className="ds-spin-content-blur">{children}</div>}
      <div className="ds-spin-overlay">
        <div className="ds-spin-spinner-wrap">
          <svg className="ds-spin-spinner" viewBox="0 0 50 50">
            <circle className="ds-spin-path" cx="25" cy="25" r="20" fill="none" strokeWidth="4" />
          </svg>
        </div>
        {description && <div className="ds-spin-desc">{description}</div>}
      </div>
    </div>
  );
}
