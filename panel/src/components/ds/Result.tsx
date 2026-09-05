import type { ReactNode } from 'react';
import { WarningOutlined, CloseCircleOutlined, InfoCircleOutlined, CheckCircleOutlined } from '@ant-design/icons';

export interface ResultProps {
  status?: 'success' | 'error' | 'info' | 'warning';
  title?: ReactNode;
  subTitle?: ReactNode;
  extra?: ReactNode;
  className?: string;
}

export function Result({
  status = 'info',
  title,
  subTitle,
  extra,
  className = '',
}: ResultProps) {
  let icon = <InfoCircleOutlined style={{ color: 'var(--color-primary)' }} />;
  if (status === 'error') {
    icon = <CloseCircleOutlined style={{ color: 'var(--color-error)' }} />;
  } else if (status === 'warning') {
    icon = <WarningOutlined style={{ color: 'var(--color-warning)' }} />;
  } else if (status === 'success') {
    icon = <CheckCircleOutlined style={{ color: 'var(--color-success)' }} />;
  }

  return (
    <div className={`ds-result ds-result--${status} ${className}`}>
      <div className="ds-result__icon">{icon}</div>
      {title && <h2 className="ds-result__title">{title}</h2>}
      {subTitle && <p className="ds-result__subtitle">{subTitle}</p>}
      {extra && <div className="ds-result__extra">{extra}</div>}
    </div>
  );
}
