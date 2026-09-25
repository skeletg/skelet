import { useCallback, useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { PackagePlusIcon, PencilIcon, PlusIcon, SearchIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
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
  ProductPickerDocument,
  RemoveKitComponentDocument,
  UpdateKitComponentDocument,
} from './Catalog.gql'
// Kit components share the grouped staging layer - same (member, qty, order)
// shape, only the copy and the committed mutations differ
import {
  applyOps,
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

type KitComponentRow = {
  id: string
  component: MemberProduct
  quantity: number
  sortOrder: number
}

type Kit = {
  id: string
  kitComponents: KitComponentRow[]
}

export function KitStructureSection({ product, canManage }: { product: Kit; canManage: boolean }) {
  const [pendingOps, setPendingOps] = useState<PendingOp[]>([])

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
  const diff = useMemo(() => opsToDiff(asMembers, reduced), [asMembers, reduced])

  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [addComponent] = useMutation(AddKitComponentDocument, { refetchQueries: refetch })
  const [updateComponent] = useMutation(UpdateKitComponentDocument, { refetchQueries: refetch })
  const [removeComponent] = useMutation(RemoveKitComponentDocument, { refetchQueries: refetch })

  const save = useCallback(async () => {
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
    await commitOps(reduced, product.id, committers)
    setPendingOps([])
  }, [reduced, product.id, product.kitComponents, addComponent, updateComponent, removeComponent])

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

      <AlertDialog
        open={!!confirmingRemove}
        onOpenChange={(v) => {
          if (!v) setConfirmingRemove(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Remove {confirmingRemove?.member.name ?? confirmingRemove?.member.sku ?? ''} from this kit?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Stages the component for removal. The product itself isn't deleted - only its place in
              this kit goes when you confirm the page-level save.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (confirmingRemove) {
                  stage({ kind: 'remove-member', id: confirmingRemove.id })
                  setConfirmingRemove(null)
                }
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Stage remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const { data, loading } = useQuery(ProductPickerDocument, {
    variables: { search: debounced || null, kinds: ['SIMPLE'], limit: 25 },
  })
  // The product rides along with the pick - under server search a picked row can scroll out
  // of the current page, and onSubmit must still be able to stage it
  const [picked, setPicked] = useState<Map<string, { quantity: number; product: MemberProduct }>>(new Map())

  const linkedIds = useMemo(() => new Set(existing.map((m) => m.member.id)), [existing])

  // The server filters kind and searches - excluding the parent and already-linked members stays local
  const filtered = useMemo(() => {
    const all = (data?.productsPaged.items ?? []) as Array<MemberProduct>
    return all.filter((p) => p.id !== parentId && !linkedIds.has(p.id))
  }, [data, parentId, linkedIds])

  function togglePicked(product: MemberProduct) {
    setPicked((prev) => {
      const next = new Map(prev)
      if (next.has(product.id)) next.delete(product.id)
      else next.set(product.id, { quantity: 1, product })
      return next
    })
  }

  function onSubmit() {
    const entries = Array.from(picked.entries())
    if (entries.length === 0) return
    let nextSort = existing.length
    for (const [, params] of entries) {
      onStage({
        kind: 'add-member',
        tempId: newTempId(),
        product: params.product,
        defaultQuantity: params.quantity,
        sortOrder: nextSort,
      })
      nextSort++
    }
    setPicked(new Map())
    onOpenChange(false)
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-[96vw] flex-col gap-0 p-0 sm:!max-w-[min(96vw,1200px)]" side="right">
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>Add components to this kit</SheetTitle>
          <SheetDescription>
            Pick one or more simple products and set how many units one kit consumes. Stages until the
            page-level save.
          </SheetDescription>
        </SheetHeader>

        <div className="grid gap-2 border-b px-4 py-3">
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search name, slug, or SKU…"
              className="h-9 pl-8"
            />
          </div>
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>{loading ? 'Loading…' : `Showing ${filtered.length} match${filtered.length === 1 ? '' : 'es'}`}</span>
            <span>{picked.size > 0 ? `${picked.size} selected` : ''}</span>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {filtered.length === 0 && !loading ? (
            <p className="p-6 text-sm text-muted-foreground">
              No eligible simple products match - try another search, or create some first.
            </p>
          ) : (
            <Table>
              <TableHeader className="sticky top-0 z-10 bg-popover">
                <TableRow>
                  <TableHead className="w-10" />
                  <TableHead>Name</TableHead>
                  <TableHead className="w-24">SKU</TableHead>
                  <TableHead className="w-24">Price</TableHead>
                  <TableHead className="w-28">Qty per kit</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((p) => {
                  const isPicked = picked.has(p.id)
                  const params = picked.get(p.id)
                  return (
                    <TableRow key={p.id}>
                      <TableCell>
                        <Checkbox
                          checked={isPicked}
                          onCheckedChange={() => togglePicked(p)}
                          aria-label={`Pick ${p.name ?? p.sku ?? ''}`}
                        />
                      </TableCell>
                      <TableCell>
                        <span className="font-medium">{p.name ?? p.sku ?? ''}</span>
                      </TableCell>
                      <TableCell>
                        {p.sku ? (
                          <code className="font-mono text-xs">{p.sku}</code>
                        ) : (
                          <span className="text-xs text-muted-foreground">-</span>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {formatCents(p.defaultPrice, p.defaultCurrencyCode)}
                      </TableCell>
                      <TableCell>
                        <Input
                          type="number"
                          min={0.0001}
                          step="any"
                          className="h-7 w-24 font-mono text-xs"
                          value={params?.quantity ?? 1}
                          disabled={!isPicked}
                          onChange={(e) =>
                            setPicked((prev) => {
                              const next = new Map(prev)
                              const cur = next.get(p.id)
                              if (!cur) return prev
                              next.set(p.id, { ...cur, quantity: Math.max(0.0001, Number(e.target.value) || 1) })
                              return next
                            })
                          }
                        />
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </div>

        <SheetFooter className="border-t px-4 py-3">
          <div className="flex items-center justify-end gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button onClick={onSubmit} disabled={picked.size === 0}>
              {`Stage ${picked.size || ''} component${picked.size === 1 ? '' : 's'}`.trim()}
            </Button>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
