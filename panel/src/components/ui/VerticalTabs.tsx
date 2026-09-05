import type React from 'react';

interface TabItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
}

interface VerticalTabsProps {
  items: TabItem[];
  activeKey: string;
  onChange: (key: string) => void;
}

export default function VerticalTabs({ items, activeKey, onChange }: VerticalTabsProps) {
  return (
    <div className="vertical-tabs-container">
      {items.map((item) => (
        <button
          key={item.key}
          data-tab-key={item.key}
          type="button"
          onClick={() => onChange(item.key)}
          className={`vtab-btn${item.key === activeKey ? ' is-active' : ''}`}
        >
          {item.icon && <span className="vtab-icon">{item.icon}</span>}
          <span>{item.label}</span>
        </button>
      ))}
    </div>
  );
}
