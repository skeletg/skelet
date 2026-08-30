import { useCallback, useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  PackagePlusIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  Trash2Icon,
} from 'lucide-react'
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
  AddGroupedMemberDocument,
  ProductDocument,
  ProductsDocument,
  RemoveGroupedMemberDocument,
  UpdateGroupedMemberDocument,
} from './Catalog.gql'
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

type Grouped = {
  id: string
  groupedMembers: GroupedMember[]
}

export function GroupedStructureSection({
  product,
  canManage,
}: {
  product: Grouped
  canManage: boolean
}) {
  const [pendingOps, setPendingOps] = useState<PendingOp[]>([])

  const stage = useCallback((op: PendingOp) => {
    setPendingOps((prev) => reduceOps([...prev, op]))
  }, [])

  const reduced = useMemo(() => reduceOps(pendingOps), [pendingOps])
  const staged = useMemo(() => applyOps(product.groupedMembers, reduced), [product.groupedMembers, reduced])
  const diff = useMemo(() => opsToDiff(product.groupedMembers, reduced), [product.groupedMembers, reduced])

  const refetch = [{ query: ProductDocument, variables: { id: product.id } }]
  const [addMemberMutation] = useMutation(AddGroupedMemberDocument, { refetchQueries: refetch })
  const [updateMemberMutation] = useMutation(UpdateGroupedMemberDocument, { refetchQueries: refetch })
  const [removeMemberMutation] = useMutation(RemoveGroupedMemberDocument, { refetchQueries: refetch })

  const save = useCallback(async () => {
    if (reduced.length === 0) return
    const knownMemberIds = new Set(product.groupedMembers.map((m) => m.id))
    const committers: Committers = {
      addMember: async (op) => {
        const res = await addMemberMutation({
          variables: {
            input: {
              parentId: op.parentId,
              memberId: op.product.id,
              defaultQuantity: op.defaultQuantity,
              sortOrder: op.sortOrder,
            },
          },
        })
        const members = res.data?.addGroupedMember.groupedMembers ?? []
        const fresh = members.find((m) => !knownMemberIds.has(m.id))
        if (!fresh) {
          throw new Error('Could not resolve newly-added grouped member id from response.')
        }
        knownMemberIds.add(fresh.id)
        return fresh.id
      },
      updateMember: async (op) => {
        const variables: { input: { id: string; defaultQuantity?: number; sortOrder?: number } } = {
          input: { id: op.id },
        }
        if (op.defaultQuantity !== undefined) variables.input.defaultQuantity = op.defaultQuantity
        if (op.sortOrder !== undefined) variables.input.sortOrder = op.sortOrder
        await updateMemberMutation({ variables })
      },
      removeMember: async (op) => {
        await removeMemberMutation({ variables: { id: op.id } })
      },
    }
    await commitOps(reduced, product.id, committers)
    setPendingOps([])
  }, [
    reduced,
    product.id,
    product.groupedMembers,
    addMemberMutation,
    updateMemberMutation,
    removeMemberMutation,
  ])

  useProductSaveSection({ id: 'grouped-structure', title: 'Grouped members', diff, save })

  const [pickerOpen, setPickerOpen] = useState(false)
  const [editingMember, setEditingMember] = useState<GroupedMember | null>(null)
  const [confirmingRemove, setConfirmingRemove] = useState<GroupedMember | null>(null)

  const sortedMembers = useMemo(
    () => [...staged].sort((a, b) => a.sortOrder - b.sortOrder),
    [staged],
  )

  function onRemoveMember(member: GroupedMember) {
    stage({ kind: 'remove-member', id: member.id })
    setConfirmingRemove(null)
  }

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <div className="grid gap-1">
          <h3 className="text-sm font-medium">Grouped members</h3>
          <p className="text-sm text-muted-foreground">
            Pick the simple products this group displays together. Each member is added to the cart
            individually - the grouped parent itself isn't purchasable. Edits batch into the page-level
            "Save changes".
          </p>
        </div>
        {canManage && (
          <Button size="sm" onClick={() => setPickerOpen(true)}>
            <PlusIcon className="size-4" />
            Add members
          </Button>
        )}
      </div>

      {sortedMembers.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
          No members yet. Add one to start filling out this group.
        </p>
      ) : (
        <Card className="overflow-hidden p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Product</TableHead>
                <TableHead className="w-24">SKU</TableHead>
                <TableHead className="w-24">Default qty</TableHead>
                <TableHead className="w-24">Price</TableHead>
                <TableHead className="w-20">Order</TableHead>
                {canManage && <TableHead className="w-20" />}
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedMembers.map((m) => {
                const pending = isTemp(m.id)
                return (
                  <TableRow key={m.id} className={pending ? 'bg-amber-50/30 dark:bg-amber-950/10' : ''}>
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2">
                        <PackagePlusIcon className="size-4 text-muted-foreground" />
                        {m.member.name ?? m.member.sku ?? ''}
                        {pending && (
                          <Badge variant="outline" className="font-normal text-amber-700 dark:text-amber-400">
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
                            onClick={() => setEditingMember(m)}
                            aria-label="Edit member"
                          >
                            <PencilIcon className="size-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive"
                            onClick={() => setConfirmingRemove(m)}
                            aria-label="Remove member"
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
        <MemberPickerSheet
          parentId={product.id}
          existing={sortedMembers}
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          onStage={stage}
        />
      )}
      {canManage && editingMember && (
        <EditMemberDialog
          member={editingMember}
          open
          onOpenChange={(v) => {
            if (!v) setEditingMember(null)
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
              Remove {confirmingRemove?.member.name ?? confirmingRemove?.member.sku ?? ''} from this group?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Stages the member for removal. The product itself isn't deleted - only the link to this
              group is removed when you confirm the page-level save.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (confirmingRemove) onRemoveMember(confirmingRemove)
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

// ──────────────────────── Edit member ────────────────────────

function EditMemberDialog({
  member,
  open,
  onOpenChange,
  onStage,
}: {
  member: GroupedMember
  open: boolean
  onOpenChange: (v: boolean) => void
  onStage: (op: PendingOp) => void
}) {
  const [defaultQuantity, setDefaultQuantity] = useState(member.defaultQuantity)
  const [sortOrder, setSortOrder] = useState(member.sortOrder)

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const op = {
      kind: 'update-member' as const,
      id: member.id,
      ...(defaultQuantity !== member.defaultQuantity ? { defaultQuantity } : {}),
      ...(sortOrder !== member.sortOrder ? { sortOrder } : {}),
    }
    if (Object.keys(op).length > 2) onStage(op)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit member</DialogTitle>
          <DialogDescription>{member.member.name ?? member.member.sku ?? ''}</DialogDescription>
        </DialogHeader>

        <form id="edit-grouped-member" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="gm-qty-edit">Default quantity</Label>
              <Input
                id="gm-qty-edit"
                type="number"
                min={1}
                value={defaultQuantity}
                onChange={(e) => setDefaultQuantity(Math.max(1, Number(e.target.value) || 1))}
              />
              <p className="text-xs text-muted-foreground">Pre-fills the qty input on the channel.</p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="gm-sort-edit">Sort order</Label>
              <Input
                id="gm-sort-edit"
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
          <Button type="submit" form="edit-grouped-member">
            Stage edit
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ──────────────────────── Member picker ────────────────────────

function MemberPickerSheet({
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
  const { data, loading } = useQuery(ProductsDocument)

  const [search, setSearch] = useState('')
  const [picked, setPicked] = useState<Map<string, { quantity: number }>>(new Map())

  const linkedIds = useMemo(() => new Set(existing.map((m) => m.member.id)), [existing])

  const candidates = useMemo(() => {
    const all = (data?.products ?? []) as Array<MemberProduct>
    return all
      .filter((p) => p.kind === 'SIMPLE' && p.id !== parentId && !linkedIds.has(p.id))
      .sort((a, b) => (a.name ?? a.sku ?? '').localeCompare(b.name ?? b.sku ?? ''))
  }, [data, parentId, linkedIds])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return candidates
    return candidates.filter((p) => {
      const haystack = [p.name ?? '', p.sku ?? ''].join(' ').toLowerCase()
      return haystack.includes(q)
    })
  }, [candidates, search])

  function togglePicked(id: string) {
    setPicked((prev) => {
      const next = new Map(prev)
      if (next.has(id)) next.delete(id)
      else next.set(id, { quantity: 1 })
      return next
    })
  }

  function patchRow(id: string, patch: Partial<{ quantity: number }>) {
    setPicked((prev) => {
      const next = new Map(prev)
      const current = next.get(id)
      if (!current) return prev
      next.set(id, { ...current, ...patch })
      return next
    })
  }

  function onSubmit() {
    const entries = Array.from(picked.entries())
    if (entries.length === 0) return

    let nextSort = existing.length
    for (const [pid, params] of entries) {
      const product = candidates.find((c) => c.id === pid)
      if (!product) continue
      onStage({
        kind: 'add-member',
        tempId: newTempId(),
        product,
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
      <SheetContent
        className="flex w-[96vw] flex-col gap-0 p-0 sm:!max-w-[min(96vw,1200px)]"
        side="right"
      >
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>Add members to this group</SheetTitle>
          <SheetDescription>
            Pick one or more simple products. Each row sets the default quantity shown on the channel.
            Stages until the page-level save.
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
            <span>{loading ? 'Loading…' : `Showing ${filtered.length} of ${candidates.length}`}</span>
            <span>{picked.size > 0 ? `${picked.size} selected` : ''}</span>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {filtered.length === 0 && !loading ? (
            <p className="p-6 text-sm text-muted-foreground">
              {candidates.length === 0
                ? 'No eligible simple products - create some first, or remove existing members.'
                : 'No candidates match the current search.'}
            </p>
          ) : (
            <Table>
              <TableHeader className="sticky top-0 z-10 bg-popover">
                <TableRow>
                  <TableHead className="w-10" />
                  <TableHead>Name</TableHead>
                  <TableHead className="w-24">SKU</TableHead>
                  <TableHead className="w-24">Price</TableHead>
                  <TableHead className="w-24">Default qty</TableHead>
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
                          onCheckedChange={() => togglePicked(p.id)}
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
                          min={1}
                          className="h-7 w-20 font-mono text-xs"
                          value={params?.quantity ?? 1}
                          disabled={!isPicked}
                          onChange={(e) => patchRow(p.id, { quantity: Math.max(1, Number(e.target.value) || 1) })}
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
              {`Stage ${picked.size || ''} member${picked.size === 1 ? '' : 's'}`.trim()}
            </Button>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
