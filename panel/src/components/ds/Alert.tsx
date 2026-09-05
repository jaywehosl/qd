import type { ReactNode } from 'react';
import {
  CheckCircleFilled,
  CloseCircleFilled,
  ExclamationCircleFilled,
  InfoCircleFilled,
} from '@ant-design/icons';

export type AlertTone = 'info' | 'success' | 'warning' | 'error';

export interface AlertProps {
  tone?: AlertTone;
  title?: ReactNode;
  description?: ReactNode;
  className?: string;
  style?: React.CSSProperties;
}

const ICONS = {
  info: <InfoCircleFilled />,
  success: <CheckCircleFilled />,
  warning: <ExclamationCircleFilled />,
  error: <CloseCircleFilled />,
};

export function Alert({ tone = 'info', title, description, className = '', style }: AlertProps) {
  return (
    <div className={['ds-alert', `ds-alert--${tone}`, className].filter(Boolean).join(' ')} style={style} role="alert">
      <span className="ds-alert__icon">{ICONS[tone]}</span>
      <div className="ds-alert__body">
        {title != null && <div className="ds-alert__title">{title}</div>}
        {description != null && <div className="ds-alert__desc">{description}</div>}
      </div>
    </div>
  );
}
