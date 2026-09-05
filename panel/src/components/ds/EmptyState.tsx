import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { InboxOutlined } from '@ant-design/icons';

export interface EmptyStateProps {
  icon?: ReactNode;
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
  children?: ReactNode;
}

export function EmptyState({
  icon = <InboxOutlined />,
  title,
  description,
  action,
  className = '',
  children,
}: EmptyStateProps) {
  const { t } = useTranslation();

  const displayTitle = title ?? t('noData', 'Nothing here yet');
  const displayDescription = description ?? t('emptyDesc', 'No records found in this list');

  return (
    <div className={`ds-empty-state ${className}`}>
      <div className="ds-empty-state__icon-wrap">
        <div className="ds-empty-state__pulse" />
        <div className="ds-empty-state__icon">{icon}</div>
      </div>
      <h3 className="ds-empty-state__title">{displayTitle}</h3>
      {displayDescription && <p className="ds-empty-state__desc">{displayDescription}</p>}
      {action && <div className="ds-empty-state__action">{action}</div>}
      {children}
    </div>
  );
}
