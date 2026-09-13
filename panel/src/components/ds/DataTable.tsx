import { Fragment, useState } from 'react';
import type { ReactNode } from 'react';
import { RightOutlined } from '@ant-design/icons';
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getPaginationRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table';
import { Pagination, type PaginationProps } from './Pagination';
import { EmptyState } from './EmptyState';


export type { ColumnDef, SortingState } from '@tanstack/react-table';

export interface DataTableRowSelection<T> {
  selectedIds: string[];
  onChange: (ids: string[]) => void;
  isSelectable?: (row: T) => boolean;
}

export interface DataTableExpandable<T> {
  render: (row: T) => ReactNode;
  isExpandable?: (row: T) => boolean;
}

export interface DataTableProps<T> {
  data: T[];
  columns: ColumnDef<T, unknown>[];
  getRowId?: (row: T, index: number) => string;
  empty?: ReactNode;
  sortable?: boolean;
  sorting?: SortingState;
  onSortingChange?: (sorting: SortingState) => void;
  paginateRows?: boolean;
  rowSelection?: DataTableRowSelection<T>;
  expandable?: DataTableExpandable<T>;
  pagination?: PaginationProps;
}

export function DataTable<T>({
  data,
  columns,
  getRowId,
  empty,
  sortable = true,
  sorting: sortingProp,
  onSortingChange,
  paginateRows,
  rowSelection,
  expandable,
  pagination,
}: DataTableProps<T>) {
  const [ownSorting, setOwnSorting] = useState<SortingState>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const controlled = sortingProp !== undefined;
  const sorting = controlled ? sortingProp : ownSorting;
  const setSorting = controlled ? (onSortingChange ?? (() => {})) : setOwnSorting;

  const table = useReactTable({
    data,
    columns,
    state: {
      sorting,
      ...(paginateRows && pagination
        ? { pagination: { pageIndex: Math.max(0, pagination.page - 1), pageSize: pagination.pageSize } }
        : {}),
    },
    onSortingChange: (updater) => {
      setSorting(typeof updater === 'function' ? updater(sorting) : updater);
    },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: sortable && !controlled ? getSortedRowModel() : undefined,
    getPaginationRowModel: paginateRows ? getPaginationRowModel() : undefined,
    manualSorting: controlled,
    enableSorting: sortable,
    getRowId,
  });

  const rows = table.getRowModel().rows;
  const leadCols = (rowSelection ? 1 : 0) + (expandable ? 1 : 0);
  const totalCols = columns.length + leadCols;

  const selectableRows = rowSelection
    ? rows.filter((r) => !rowSelection.isSelectable || rowSelection.isSelectable(r.original))
    : [];
  const allSelected = rowSelection != null
    && selectableRows.length > 0
    && selectableRows.every((r) => rowSelection.selectedIds.includes(r.id));

  function toggleAll() {
    if (!rowSelection) return;
    const ids = selectableRows.map((r) => r.id);
    rowSelection.onChange(allSelected ? [] : ids);
  }

  function toggleRow(id: string) {
    if (!rowSelection) return;
    const set = new Set(rowSelection.selectedIds);
    if (set.has(id)) set.delete(id);
    else set.add(id);
    rowSelection.onChange([...set]);
  }

  function toggleExpand(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <>
    <div className="ds-table-wrap">
      <table className="ds-table">
        <thead>
          {table.getHeaderGroups().map((hg) => (
            <tr key={hg.id}>
              {expandable && <th style={{ width: 36 }} />}
              {rowSelection && (
                <th style={{ width: 44 }}>
                  <input
                    type="checkbox"
                    className="ds-check"
                    checked={allSelected}
                    onChange={toggleAll}
                    aria-label="Select all"
                  />
                </th>
              )}
              {hg.headers.map((header) => {
                const canSort = header.column.getCanSort();
                const sorted = header.column.getIsSorted();
                return (
                  <th
                    key={header.id}
                    className={canSort ? 'ds-th--sortable' : undefined}
                    style={{ width: header.getSize() !== 150 ? header.getSize() : undefined }}
                    onClick={canSort ? header.column.getToggleSortingHandler() : undefined}
                  >
                    {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                    {canSort && sorted && (
                      <span className="ds-table__sort">{sorted === 'asc' ? '▲' : '▼'}</span>
                    )}
                  </th>
                );
              })}
            </tr>
          ))}
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={totalCols}>
                <div className="ds-table__empty">{empty ?? <EmptyState />}</div>
              </td>
            </tr>
          ) : (
            rows.map((row) => {
              const canExpand = expandable && (!expandable.isExpandable || expandable.isExpandable(row.original));
              const isOpen = expanded.has(row.id);
              const selectable = !rowSelection?.isSelectable || rowSelection.isSelectable(row.original);
              return (
                <Fragment key={row.id}>
                  <tr data-row-id={row.id}>
                    {expandable && (
                      <td style={{ width: 36 }}>
                        {canExpand && (
                          <button
                            type="button"
                            className={`ds-table__expand-btn${isOpen ? ' is-open' : ''}`}
                            onClick={() => toggleExpand(row.id)}
                            aria-label="Toggle row"
                          >
                            <RightOutlined />
                          </button>
                        )}
                      </td>
                    )}
                    {rowSelection && (
                      <td style={{ width: 44 }}>
                        <input
                          type="checkbox"
                          className="ds-check"
                          checked={rowSelection.selectedIds.includes(row.id)}
                          disabled={!selectable}
                          onChange={() => toggleRow(row.id)}
                          aria-label="Select row"
                        />
                      </td>
                    )}
                    {row.getVisibleCells().map((cell) => (
                      <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
                    ))}
                  </tr>
                  {canExpand && isOpen && (
                    <tr>
                      <td className="ds-table__expanded-cell" colSpan={totalCols}>
                        {expandable!.render(row.original)}
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })
          )}
        </tbody>
      </table>
    </div>
    {pagination && <Pagination {...pagination} />}
    </>
  );
}
