import { useMemo, useState } from 'react';
import * as RPopover from '@radix-ui/react-popover';
import { CalendarOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import { PersianDateTimePicker } from 'persian-calendar-suite';

import { useDatepicker } from '@/hooks/useDatepicker';
import { useTheme } from '@/hooks/useTheme';
import Calendar from './Calendar';
interface DateTimePickerProps {
  value: Dayjs | null;
  onChange: (next: Dayjs | null) => void;
  showTime?: boolean;
  /** Retained for API compatibility; the gregorian native input formats itself. */
  format?: string;
  placeholder?: string;
  disabled?: boolean;
}

const LIGHT_THEME = {
  primaryColor: '#1677ff',
  backgroundColor: '#ffffff',
  borderColor: '#d9d9d9',
  hoverColor: 'rgba(22, 119, 255, 0.10)',
  selectedTextColor: '#ffffff',
  textColor: 'rgba(0, 0, 0, 0.88)',
};

const DARK_THEME = {
  primaryColor: '#1677ff',
  backgroundColor: '#23252b',
  borderColor: 'rgba(255, 255, 255, 0.12)',
  hoverColor: 'rgba(22, 119, 255, 0.18)',
  selectedTextColor: '#ffffff',
  textColor: 'rgba(255, 255, 255, 0.88)',
};

const ULTRA_DARK_THEME = {
  primaryColor: '#1677ff',
  backgroundColor: '#101013',
  borderColor: 'rgba(255, 255, 255, 0.08)',
  hoverColor: 'rgba(22, 119, 255, 0.16)',
  selectedTextColor: '#ffffff',
  textColor: 'rgba(255, 255, 255, 0.88)',
};

export default function DateTimePicker({
  value,
  onChange,
  showTime = true,
  placeholder = '',
  disabled = false,
}: DateTimePickerProps) {
  const { datepicker } = useDatepicker();
  const { isDark, isUltra } = useTheme();
  const [open, setOpen] = useState(false);

  const persianTheme = useMemo(() => {
    if (isUltra) return ULTRA_DARK_THEME;
    if (isDark) return DARK_THEME;
    return LIGHT_THEME;
  }, [isDark, isUltra]);

  if (datepicker === 'jalalian') {
    return (
      <div className={`jdp-wrap${isDark ? ' jdp-dark' : ''}${isUltra ? ' jdp-ultra' : ''}${disabled ? ' jdp-disabled' : ''}`}>
        <PersianDateTimePicker
          value={value ? value.valueOf() : null}
          onChange={(next: number | string | null) => {
            if (next == null || next === '') {
              onChange(null);
              return;
            }
            const ms = typeof next === 'number' ? next : Number(next);
            if (Number.isFinite(ms)) onChange(dayjs(ms));
          }}
          showTime={showTime}
          outputFormat="timestamp"
          persianNumbers
          rtlCalendar
          theme={persianTheme}
        />
      </div>
    );
  }

  const shown = value && value.isValid()
    ? value.format(showTime ? 'DD.MM.YYYY HH:mm' : 'DD.MM.YYYY')
    : '';

  return (
    <RPopover.Root open={open} onOpenChange={(o) => !disabled && setOpen(o)}>
      <RPopover.Trigger asChild>
        <button type="button" className="ds-input dt-field" disabled={disabled}>
          <span className={shown ? undefined : 'dt-field__hint'}>{shown || placeholder || '—'}</span>
          <CalendarOutlined className="dt-field__icon" />
        </button>
      </RPopover.Trigger>
      <RPopover.Portal>
        <RPopover.Content className="ds-popover" side="bottom" align="start" sideOffset={6} collisionPadding={8}>
          <Calendar
            value={value}
            onChange={onChange}
            showTime={showTime}
            onDone={() => setOpen(false)}
          />
        </RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}
