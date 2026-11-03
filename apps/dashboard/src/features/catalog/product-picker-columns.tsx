import type { ColumnDef } from '@/components/data-table/tanstack'
import type { PickerProduct } from './ProductPickerDialog'
import { DefaultToggle, EachVariantToggle, QuantityInput } from './ProductPickerFields'

// Per-pick field columns shared by the structure pickers (bundle, grouped, kit) - plain builders,
// not components, so this file stays fast-refresh friendly. Each input is disabled until its row
// is picked, and reads and writes the pick through usePickedExtra

export function quantityColumn(title = 'Quantity'): ColumnDef<PickerProduct> {
  return { id: 'quantity', size: 96, header: () => title, cell: ({ row }) => <QuantityInput id={row.original.id} label={title} /> }
}

export function defaultColumn(title = 'Default'): ColumnDef<PickerProduct> {
  return { id: 'isDefault', size: 72, header: () => title, cell: ({ row }) => <DefaultToggle id={row.original.id} /> }
}

export function eachVariantColumn(title = 'Each variant'): ColumnDef<PickerProduct> {
  return { id: 'eachVariant', size: 96, header: () => title, cell: ({ row }) => <EachVariantToggle id={row.original.id} kind={row.original.kind} /> }
}
