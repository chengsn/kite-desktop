import { useState } from 'react'
import { Column, Table } from '@tanstack/react-table'
import { ListFilter } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { FacetedFilterContent } from './column-filter-popover'
import { getColumnFilterLabel, isFilterableColumn } from './column-filter-utils'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'

function hasActiveFilter<TData>(column: Column<TData, unknown>) {
  const value = column.getFilterValue()
  if (Array.isArray(value)) return value.length > 0
  return value !== undefined && value !== null && value !== ''
}

function FilterSection<TData>({ column }: { column: Column<TData, unknown> }) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const value = column.getFilterValue()
  const selectedCount = Array.isArray(value) ? value.length : 0

  return (
    <section className="space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <h4 className="truncate text-xs font-medium">
          {getColumnFilterLabel(column, t)}
        </h4>
        {selectedCount > 0 ? (
          <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">
            {selectedCount}
          </span>
        ) : null}
      </div>
      <FacetedFilterContent
        column={column}
        search={search}
        onSearchChange={setSearch}
        hasActiveFilter={hasActiveFilter(column)}
        t={t}
      />
    </section>
  )
}

export function TableFiltersPopover<TData>({
  table,
  className,
}: {
  table: Table<TData>
  className?: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  // The filter button must stay reachable even when the current result set is
  // empty (for example after a namespace switch), otherwise active filters
  // cannot be cleared. Opt-in columns keep the old toolbar semantics; the
  // active count also includes filters set from the per-column header icons.
  const filterableColumns = table
    .getAllLeafColumns()
    .filter(
      (column) =>
        column.id !== 'select' &&
        column.id !== 'actions' &&
        column.getCanFilter()
    )
  const columns = filterableColumns.filter((column) =>
    isFilterableColumn(column, {
      requireOptIn: true,
      requireFacetedValues: false,
    })
  )
  const activeCount = filterableColumns.filter(hasActiveFilter).length

  if (columns.length === 0 && activeCount === 0) {
    return null
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          className={cn('gap-2', className)}
          aria-label={t('resourceTable.filters', 'Filters')}
        >
          <ListFilter className="h-4 w-4" />
          <span>{t('resourceTable.filters', 'Filters')}</span>
          {activeCount > 0 ? (
            <Badge
              variant="secondary"
              className="h-5 min-w-5 px-1 text-[10px] tabular-nums"
            >
              {activeCount}
            </Badge>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="max-h-[70vh] w-80 overflow-y-auto p-3"
        align="start"
      >
        <div className="space-y-4">
          {columns.map((column) => (
            <FilterSection key={column.id} column={column} />
          ))}
          {activeCount > 0 ? (
            <Button
              variant="ghost"
              size="sm"
              className="w-full"
              onClick={() => {
                filterableColumns.forEach((column) =>
                  column.setFilterValue(undefined)
                )
              }}
            >
              {t('resourceTable.clearAllFilters', 'Clear all filters')}
            </Button>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  )
}
