import { useState } from 'react';
import * as RPopover from '@radix-ui/react-popover';
import { CalendarOutlined } from '@ant-design/icons';
import type { Dayjs } from 'dayjs';

import Calendar from './Calendar';
interface DateTimePickerProps {
  value: Dayjs | null;
  onChange: (next: Dayjs | null) => void;
  showTime?: boolean;
  format?: string;
  placeholder?: string;
  disabled?: boolean;
}

export default function DateTimePicker({
  value,
  onChange,
  showTime = true,
  placeholder = '',
  disabled = false,
}: DateTimePickerProps) {
  const [open, setOpen] = useState(false);

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
