import { useCallback, useMemo, useState, type FormEvent } from 'react'
import { useMutation } from '@apollo/client/react'
import {
  PackagePlusIcon,
  PencilIcon,
  PlusIcon,
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
import { ProductPickerDialog, type PickExtra, type PickerProduct } from './ProductPickerDialog'
import { quantityColumn } from './product-picker-columns'

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
          <div className="grid grid-cols-2 gap-3 items-start">
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
  const takenIds = useMemo(() => new Set(existing.map((m) => m.member.id)), [existing])
  const selfId = useMemo(() => new Set([parentId]), [parentId])
  const extra = useMemo<PickExtra<{ quantity: number }>>(
    () => ({ initial: () => ({ quantity: 1 }), columns: [quantityColumn('Default qty')] }),
    [],
  )

  return (
    <ProductPickerDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Add members to this group"
      description="Pick one or more simple products. Each row sets the default quantity shown on the channel. Stages until the page-level save."
      kinds={['SIMPLE']}
      takenIds={takenIds}
      takenLabel="In this group"
      excludeIds={selfId}
      showPrice
      extra={extra}
      confirmLabel={(n) => `Stage ${n || ''} member${n === 1 ? '' : 's'}`.replace('  ', ' ')}
      onConfirm={(picks) => {
        let nextSort = existing.length
        for (const { product, extra: params } of picks) {
          onStage({
            kind: 'add-member',
            tempId: newTempId(),
            product: toMemberProduct(product),
            defaultQuantity: params.quantity,
            sortOrder: nextSort,
          })
          nextSort++
        }
      }}
    />
  )
}

function toMemberProduct(p: PickerProduct): MemberProduct {
  return { id: p.id, sku: p.sku, name: p.name, kind: p.kind, defaultPrice: p.defaultPrice, defaultCurrencyCode: p.defaultCurrencyCode }
}
