import type { ReactNode } from 'react';
interface SettingListItemProps {
  paddings?: 'small' | 'default';
  title?: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  control?: ReactNode;
}

export default function SettingListItem({
  paddings = 'default',
  title,
  description,
  children,
  control,
}: SettingListItemProps) {
  return (
    <div className={`setting-list-item${paddings === 'small' ? ' is-tight' : ''}`}>
      <div className="setting-list-grid">
        <div className="setting-list-meta">
          {title && <div className="setting-list-title">{title}</div>}
          {description && <div className="setting-list-description">{description}</div>}
        </div>
        <div className="setting-list-control">
          {control ?? children}
        </div>
      </div>
    </div>
  );
}
