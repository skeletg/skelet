import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useApolloClient, useMutation } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, PlusIcon, Trash2Icon, Undo2Icon } from 'lucide-react'
import {
  DataTable,
  DataTableColumnHeader,
  imageColumn,
  selectColumn,
  type BulkAction,
  type RowAction,
} from '@/components/data-table'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { formatCents } from '@/lib/format'
import { cn } from '@/lib/utils'
import { added, changed, removed } from '@/shared/saving/diff'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  AddProductToCollectionDocument,
  CollectionDocument,
  RemoveProductFromCollectionDocument,
  ReorderCollectionProductsDocument,
  type CollectionQuery,
} from './Collections.gql'
import { productLabel } from './category-products'
import { ProductPickerDialog, type PickerProduct } from './ProductPickerDialog'

type CollectionRecord = NonNullable<CollectionQuery['collection']>
type Product = Pick<PickerProduct, 'id' | 'name' | 'sku' | 'defaultPrice' | 'defaultCurrencyCode' | 'thumbnail'>
type Row = Product & { position: number; staged: 'add' | 'remove' | null }

// Staged membership: order (the storefront renders the collection in it), picks not yet saved,
// and removals. Null means "no edits" - the rows follow the server, so a refetch after save
// needs no draft reconciliation
type Draft = { order: string[]; added: Map<string, Product>; removed: Set<string> }

