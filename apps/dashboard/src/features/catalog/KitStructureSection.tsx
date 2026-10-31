import { useCallback, useMemo, useState, type FormEvent } from 'react'
import { useMutation } from '@apollo/client/react'
import { PackagePlusIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatCents } from '@/lib/format'
import {
  AddKitComponentDocument,
  ProductDocument,
  RemoveKitComponentDocument,
  UpdateKitComponentDocument,
  UpdateProductDocument,
} from './Catalog.gql'
import { changed } from '@/shared/saving/diff'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
// Kit components share the grouped staging layer - same (member, qty, order)
// shape, only the copy and the committed mutations differ
import {
  applyOps,
  CommitError,
  commitOps,
  isTemp,
  newTempId,
  opsToDiff,
  reduceOps,
  type Committers,
  type GroupedMember,
  type MemberProduct,
  type PendingOp,
} from './groupedStaging'
import { useProductSaveSection } from './productSaveRegistry'
import { ProductPickerDialog, type PickExtra } from './ProductPickerDialog'
import { quantityColumn } from './product-picker-columns'

type KitComponentRow = {
  id: string
  component: MemberProduct
  quantity: number
  sortOrder: number
}

type Kit = {
  id: string
  kitItemsReturnable: boolean
  kitComponents: KitComponentRow[]
}

const returnsLabel = (byItem: boolean) => (byItem ? 'Whole kit or single items' : 'Whole kit only')

