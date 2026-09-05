import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LeftOutlined, RightOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';

interface CalendarProps {
  value: Dayjs | null;
  onChange: (next: Dayjs | null) => void;
  showTime?: boolean;
  onDone?: () => void;
}

const WEEK_STARTS_MONDAY = 1;

export default function Calendar({ value, onChange, showTime = true, onDone }: CalendarProps) {
  const { t } = useTranslation();
  const [month, setMonth] = useState(() => (value ?? dayjs()).startOf('month'));

  const days = useMemo(() => {
    const lead = (month.day() - WEEK_STARTS_MONDAY + 7) % 7;
    const first = month.subtract(lead, 'day');
    return Array.from({ length: 42 }, (_, i) => first.add(i, 'day'));
  }, [month]);

  const weekdays = useMemo(() => days.slice(0, 7).map((d) => d.format('dd')), [days]);

  const pick = (day: Dayjs) => {
    const base = value ?? dayjs().startOf('day');
    onChange(day.hour(base.hour()).minute(base.minute()).second(base.second()));
  };

  const setPart = (part: 'hour' | 'minute', raw: string) => {
    const digits = raw.replace(/\D/g, '');
    if (digits === '') return;
    const n = Number(digits);
    if (!Number.isFinite(n)) return;
    const base = value ?? month.startOf('day');
    onChange(part === 'hour' ? base.hour(Math.min(23, Math.max(0, n))) : base.minute(Math.min(59, Math.max(0, n))));
  };

  return (
    <div className="cal">
      <div className="cal-head">
        <button type="button" className="cal-step" onClick={() => setMonth(month.subtract(1, 'month'))} aria-label="Previous month">
          <LeftOutlined />
        </button>
        <span className="cal-title">{month.format('MMMM YYYY')}</span>
        <button type="button" className="cal-step" onClick={() => setMonth(month.add(1, 'month'))} aria-label="Next month">
          <RightOutlined />
        </button>
      </div>

      <div className="cal-grid cal-grid--head">
        {weekdays.map((w, i) => <span key={`${w}-${i}`} className="cal-weekday">{w}</span>)}
      </div>

      <div className="cal-grid">
        {days.map((day) => {
          const outside = day.month() !== month.month();
          const chosen = !!value && day.isSame(value, 'day');
          const today = day.isSame(dayjs(), 'day');
          return (
            <button
              key={day.valueOf()}
              type="button"
              className={`cal-day${outside ? ' is-outside' : ''}${chosen ? ' is-chosen' : ''}${today ? ' is-today' : ''}`}
              onClick={() => pick(day)}
            >
              {day.date()}
            </button>
          );
        })}
      </div>

      {/* Text rather than number: a number field cannot show 05, and its
          spinners crowd a box this size. */}
      {showTime && (
        <div className="cal-time">
          <input
            type="text"
            inputMode="numeric"
            maxLength={2}
            className="ds-input"
            value={String(value ? value.hour() : 0).padStart(2, '0')}
            onChange={(e) => setPart('hour', e.target.value)}
            aria-label="Hour"
          />
          <span className="cal-colon">:</span>
          <input
            type="text"
            inputMode="numeric"
            maxLength={2}
            className="ds-input"
            value={String(value ? value.minute() : 0).padStart(2, '0')}
            onChange={(e) => setPart('minute', e.target.value)}
            aria-label="Minute"
          />
        </div>
      )}

      <div className="cal-foot">
        <button type="button" className="cal-link" onClick={() => onChange(null)}>
          {t('clear', { defaultValue: 'Clear' })}
        </button>
        <button
          type="button"
          className="cal-link"
          onClick={() => { const now = dayjs(); setMonth(now.startOf('month')); onChange(now); }}
        >
          {t('today', { defaultValue: 'Today' })}
        </button>
        <button type="button" className="cal-done" onClick={onDone}>
          {t('done', { defaultValue: 'Done' })}
        </button>
      </div>
    </div>
  );
}
