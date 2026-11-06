import { Column } from '@tanstack/react-table'

export function isFilterableColumn<TData>(
  column: Column<TData, unknown>,
  options: { requireOptIn?: boolean; requireFacetedValues?: boolean } = {}
) {
  if (!column.getCanFilter()) return false
  if (options.requireOptIn && column.columnDef.enableColumnFilter !== true) {
    return false
  }
  if (options.requireFacetedValues === false) return true

  const facetedValues = column.getFacetedUniqueValues()
  if (facetedValues.size === 0) return false

  return !Array.from(facetedValues.keys()).every(
    (value) => value != null && typeof value === 'object'
  )
}

export function getColumnFilterLabel<TData>(
  column: Column<TData, unknown>,
  t: (key: string) => string
) {
  return typeof column.columnDef.header === 'string'
    ? column.columnDef.header
    : (column.columnDef.meta as { filterLabel?: string } | undefined)
        ?.filterLabel || t('common.filter')
}
