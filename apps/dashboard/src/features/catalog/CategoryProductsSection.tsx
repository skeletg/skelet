import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useApolloClient, useMutation } from '@apollo/client/react'
import { PlusIcon, Trash2Icon, Undo2Icon } from 'lucide-react'
import {
  DataTable,
  DataTableColumnHeader,
  imageColumn,
  selectColumn,
  type BulkAction,
  type RowAction,
} from '@/components/data-table'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { added, removed } from '@/shared/saving/diff'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { cn } from '@/lib/utils'
import { AddProductToCategoryDocument, RemoveProductFromCategoryDocument } from './Catalog.gql'
import { CategoryDocument, type CategoryDetailFieldsFragment } from './Categories.gql'
import { ProductPickerDialog, type PickerProduct } from './ProductPickerDialog'
import { productLabel } from './category-products'
import { Section } from './CategorySection'

// The fields a row renders - what the category query and the picker both carry
type PickedProduct = Pick<PickerProduct, 'id' | 'name' | 'sku' | 'kind' | 'thumbnail' | 'displayImage'>
type Row = PickedProduct & { staged: 'add' | 'remove' | null }

// The category's product assignments, staged like every other section: the dialog's picks and the
// row removals only mark the draft, and the page-level Save applies them after the confirm dialog
export function CategoryProductsSection({
  category,
  canManage,
}: {
  category: Pick<CategoryDetailFieldsFragment, 'id' | 'slug' | 'name' | 'products'>
  canManage: boolean
}) {
  const client = useApolloClient()
  const [addToCategory] = useMutation(AddProductToCategoryDocument)
  const [removeFromCategory] = useMutation(RemoveProductFromCategoryDocument)

  const [pendingAdd, setPendingAdd] = useState<Map<string, PickedProduct>>(() => new Map())
  const [pendingRemove, setPendingRemove] = useState<Set<string>>(() => new Set())
  const [pickerOpen, setPickerOpen] = useState(false)

  // Reset on entity identity only - a refetch of the same category must not wipe staged work
  const [prevId, setPrevId] = useState(category.id)
  if (prevId !== category.id) {
    setPrevId(category.id)
    setPendingAdd(new Map())
    setPendingRemove(new Set())
  }

  const assigned = category.products
  const rows = useMemo<Row[]>(() => {
    const assignedIds = new Set(assigned.map((p) => p.id))
    return [
      ...[...pendingAdd.values()]
        .filter((p) => !assignedIds.has(p.id))
        .map((p) => ({ ...p, staged: 'add' as const })),
      ...assigned.map((p) => ({ ...p, staged: pendingRemove.has(p.id) ? ('remove' as const) : null })),
    ]
  }, [assigned, pendingAdd, pendingRemove])

  // A row staged for removal stays pickable, so ticking it again in the dialog undoes the removal
  const takenIds = useMemo(() => new Set(rows.filter((r) => r.staged !== 'remove').map((r) => r.id)), [rows])

  const stageRemove = useCallback((targets: Row[]) => {
    const adds = targets.filter((r) => r.staged === 'add').map((r) => r.id)
    const removes = targets.filter((r) => r.staged !== 'add').map((r) => r.id)
    if (adds.length) {
      setPendingAdd((prev) => {
        const next = new Map(prev)
        for (const id of adds) next.delete(id)
        return next
      })
    }
    if (removes.length) setPendingRemove((prev) => new Set([...prev, ...removes]))
  }, [])

  const undo = useCallback((row: Row) => {
    if (row.staged === 'add') {
      setPendingAdd((prev) => {
        const next = new Map(prev)
        next.delete(row.id)
        return next
      })
    } else {
      setPendingRemove((prev) => {
        const next = new Set(prev)
        next.delete(row.id)
        return next
      })
    }
  }, [])

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      rows
        .filter((r) => r.staged)
        .map((r) => (r.staged === 'add' ? added(productLabel(r), r.sku) : removed(productLabel(r), r.sku))),
    [rows],
  )

  const save = useCallback(async () => {
    const adds = rows.filter((r) => r.staged === 'add')
    const removes = rows.filter((r) => r.staged === 'remove')
    const results = await Promise.allSettled([
      ...adds.map((r) => addToCategory({ variables: { productId: r.id, categoryId: category.id } })),
      ...removes.map((r) => removeFromCategory({ variables: { productId: r.id, categoryId: category.id } })),
    ])
    const failed = [...adds, ...removes].filter((_, i) => results[i].status === 'rejected')
    // One read after the batch instead of a refetch per mutation - the mutations answer with the
    // product, not the category, so the list has to be read again either way
    await client.query({ query: CategoryDocument, variables: { id: category.id }, fetchPolicy: 'network-only' })
    // Keep only what failed staged, so a retry sends exactly those
    setPendingAdd(new Map(failed.filter((r) => r.staged === 'add').map((r) => [r.id, r])))
    setPendingRemove(new Set(failed.filter((r) => r.staged === 'remove').map((r) => r.id)))
    if (failed.length) {
      throw new Error(
        `Could not update ${failed.length} product${failed.length === 1 ? '' : 's'}: ${failed.map(productLabel).join(', ')}`,
      )
    }
  }, [rows, addToCategory, removeFromCategory, client, category.id])

  useSaveSection({ id: 'category-products', title: 'Products', diff, save })

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      ...(canManage ? [selectColumn<Row>()] : []),
      imageColumn<Row>({ image: (r) => r.thumbnail ?? r.displayImage, alt: productLabel, size: 32 }),
      {
        id: 'name',
        accessorFn: (r) => `${productLabel(r)} ${r.sku ?? ''}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Product" />,
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <Link
              to={`/products/${row.original.id}`}
              className={cn(
                'truncate font-medium hover:underline',
                row.original.staged === 'remove' && 'text-muted-foreground line-through',
              )}
            >
              {productLabel(row.original)}
            </Link>
            {row.original.staged === 'add' ? (
              <span className="shrink-0 text-xs text-success">Will add</span>
            ) : row.original.staged === 'remove' ? (
              <span className="shrink-0 text-xs text-destructive">Will remove</span>
            ) : null}
          </div>
        ),
      },
      {
        accessorKey: 'sku',
        header: ({ column }) => <DataTableColumnHeader column={column} title="SKU" />,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.sku ?? '-'}</span>
        ),
      },
      {
        accessorKey: 'kind',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Kind" />,
        cell: ({ row }) => (
          <Badge variant={row.original.kind === 'SIMPLE' ? 'secondary' : 'outline'} className="font-normal capitalize">
            {row.original.kind.toLowerCase()}
          </Badge>
        ),
      },
    ],
    [canManage],
  )

  const rowActions = useMemo<RowAction<Row>[]>(
    () =>
      canManage
        ? [
            { id: 'view', label: 'Open product', to: (r) => `/products/${r.id}` },
            {
              id: 'remove',
              label: 'Remove from category',
              icon: Trash2Icon,
              destructive: true,
              separatorBefore: true,
              hidden: (r) => r.staged !== null,
              onSelect: (r) => stageRemove([r]),
            },
            {
              id: 'undo',
              label: 'Undo',
              icon: Undo2Icon,
              separatorBefore: true,
              hidden: (r) => r.staged === null,
              onSelect: undo,
            },
          ]
        : [],
    [canManage, stageRemove, undo],
  )

  const bulkActions = useMemo<BulkAction<Row>[]>(
    () => [
      {
        id: 'remove',
        label: 'Remove selected',
        icon: Trash2Icon,
        variant: 'outline',
        supportsAllMatching: false,
        disabled: (sel) => sel.every((r) => r.staged === 'remove'),
        perform: async (sel) => stageRemove(sel.filter((r) => r.staged !== 'remove')),
      },
    ],
    [stageRemove],
  )

  const categoryName = category.name || category.slug
  const addButton = canManage ? (
    <Button type="button" size="sm" onClick={() => setPickerOpen(true)}>
      <PlusIcon className="size-4" />
      Add products
    </Button>
  ) : null

  const liveCount = rows.filter((r) => r.staged !== 'remove').length

  return (
    <Section
      title={`Products (${liveCount})`}
      description="Products assigned directly to this category. Changes apply when you save the page."
      actions={rows.length > 0 ? addButton : null}
    >
      {rows.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-md border border-dashed bg-muted/20 px-6 py-10 text-center">
          <p className="text-sm text-muted-foreground">No products are in this category yet.</p>
          {addButton}
        </div>
      ) : (
        <DataTable
          columns={columns}
          data={rows}
          getRowId={(r) => r.id}
          pageSize={10}
          searchPlaceholder="Filter products…"
          bulkActions={canManage ? bulkActions : undefined}
          rowActions={rowActions}
          rowActionLabel={productLabel}
        />
      )}

      {canManage ? (
        <ProductPickerDialog
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          title={`Add products to ${categoryName}`}
          description="Tick every product that belongs here. Nothing is saved until you save the page."
          takenIds={takenIds}
          takenLabel="In category"
          onConfirm={(picks) => {
            const picked = picks.map((p) => p.product)
            // Re-picking something staged for removal just cancels the removal
            const cancelled = picked.filter((p) => pendingRemove.has(p.id)).map((p) => p.id)
            if (cancelled.length) {
              setPendingRemove((prev) => new Set([...prev].filter((id) => !cancelled.includes(id))))
            }
            setPendingAdd((prev) => {
              const next = new Map(prev)
              for (const p of picked) if (!cancelled.includes(p.id)) next.set(p.id, p)
              return next
            })
          }}
        />
      ) : null}
    </Section>
  )
}