export function KitStructureSection({ product, canManage }: { product: Kit; canManage: boolean }) {
  const [pendingOps, setPendingOps] = useState<PendingOp[]>([])
  // null = unchanged; staged like the components and applied by the page Save
  const [stagedReturnable, setStagedReturnable] = useState<boolean | null>(null)
  const itemsReturnable = stagedReturnable ?? product.kitItemsReturnable

  const stage = useCallback((op: PendingOp) => {
    setPendingOps((prev) => reduceOps([...prev, op]))
  }, [])

  // Map kit rows into the staging layer's member shape
  const asMembers = useMemo<GroupedMember[]>(
    () =>
      product.kitComponents.map((c) => ({
        id: c.id,
        member: c.component,
        defaultQuantity: c.quantity,
        sortOrder: c.sortOrder,
      })),
    [product.kitComponents],
  )

  const reduced = useMemo(() => reduceOps(pendingOps), [pendingOps])
  const staged = useMemo(() => applyOps(asMembers, reduced), [asMembers, reduced])
  const returnsChange = stagedReturnable !== null && stagedReturnable !== product.kitItemsReturnable
  const diff = useMemo(() => {
    const returns = returnsChange ? changed('Returns', returnsLabel(product.kitItemsReturnable), returnsLabel(itemsReturnable)) : null
    return [...opsToDiff(asMembers, reduced), ...(returns ? [returns] : [])]
  }, [asMembers, reduced, returnsChange, product.kitItemsReturnable, itemsReturnable])

  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [addComponent] = useMutation(AddKitComponentDocument, { refetchQueries: refetch })
  const [updateComponent] = useMutation(UpdateKitComponentDocument, { refetchQueries: refetch })
  const [removeComponent] = useMutation(RemoveKitComponentDocument, { refetchQueries: refetch })
  const [updateProduct] = useMutation(UpdateProductDocument, { refetchQueries: refetch })

  const save = useCallback(async () => {
    if (returnsChange) {
      await updateProduct({ variables: { input: { id: product.id, kitItemsReturnable: stagedReturnable } } })
      setStagedReturnable(null)
    }
    if (reduced.length === 0) return
    const knownIds = new Set(product.kitComponents.map((c) => c.id))
    const committers: Committers = {
      addMember: async (op) => {
        const res = await addComponent({
          variables: {
            input: {
              productId: op.parentId,
              componentProductId: op.product.id,
              quantity: op.defaultQuantity,
              sortOrder: op.sortOrder,
            },
          },
        })
        const rows = res.data?.addKitComponent.kitComponents ?? []
        const fresh = rows.find((c) => !knownIds.has(c.id))
        if (!fresh) throw new Error('Could not resolve newly-added kit component id from response.')
        knownIds.add(fresh.id)
        return fresh.id
      },
      updateMember: async (op) => {
        const input: { id: string; quantity?: number; sortOrder?: number } = { id: op.id }
        if (op.defaultQuantity !== undefined) input.quantity = op.defaultQuantity
        if (op.sortOrder !== undefined) input.sortOrder = op.sortOrder
        await updateComponent({ variables: { input } })
      },
      removeMember: async (op) => {
        await removeComponent({ variables: { id: op.id } })
      },
    }
    try {
      await commitOps(reduced, product.id, committers)
    } catch (e) {
      // keep only the unapplied tail - a retry resumes instead of re-running committed adds and duplicating rows
      if (e instanceof CommitError) setPendingOps(e.remaining as PendingOp[])
      throw e
    }
    setPendingOps([])
  }, [reduced, product.id, product.kitComponents, addComponent, updateComponent, removeComponent, returnsChange, stagedReturnable, updateProduct])

  useProductSaveSection({ id: 'kit-structure', title: 'Kit components', diff, save })

  const [pickerOpen, setPickerOpen] = useState(false)
  const [editing, setEditing] = useState<GroupedMember | null>(null)
  const [confirmingRemove, setConfirmingRemove] = useState<GroupedMember | null>(null)

  const sorted = useMemo(() => [...staged].sort((a, b) => a.sortOrder - b.sortOrder), [staged])

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <div className="grid gap-1">
          <h3 className="text-sm font-medium">Kit components</h3>
          <p className="text-sm text-muted-foreground">
            The fixed set of simple products assembled into this kit. The kit sells as one line at its
            own price; fulfillment decrements each component's inventory. Edits batch into the
            page-level "Save changes".
          </p>
        </div>
        {canManage && (
          <Button size="sm" onClick={() => setPickerOpen(true)}>
            <PlusIcon className="size-4" />
            Add components
          </Button>
        )}
      </div>

      <div className="flex items-start justify-between gap-4 rounded-md border px-4 py-3">
        <div className="grid gap-1">
          <Label htmlFor="kit-items-returnable">Return items separately</Label>
          <p className="text-sm text-muted-foreground">
            Let a customer return one item out of the kit, refunded at its share of the kit price. Off, the kit only
            comes back whole. Each order keeps the setting it was placed under.
          </p>
        </div>
        <Switch
          id="kit-items-returnable"
          checked={itemsReturnable}
          onCheckedChange={(v) => setStagedReturnable(v === product.kitItemsReturnable ? null : v)}
          disabled={!canManage}
        />
      </div>

      {sorted.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
          No components yet. A kit needs at least one to be sellable.
        </p>
      ) : (
        <Card className="overflow-hidden p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Component</TableHead>
                <TableHead className="w-24">SKU</TableHead>
                <TableHead className="w-24">Qty per kit</TableHead>
                <TableHead className="w-24">Price</TableHead>
                <TableHead className="w-20">Order</TableHead>
                {canManage && <TableHead className="w-20" />}
              </TableRow>
            </TableHeader>
            <TableBody>
              {sorted.map((m) => {
                const pending = isTemp(m.id)
                return (
                  <TableRow key={m.id} className={pending ? 'bg-warning-soft' : ''}>
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2">
                        <PackagePlusIcon className="size-4 text-muted-foreground" />
                        {m.member.name ?? m.member.sku ?? ''}
                        {pending && (
                          <Badge variant="outline" className="font-normal text-warning">
                            Unsaved
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      {m.member.sku ? (
                        <code className="font-mono text-xs">{m.member.sku}</code>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{m.defaultQuantity}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {formatCents(m.member.defaultPrice, m.member.defaultCurrencyCode)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{m.sortOrder}</TableCell>
                    {canManage && (
                      <TableCell>
                        <div className="flex gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7"
                            onClick={() => setEditing(m)}
                            aria-label="Edit component"
                          >
                            <PencilIcon className="size-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive"
                            onClick={() => setConfirmingRemove(m)}
                            aria-label="Remove component"
                          >
                            <Trash2Icon className="size-3.5" />
                          </Button>
                        </div>
                      </TableCell>
                    )}
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      {canManage && (
        <ComponentPickerSheet
          parentId={product.id}
          existing={sorted}
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          onStage={stage}
        />
      )}
      {canManage && editing && (
        <EditComponentDialog
          component={editing}
          open
          onOpenChange={(v) => {
            if (!v) setEditing(null)
          }}
          onStage={stage}
        />
      )}

      <ConfirmDialog
        open={!!confirmingRemove}
        onOpenChange={(v) => {
          if (!v) setConfirmingRemove(null)
        }}
        title={<>Remove {confirmingRemove?.member.name ?? confirmingRemove?.member.sku ?? ''} from this kit?</>}
        description="Stages the component for removal. The product itself isn't deleted - only its place in this kit goes when you confirm the page-level save."
        confirmLabel="Stage remove"
        destructive
        onConfirm={() => {
          if (confirmingRemove) stage({ kind: 'remove-member', id: confirmingRemove.id })
        }}
      />
    </div>
  )
}

function EditComponentDialog({
  component,
  open,
  onOpenChange,
  onStage,
}: {
  component: GroupedMember
  open: boolean
  onOpenChange: (v: boolean) => void
  onStage: (op: PendingOp) => void
}) {
  const [quantity, setQuantity] = useState(component.defaultQuantity)
  const [sortOrder, setSortOrder] = useState(component.sortOrder)

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const op = {
      kind: 'update-member' as const,
      id: component.id,
      ...(quantity !== component.defaultQuantity ? { defaultQuantity: quantity } : {}),
      ...(sortOrder !== component.sortOrder ? { sortOrder } : {}),
    }
    if (Object.keys(op).length > 2) onStage(op)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit component</DialogTitle>
          <DialogDescription>{component.member.name ?? component.member.sku ?? ''}</DialogDescription>
        </DialogHeader>

        <form id="edit-kit-component" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-2 gap-3 items-start">
            <div className="grid gap-2">
              <Label htmlFor="kc-qty-edit">Quantity per kit</Label>
              <Input
                id="kc-qty-edit"
                type="number"
                min={0.0001}
                step="any"
                value={quantity}
                onChange={(e) => setQuantity(Math.max(0.0001, Number(e.target.value) || 1))}
              />
              <p className="text-xs text-muted-foreground">How many units one kit consumes. Decimals allowed.</p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="kc-sort-edit">Sort order</Label>
              <Input
                id="kc-sort-edit"
                type="number"
                inputMode="numeric"
                value={sortOrder}
                onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
              />
            </div>
          </div>
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="submit" form="edit-kit-component">
            Stage edit
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ComponentPickerSheet({
  parentId,
  existing,
  open,
  onOpenChange,
  onStage,
}: {
  parentId: string
  existing: GroupedMember[]
  open: boolean
  onOpenChange: (v: boolean) => void
  onStage: (op: PendingOp) => void
}) {
  const takenIds = useMemo(() => new Set(existing.map((m) => m.member.id)), [existing])
  const selfId = useMemo(() => new Set([parentId]), [parentId])
  const extra = useMemo<PickExtra<{ quantity: number }>>(
    () => ({ initial: () => ({ quantity: 1 }), columns: [quantityColumn('Units per kit')] }),
    [],
  )

  return (
    <ProductPickerDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Add components to this kit"
      description="Pick one or more simple products and set how many units one kit consumes. Stages until the page-level save."
      kinds={['SIMPLE']}
      takenIds={takenIds}
      takenLabel="In this kit"
      excludeIds={selfId}
      showPrice
      extra={extra}
      confirmLabel={(n) => `Stage ${n || ''} component${n === 1 ? '' : 's'}`.replace('  ', ' ')}
      onConfirm={(picks) => {
        let nextSort = existing.length
        for (const { product, extra: params } of picks) {
          onStage({
            kind: 'add-member',
            tempId: newTempId(),
            product: {
              id: product.id,
              sku: product.sku,
              name: product.name,
              kind: product.kind,
              defaultPrice: product.defaultPrice,
              defaultCurrencyCode: product.defaultCurrencyCode,
            },
            defaultQuantity: params.quantity,
            sortOrder: nextSort,
          })
          nextSort++
        }
      }}
    />
  )
}
