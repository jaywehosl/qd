import { LeftOutlined, RightOutlined } from '@ant-design/icons';
import { Select } from './Select';

export interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number, pageSize: number) => void;
  pageSizeOptions?: number[];
  showTotal?: (total: number, range: [number, number]) => string;
}

function pageList(current: number, totalPages: number): (number | 'gap')[] {
  if (totalPages <= 7) return Array.from({ length: totalPages }, (_, i) => i + 1);
  const out: (number | 'gap')[] = [1];
  const start = Math.max(2, current - 1);
  const end = Math.min(totalPages - 1, current + 1);
  if (start > 2) out.push('gap');
  for (let p = start; p <= end; p++) out.push(p);
  if (end < totalPages - 1) out.push('gap');
  out.push(totalPages);
  return out;
}

export function Pagination({
  page,
  pageSize,
  total,
  onChange,
  pageSizeOptions = [10, 25, 50, 100],
  showTotal,
}: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const current = Math.min(Math.max(1, page), totalPages);
  const from = total === 0 ? 0 : (current - 1) * pageSize + 1;
  const to = Math.min(current * pageSize, total);

  function go(p: number) {
    const next = Math.min(Math.max(1, p), totalPages);
    if (next !== current) onChange(next, pageSize);
  }

  return (
    <div className="ds-pagination">
      {/* Only when a caller asks for it: pages that already print their own
          "showing X of Y" would otherwise say the same thing twice. */}
      {showTotal && (
        <span className="ds-pagination__total">{showTotal(total, [from, to])}</span>
      )}

      <div className="ds-pagination__pages">
        <button className="ds-pager__btn" disabled={current <= 1} onClick={() => go(current - 1)} aria-label="Previous">
          <LeftOutlined />
        </button>
        {pageList(current, totalPages).map((p, i) =>
          p === 'gap' ? (
            <span key={`gap-${i}`} className="ds-pager__gap">…</span>
          ) : (
            <button
              key={p}
              className={`ds-pager__btn${p === current ? ' is-active' : ''}`}
              onClick={() => go(p)}
            >
              {p}
            </button>
          ),
        )}
        <button className="ds-pager__btn" disabled={current >= totalPages} onClick={() => go(current + 1)} aria-label="Next">
          <RightOutlined />
        </button>
      </div>

      <div className="ds-pagination__size">
        <Select
          value={String(pageSize)}
          onChange={(v) => onChange(1, Number(v))}
          options={pageSizeOptions.map((n) => ({ value: String(n), label: `${n} / page` }))}
        />
      </div>
    </div>
  );
}
