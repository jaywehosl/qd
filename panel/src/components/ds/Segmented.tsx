import type { ReactNode } from 'react';

export interface SegmentedOption {
  value: string;
  label: ReactNode;
}

export interface SegmentedProps {
  value: string;
  onChange: (value: string) => void;
  options: SegmentedOption[];
  className?: string;
}

/** Button-style single-select (replaces antd Radio.Group buttonStyle="solid"). */
export function Segmented({ value, onChange, options, className = '' }: SegmentedProps) {
  return (
    <div className={['ds-segmented', className].filter(Boolean).join(' ')} role="radiogroup">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={o.value === value}
          className={`ds-segmented__item${o.value === value ? ' is-active' : ''}`}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
