import { Checkbox } from '@/components/ui/checkbox'
import type { ColumnDef, RowData } from '@/components/data-table/tanstack'

// Split from DataTable so that file exports only the component - fast refresh needs the separation

// Reusable selection column - drop in as the first entry of your `columns`
// array when you want row selection (required for bulk actions)
export function selectColumn<TData extends RowData>(): ColumnDef<TData> {
  return {
    id: 'select',
    header: ({ table }) => (
      <Checkbox
        checked={
          table.getIsAllPageRowsSelected() ||
          (table.getIsSomePageRowsSelected() && 'indeterminate')
        }
        onCheckedChange={(v) => table.toggleAllPageRowsSelected(!!v)}
        aria-label="Select all"
      />
    ),
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        onCheckedChange={(v) => row.toggleSelected(!!v)}
        aria-label="Select row"
      />
    ),
    enableSorting: false,
    enableHiding: false,
    size: 32,
  }
}