// The collection's products, in storefront order - adds, removals and moves stage like every other
// section and apply through the page-level Save
export function CollectionProductsSection({
  collection,
  canManage,
}: {
  collection: CollectionRecord
  canManage: boolean
}) {
  const client = useApolloClient()
  const [addProduct] = useMutation(AddProductToCollectionDocument)
  const [removeProduct] = useMutation(RemoveProductFromCollectionDocument)
  const [reorderProducts] = useMutation(ReorderCollectionProductsDocument)
  const [pickerOpen, setPickerOpen] = useState(false)

  const server = collection.products
  const [draft, setDraft] = useState<Draft | null>(null)
  // Reset on entity identity only - a refetch of the same collection must not wipe staged work
  const [prevId, setPrevId] = useState(collection.id)
  if (prevId !== collection.id) {
    setPrevId(collection.id)
    setDraft(null)
  }

  const edit = (fn: (d: Draft) => Draft) =>
    setDraft((prev) => fn(prev ?? { order: server.map((p) => p.id), added: new Map(), removed: new Set() }))

  const byId = useMemo(() => {
    const m = new Map<string, Product>(server.map((p) => [p.id, p]))
    for (const [id, p] of draft?.added ?? []) m.set(id, p)
    return m
  }, [server, draft])

  const rows = useMemo<Row[]>(() => {
    const order = draft?.order ?? server.map((p) => p.id)
    let position = 0
    return order.flatMap((id) => {
      const product = byId.get(id)
      if (!product) return []
      const staged = draft?.added.has(id) ? 'add' : draft?.removed.has(id) ? 'remove' : null
      // Rows staged for removal keep their place but take no position - they won't be there
      return [{ ...product, staged, position: staged === 'remove' ? 0 : ++position }]
    })
  }, [draft, server, byId])

  const finalIds = useMemo(() => rows.filter((r) => r.staged !== 'remove').map((r) => r.id), [rows])
  // Where the server puts things without a reorder: survivors in their order, adds appended
  const naturalIds = useMemo(
    () => [
      ...server.filter((p) => !draft?.removed.has(p.id)).map((p) => p.id),
      ...[...(draft?.added.keys() ?? [])],
    ],
    [server, draft],
  )
  const orderChanged = finalIds.join() !== naturalIds.join()

  const move = useCallback(
    (id: string, to: 'up' | 'down' | 'top' | 'bottom') =>
      edit((d) => {
        const order = [...d.order]
        const from = order.indexOf(id)
        if (from < 0) return d
        order.splice(from, 1)
        const target =
          to === 'top' ? 0 : to === 'bottom' ? order.length : to === 'up' ? Math.max(0, from - 1) : Math.min(order.length, from + 1)
        order.splice(target, 0, id)
        return { ...d, order }
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [server],
  )

  const stageRemove = useCallback(
    (targets: Row[]) =>
      edit((d) => {
        const addedNext = new Map(d.added)
        const removedNext = new Set(d.removed)
        const dropped = new Set<string>()
        for (const r of targets) {
          // A staged add that's removed again simply never happened
          if (addedNext.delete(r.id)) dropped.add(r.id)
          else removedNext.add(r.id)
        }
        return { order: d.order.filter((id) => !dropped.has(id)), added: addedNext, removed: removedNext }
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [server],
  )

  const undo = useCallback(
    (row: Row) => {
      if (row.staged === 'add') stageRemove([row])
      else
        edit((d) => {
          const removedNext = new Set(d.removed)
          removedNext.delete(row.id)
          return { ...d, removed: removedNext }
        })
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [stageRemove, server],
  )

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = rows
      .filter((r) => r.staged)
      .map((r) => (r.staged === 'add' ? added(productLabel(r), r.sku) : removed(productLabel(r), r.sku)))
    if (orderChanged) {
      const names = (ids: string[]) => ids.map((id) => productLabel(byId.get(id) ?? { id })).join(', ')
      const entry = changed('Product order', names(naturalIds), names(finalIds))
      if (entry) out.push(entry)
    }
    return out
  }, [rows, orderChanged, naturalIds, finalIds, byId])

  const save = useCallback(async () => {
    if (!draft) return
    const collectionId = collection.id
    const removes = [...draft.removed]
    const adds = [...draft.added.values()]
    const failed = { removes: new Set<string>(), adds: new Map<string, Product>() }
    const errors: string[] = []

    const removeResults = await Promise.allSettled(
      removes.map((productId) => removeProduct({ variables: { collectionId, productId } })),
    )
    removeResults.forEach((r, i) => {
      if (r.status === 'rejected') {
        failed.removes.add(removes[i])
        errors.push(productLabel(byId.get(removes[i]) ?? { id: removes[i] }))
      }
    })
    // One at a time, so the server appends them in the order they were picked
    for (const p of adds) {
      try {
        await addProduct({ variables: { collectionId, productId: p.id } })
      } catch {
        failed.adds.set(p.id, p)
        errors.push(productLabel(p))
      }
    }
    const allApplied = errors.length === 0
    if (allApplied && orderChanged) {
      await reorderProducts({ variables: { collectionId, productIds: finalIds } })
    }

    const fresh = await client.query({ query: CollectionDocument, variables: { id: collectionId }, fetchPolicy: 'network-only' })
    if (allApplied) {
      setDraft(null)
      return
    }
    // Keep exactly what failed staged, on top of what the server now holds, so a retry sends only that
    const serverIds = (fresh.data?.collection?.products ?? []).map((p) => p.id)
    setDraft({ order: [...serverIds, ...failed.adds.keys()], added: failed.adds, removed: failed.removes })
    throw new Error(`Could not update ${errors.length} product${errors.length === 1 ? '' : 's'}: ${errors.join(', ')}`)
  }, [draft, collection.id, byId, orderChanged, finalIds, addProduct, removeProduct, reorderProducts, client])

  useSaveSection({ id: 'collection-products', title: 'Products', diff, save })

  const liveCount = finalIds.length

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      ...(canManage ? [selectColumn<Row>()] : []),
      {
        id: 'position',
        header: () => '#',
        size: 40,
        cell: ({ row }) => (
          <span className="tabular-nums text-muted-foreground">{row.original.position || ''}</span>
        ),
      },
      imageColumn<Row>({ image: (r) => r.thumbnail, alt: productLabel, size: 32 }),
      {
        id: 'name',
        accessorFn: (r) => `${productLabel(r)} ${r.sku ?? ''}`,
        header: () => 'Product',
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
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.sku ?? '-'}</span>
        ),
      },
      {
        id: 'price',
        header: () => <span className="block text-right">Price</span>,
        cell: ({ row }) => (
          <span className="block text-right tabular-nums">
            {formatCents(row.original.defaultPrice, row.original.defaultCurrencyCode)}
          </span>
        ),
      },
      ...(canManage
        ? [
            {
              id: 'move',
              size: 72,
              header: () => null,
              cell: ({ row, table }) => {
                const index = row.index
                const last = table.options.data.length - 1
                return (
                  <div className="flex justify-end gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      aria-label={`Move ${productLabel(row.original)} up`}
                      disabled={index <= 0}
                      onClick={() => move(row.original.id, 'up')}
                    >
                      <ArrowUpIcon className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      aria-label={`Move ${productLabel(row.original)} down`}
                      disabled={index === last}
                      onClick={() => move(row.original.id, 'down')}
                    >
                      <ArrowDownIcon className="size-3.5" />
                    </Button>
                  </div>
                )
              },
            } satisfies ColumnDef<Row>,
          ]
        : []),
    ],
    [canManage, move],
  )

  const rowActions = useMemo<RowAction<Row>[]>(
    () =>
      canManage
        ? [
            { id: 'view', label: 'Open product', to: (r) => `/products/${r.id}` },
            { id: 'top', label: 'Move to top', separatorBefore: true, onSelect: (r) => move(r.id, 'top') },
            { id: 'bottom', label: 'Move to bottom', onSelect: (r) => move(r.id, 'bottom') },
            {
              id: 'remove',
              label: 'Remove from collection',
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
    [canManage, move, stageRemove, undo],
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

  // A row staged for removal stays pickable, so ticking it again undoes the removal
  const takenIds = useMemo(() => new Set(finalIds), [finalIds])

  const addButton = canManage ? (
    <Button type="button" size="sm" onClick={() => setPickerOpen(true)}>
      <PlusIcon className="size-4" />
      Add products
    </Button>
  ) : null

  return (
    <DetailPageSection
      title={`Products (${liveCount})`}
      description="Products in this collection, in the order the storefront shows them. Changes apply when you save the page."
      trailing={rows.length > 0 ? addButton : null}
    >
      {rows.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-md border border-dashed bg-muted/20 px-6 py-10 text-center">
          <p className="text-sm text-muted-foreground">No products are in this collection yet.</p>
          {addButton}
        </div>
      ) : (
        <DataTable
          columns={columns}
          data={rows}
          getRowId={(r) => r.id}
          pageSize={25}
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
          title={`Add products to ${collection.name || collection.slug}`}
          description="Picked products join the end of the collection. Nothing is saved until you save the page."
          takenIds={takenIds}
          takenLabel="In collection"
          showPrice
          onConfirm={(picks) =>
            edit((d) => {
              const addedNext = new Map(d.added)
              const removedNext = new Set(d.removed)
              const order = [...d.order]
              for (const { product } of picks) {
                // Re-picking a row staged for removal just cancels the removal
                if (removedNext.delete(product.id)) continue
                addedNext.set(product.id, product)
                order.push(product.id)
              }
              return { order, added: addedNext, removed: removedNext }
            })
          }
        />
      ) : null}
    </DetailPageSection>
  )
}
