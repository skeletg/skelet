import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  DndContext,
  DragOverlay,
  useDraggable,
  useDroppable,
  closestCenter,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import {
  FolderIcon,
  FolderPlusIcon,
  GripVerticalIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
  TruckIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { useStore } from '@tanstack/react-form'
import {
  DELIVERY_KINDS,
  PRODUCT_TYPE_LABELS,
  deliveryKindsUpdate,
  productTypeDiff,
  productTypeSchema,
  toProductTypeUpdateInput,
  toProductTypeValues,
  type ExistingProductType,
} from './schemas/productType'
import {
  SaveRegistryContext,
  useSaveRegistry,
} from '@/shared/saving/saveRegistry'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AttributeInputType } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import { PluginSections } from '@/plugins/detail-sections'
import {
  AssignAttributeDocument,
  AttributesDocument,
  CreateAttributeGroupDocument,
  DeleteAttributeGroupDocument,
  DeleteProductTypeDocument,
  MoveAttributeToGroupDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  UnassignAttributeDocument,
  UpdateAttributeGroupDocument,
  SetProductTypeDeliveryKindsDocument,
  SetProductTypeUrlPathTemplateDocument,
  UpdateProductTypeDocument,
} from './Catalog.gql'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import { ProductTypeReviewsCard } from './ProductTypeReviewsCard'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type AttributeRow = {
  sortOrder: number
  groupId?: string | null
  attribute: { id: string; slug: string; name: string; inputType: AttributeInputType }
}

type GroupData = { id: string; name: string; sortOrder: number; attributes: AttributeRow[] }

// ProductTypeDetail renders one product type's full edit surface
// (identity header, attributes, groups, reviews, etc) - Accepts an
// optional productTypeId prop so ProductTypesList can hand over the
// selected row's id without round-tripping through the URL; when not
// set it falls back to the route param so the page still works
// standalone
export function ProductTypeDetail({ productTypeId }: { productTypeId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = productTypeId ?? params.id ?? ''
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('catalog.manage')

  const ptQuery = useQuery(ProductTypeDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const allAttributesQuery = useQuery(AttributesDocument)

  const productType = ptQuery.data?.productType

  const refetch = [
    { query: ProductTypeDocument, variables: { id } },
    { query: ProductTypesDocument },
  ]
  const [assignAttribute, { loading: assigning }] = useMutation(AssignAttributeDocument, { refetchQueries: refetch })
  const [unassignAttribute, { loading: unassigning }] = useMutation(UnassignAttributeDocument, { refetchQueries: refetch })
  const [deleteProductType, { loading: deleting }] = useMutation(DeleteProductTypeDocument, {
    refetchQueries: [ProductTypesDocument],
  })
  const [createAttributeGroup, { loading: creatingGroup }] = useMutation(CreateAttributeGroupDocument, { refetchQueries: refetch })
  const [updateAttributeGroup, { loading: updatingGroup }] = useMutation(UpdateAttributeGroupDocument, { refetchQueries: refetch })
  const [deleteAttributeGroup] = useMutation(DeleteAttributeGroupDocument, { refetchQueries: refetch })
  const [moveAttributeToGroup] = useMutation(MoveAttributeToGroupDocument, { refetchQueries: refetch })

  const [confirmingDelete, setConfirmingDelete] = useState(false)

  // Page save registry: field edits (the General section) stage locally
  // and the header Save applies them through the SaveConfirmDialog diff
  // modal - the dashboard-wide convention (see docs/conventions.md) - Attribute
  // assignment / group CRUD stays immediate: those restructure entities,
  // they don't edit fields
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const [assignOpen, setAssignOpen] = useState(false)
  const [pickerAttributeId, setPickerAttributeId] = useState('')
  const [pickerSortOrder, setPickerSortOrder] = useState(0)
  const [pickerGroupId, setPickerGroupId] = useState('')
  const [assignError, setAssignError] = useState<string | null>(null)

  const [actionError, setActionError] = useState<string | null>(null)

  const [createGroupOpen, setCreateGroupOpen] = useState(false)
  const [newGroupName, setNewGroupName] = useState('')
  const [newGroupSortOrder, setNewGroupSortOrder] = useState(0)
  const [createGroupError, setCreateGroupError] = useState<string | null>(null)

  const [editingGroup, setEditingGroup] = useState<{ id: string; name: string; sortOrder: number } | null>(null)
  const [editGroupName, setEditGroupName] = useState('')
  const [editGroupSortOrder, setEditGroupSortOrder] = useState(0)
  const [editGroupError, setEditGroupError] = useState<string | null>(null)

  // DnD state
  const [activeDragId, setActiveDragId] = useState<string | null>(null)

  const assignedIds = useMemo(
    () => new Set((productType?.attributes ?? []).map((a) => a.attribute.id)),
    [productType],
  )
  const assignableAttributes = useMemo(() => {
    const all = allAttributesQuery.data?.attributes ?? []
    return all.filter((a) => !assignedIds.has(a.id))
  }, [allAttributesQuery.data, assignedIds])

  const groups: GroupData[] = useMemo(
    () => [...(productType?.groups ?? [])].sort((a, b) => a.sortOrder - b.sortOrder),
    [productType],
  )
  const ungroupedAttrs = useMemo(
    () => (productType?.attributes ?? []).filter((a) => !a.groupId),
    [productType],
  )

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  async function onDelete() {
    await deleteProductType({ variables: { id } })
    navigate('/catalog/product-types', { replace: true })
  }

  async function onAssign() {
    if (!productType || !pickerAttributeId) return
    setAssignError(null)
    try {
      await assignAttribute({
        variables: {
          productTypeId: productType.id,
          attributeId: pickerAttributeId,
          sortOrder: pickerSortOrder,
          groupId: pickerGroupId || undefined,
        },
      })
      setAssignOpen(false)
      setPickerAttributeId('')
      setPickerSortOrder(0)
      setPickerGroupId('')
    } catch (e) {
      setAssignError(e instanceof Error ? e.message : 'Assign failed.')
    }
  }

  async function onUnassign(attributeId: string) {
    if (!productType) return
    setActionError(null)
    try {
      await unassignAttribute({ variables: { productTypeId: productType.id, attributeId } })
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Unassign failed.')
    }
  }

  async function onCreateGroup(e: FormEvent) {
    e.preventDefault()
    if (!productType || !newGroupName.trim()) return
    setCreateGroupError(null)
    try {
      await createAttributeGroup({
        variables: { productTypeId: productType.id, name: newGroupName.trim(), sortOrder: newGroupSortOrder },
      })
      setCreateGroupOpen(false)
      setNewGroupName('')
      setNewGroupSortOrder(0)
    } catch (err) {
      setCreateGroupError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  async function onUpdateGroup(e: FormEvent) {
    e.preventDefault()
    if (!editingGroup || !editGroupName.trim()) return
    setEditGroupError(null)
    try {
      await updateAttributeGroup({
        variables: { id: editingGroup.id, name: editGroupName.trim(), sortOrder: editGroupSortOrder },
      })
      setEditingGroup(null)
    } catch (err) {
      setEditGroupError(err instanceof Error ? err.message : 'Update failed.')
    }
  }

  async function onDeleteGroup(groupId: string) {
    setActionError(null)
    try {
      await deleteAttributeGroup({ variables: { id: groupId } })
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Delete group failed.')
    }
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveDragId(event.active.id as string)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveDragId(null)
    if (!productType || !event.over) return

    const attributeId = event.active.id as string
    const targetGroupId = event.over.id as string

    const currentAttr = productType.attributes.find((a) => a.attribute.id === attributeId)
    const currentGroupId = currentAttr?.groupId ?? null
    const targetId = targetGroupId === 'ungrouped' ? null : targetGroupId

    if (currentGroupId === targetId) return

    moveAttributeToGroup({
      variables: { productTypeId: productType.id, attributeId, groupId: targetId },
    }).catch((e) => setActionError(e instanceof Error ? e.message : 'Move failed.'))
  }

  // These sit in the page wrapper's own padding, so no px-6 of their own
  if (ptQuery.loading) {
    return <p className="text-sm text-muted-foreground">Loading product type…</p>
  }
  if (ptQuery.error) {
    return (
      <LoadError
        title="Couldn't load product type"
        message={ptQuery.error.message}
        onRetry={() => void ptQuery.refetch()}
        retrying={ptQuery.loading}
      />
    )
  }
  if (!productType) {
    return <p className="text-sm text-muted-foreground">Product type not found.</p>
  }

  const activeDragName = activeDragId
    ? productType.attributes.find((a) => a.attribute.id === activeDragId)?.attribute.name
    : undefined

  const hasGroups = groups.length > 0

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/catalog/product-types"
        backLabel="Back to product types"
        title={productType.name}
        badges={
          <>
            {productType.isDigital && (
              <Badge variant="secondary" className="font-normal">Digital</Badge>
            )}
            {productType.isShippingRequired && (
              <Badge variant="secondary" className="gap-1 font-normal">
                <TruckIcon className="size-3" />
                Ships
              </Badge>
            )}
          </>
        }
        meta={
          <>
            <code className="font-mono">{productType.slug}</code>
            <span>·</span>
            <span>
              {productType.attributes.length} attribute
              {productType.attributes.length === 1 ? '' : 's'}
            </span>
            {hasGroups && (
              <>
                <span>·</span>
                <span>{groups.length} group{groups.length === 1 ? '' : 's'}</span>
              </>
            )}
          </>
        }
        actions={
          canManage ? (
            <Button
              type="button"
              size="sm"
              disabled={!isDirty || savingAll}
              onClick={() => { setSaveError(null); setConfirmingSave(true) }}
            >
              {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
            </Button>
          ) : null
        }
      />

      <DetailPageBody>
      {actionError && (
        <div className="px-6 py-4">
          <Alert variant="destructive">
            <AlertTitle>Action failed</AlertTitle>
            <AlertDescription>{actionError}</AlertDescription>
          </Alert>
        </div>
      )}

      <GeneralProductTypeSection
        key={`${productType.id}:${productType.slug}`}
        productType={productType}
        canManage={canManage}
      />

      <DetailPageSection
        title="Attributes"
        description={
          'Every product of this type inherits these attributes.' +
          (hasGroups && canManage ? ' Drag the grip handle to move an attribute between groups.' : '')
        }
        trailing={
          canManage && (
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setNewGroupName('')
                  setNewGroupSortOrder(groups.length)
                  setCreateGroupError(null)
                  setCreateGroupOpen(true)
                }}
              >
                <FolderPlusIcon className="size-4" />
                Add group
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setAssignError(null)
                  setPickerAttributeId('')
                  setPickerSortOrder(0)
                  setPickerGroupId('')
                  setAssignOpen(true)
                }}
                disabled={assignableAttributes.length === 0}
              >
                <PlusIcon className="size-4" />
                Assign attribute
              </Button>
            </div>
          )
        }
      >
          {!hasGroups && productType.attributes.length === 0 ? (
            <p className="text-sm text-muted-foreground">No attributes assigned yet.</p>
          ) : !hasGroups ? (
            // No groups - flat table, same as before
            <div className="rounded-md border">
              <AttributeTable
                attributes={productType.attributes}
                canManage={canManage}
                draggable={false}
                unassigning={unassigning}
                onUnassign={onUnassign}
              />
            </div>
          ) : (
            // Groups - DnD-enabled sectioned layout
            <DndContext
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragEnd={handleDragEnd}
              onDragCancel={() => setActiveDragId(null)}
            >
              <div className="grid gap-3">
                {groups.map((group) => (
                  <DroppableGroup key={group.id} id={group.id}>
                    {(isOver) => (
                      <div className={`rounded-md border transition-shadow ${isOver ? 'ring-2 ring-primary/50' : ''}`}>
                        <div className="flex items-center justify-between border-b bg-muted/30 px-3 py-2">
                          <div className="flex items-center gap-2">
                            <FolderIcon className="size-4 text-muted-foreground" />
                            <span className="text-sm font-medium">{group.name}</span>
                            {/* Labelled - a bare number here reads as an attribute count, which it isn't */}
                            <Badge variant="outline" className="text-xs font-normal">
                              Order <span className="ml-1 font-mono">{group.sortOrder}</span>
                            </Badge>
                          </div>
                          {canManage && (
                            <div className="flex gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-7"
                                aria-label={`Edit group ${group.name}`}
                                onClick={() => {
                                  setEditingGroup(group)
                                  setEditGroupName(group.name)
                                  setEditGroupSortOrder(group.sortOrder)
                                  setEditGroupError(null)
                                }}
                              >
                                <PencilIcon className="size-3.5" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive"
                                aria-label={`Delete group ${group.name}`}
                                onClick={() => onDeleteGroup(group.id)}
                              >
                                <Trash2Icon className="size-3.5" />
                              </Button>
                            </div>
                          )}
                        </div>
                        {group.attributes.length === 0 ? (
                          <div className={`px-3 py-4 text-center text-sm text-muted-foreground ${isOver ? 'bg-primary/5' : ''}`}>
                            Drop an attribute here
                          </div>
                        ) : (
                          <AttributeTable
                            attributes={group.attributes}
                            canManage={canManage}
                            draggable={canManage}
                            unassigning={unassigning}
                            onUnassign={onUnassign}
                          />
                        )}
                      </div>
                    )}
                  </DroppableGroup>
                ))}

                {/* Ungrouped section (always shown as drop target when groups exist) */}
                <DroppableGroup id="ungrouped">
                  {(isOver) => (
                    <div className={`rounded-md border transition-shadow ${isOver ? 'ring-2 ring-primary/50' : ''}`}>
                      <div className="border-b bg-muted/30 px-3 py-2">
                        <span className="text-sm text-muted-foreground">Ungrouped</span>
                      </div>
                      {ungroupedAttrs.length === 0 ? (
                        <div className={`px-3 py-4 text-center text-sm text-muted-foreground ${isOver ? 'bg-primary/5' : ''}`}>
                          Drop an attribute here to remove it from its group
                        </div>
                      ) : (
                        <AttributeTable
                          attributes={ungroupedAttrs}
                          canManage={canManage}
                          draggable={canManage}
                          unassigning={unassigning}
                          onUnassign={onUnassign}
                        />
                      )}
                    </div>
                  )}
                </DroppableGroup>
              </div>

              <DragOverlay dropAnimation={null}>
                {activeDragName ? (
                  <div className="flex items-center gap-2 rounded-md border bg-card px-3 py-2 shadow-lg">
                    <GripVerticalIcon className="size-4 text-muted-foreground" />
                    <span className="text-sm font-medium">{activeDragName}</span>
                  </div>
                ) : null}
              </DragOverlay>
            </DndContext>
          )}
      </DetailPageSection>

      <ProductTypeReviewsCard
        productTypeId={productType.id}
        canManage={canManage}
      />

      {canManage && (
        <DetailPageSection
          title="Danger zone"
          description="Deleting removes the product type and its attribute assignments. Types referenced by products are rejected by the backend."
        >
          <div className="flex justify-end">
            <Button
              variant="outline"
              size="sm"
              className="text-destructive hover:bg-destructive/10 hover:text-destructive"
              onClick={() => setConfirmingDelete(true)}
              disabled={deleting}
            >
              <Trash2Icon className="size-4" />
              Delete product type
            </Button>
          </div>
        </DetailPageSection>
      )}
        <PluginSections target="product-type" entity={productType} mode="edit" />
      </DetailPageBody>

      {/* Assign attribute dialog */}
      <Dialog open={assignOpen} onOpenChange={setAssignOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Assign attribute</DialogTitle>
            <DialogDescription>
              Pick an existing attribute to attach to this product type.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="assign-attr">Attribute</Label>
              <Select value={pickerAttributeId} onValueChange={setPickerAttributeId}>
                <SelectTrigger id="assign-attr">
                  <SelectValue placeholder="Select an attribute…" />
                </SelectTrigger>
                <SelectContent>
                  {assignableAttributes.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.name} - {a.inputType}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {assignableAttributes.length === 0 && (
                <p className="text-xs text-muted-foreground">All existing attributes are already assigned.</p>
              )}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="assign-sort">Sort order</Label>
              <Input
                id="assign-sort"
                type="number"
                inputMode="numeric"
                value={pickerSortOrder}
                onChange={(e) => setPickerSortOrder(Number(e.target.value) || 0)}
              />
            </div>
            {hasGroups && (
              <div className="grid gap-2">
                <Label htmlFor="assign-group">Group (optional)</Label>
                <Select value={pickerGroupId || 'none'} onValueChange={(v) => setPickerGroupId(v === 'none' ? '' : v)}>
                  <SelectTrigger id="assign-group">
                    <SelectValue placeholder="No group (ungrouped)" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No group</SelectItem>
                    {groups.map((g) => (
                      <SelectItem key={g.id} value={g.id}>{g.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            {assignError && (
              <Alert variant="destructive">
                <AlertTitle>Assign failed</AlertTitle>
                <AlertDescription>{assignError}</AlertDescription>
              </Alert>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAssignOpen(false)}>Cancel</Button>
            <Button onClick={onAssign} disabled={!pickerAttributeId || assigning}>
              {assigning ? 'Assigning…' : 'Assign'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create group dialog */}
      <Dialog open={createGroupOpen} onOpenChange={setCreateGroupOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Add attribute group</DialogTitle>
            <DialogDescription>
              Groups let you organise attributes into sections within this product type.
            </DialogDescription>
          </DialogHeader>
          <form id="create-group-form" onSubmit={onCreateGroup} className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="new-group-name">Name</Label>
              <Input
                id="new-group-name"
                required
                placeholder="Dimensions"
                value={newGroupName}
                onChange={(e) => setNewGroupName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="new-group-order">Sort order</Label>
              <Input
                id="new-group-order"
                type="number"
                inputMode="numeric"
                value={newGroupSortOrder}
                onChange={(e) => setNewGroupSortOrder(Number(e.target.value) || 0)}
              />
            </div>
            {createGroupError && (
              <Alert variant="destructive">
                <AlertTitle>Create failed</AlertTitle>
                <AlertDescription>{createGroupError}</AlertDescription>
              </Alert>
            )}
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateGroupOpen(false)}>Cancel</Button>
            <Button type="submit" form="create-group-form" disabled={!newGroupName.trim() || creatingGroup}>
              {creatingGroup ? 'Creating…' : 'Create group'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit group dialog */}
      <Dialog open={!!editingGroup} onOpenChange={(v) => { if (!v) setEditingGroup(null) }}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Edit group</DialogTitle>
          </DialogHeader>
          <form id="edit-group-form" onSubmit={onUpdateGroup} className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="edit-group-name">Name</Label>
              <Input
                id="edit-group-name"
                required
                value={editGroupName}
                onChange={(e) => setEditGroupName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="edit-group-order">Sort order</Label>
              <Input
                id="edit-group-order"
                type="number"
                inputMode="numeric"
                value={editGroupSortOrder}
                onChange={(e) => setEditGroupSortOrder(Number(e.target.value) || 0)}
              />
            </div>
            {editGroupError && (
              <Alert variant="destructive">
                <AlertTitle>Update failed</AlertTitle>
                <AlertDescription>{editGroupError}</AlertDescription>
              </Alert>
            )}
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingGroup(null)}>Cancel</Button>
            <Button type="submit" form="edit-group-form" disabled={!editGroupName.trim() || updatingGroup}>
              {updatingGroup ? 'Saving…' : 'Save changes'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete product type confirm */}
      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={<>Delete {productType.name}?</>}
        description="This permanently removes the product type and its attribute assignments. Product types with products attached cannot be deleted - the backend will reject those."
        confirmLabel="Delete product type"
        busyLabel="Deleting…"
        destructive
        onConfirm={onDelete}
      />

      {/* Review-before-save modal: lists every staged change across the
          page's registered sections (General) */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Product type · ${productType.name}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// ---------- DroppableGroup ----------

function DroppableGroup({
  id,
  children,
}: {
  id: string
  children: (isOver: boolean) => ReactNode
}) {
  const { isOver, setNodeRef } = useDroppable({ id })
  return <div ref={setNodeRef}>{children(isOver)}</div>
}

// ---------- AttributeTable ----------

function AttributeTable({
  attributes,
  canManage,
  draggable,
  unassigning,
  onUnassign,
}: {
  attributes: AttributeRow[]
  canManage: boolean
  draggable: boolean
  unassigning: boolean
  onUnassign: (attributeId: string) => void
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          {draggable && <TableHead className="w-8" />}
          <TableHead>Attribute</TableHead>
          <TableHead>Type</TableHead>
          <TableHead className="w-16">Order</TableHead>
          {canManage && <TableHead className="w-10" />}
        </TableRow>
      </TableHeader>
      <TableBody>
        {attributes.map((pa) =>
          draggable ? (
            <DraggableAttributeRow
              key={pa.attribute.id}
              pa={pa}
              canManage={canManage}
              unassigning={unassigning}
              onUnassign={onUnassign}
            />
          ) : (
            <StaticAttributeRow
              key={pa.attribute.id}
              pa={pa}
              canManage={canManage}
              unassigning={unassigning}
              onUnassign={onUnassign}
            />
          )
        )}
      </TableBody>
    </Table>
  )
}

// ---------- Attribute rows ----------

type AttributeRowProps = {
  pa: AttributeRow
  canManage: boolean
  unassigning: boolean
  onUnassign: (id: string) => void
}

function StaticAttributeRow({ pa, canManage, unassigning, onUnassign }: AttributeRowProps) {
  return (
    <TableRow>
      <TableCell>
        <Link to={`/catalog/attributes/${pa.attribute.id}`} className="font-medium hover:underline">
          {pa.attribute.name}
        </Link>
      </TableCell>
      <TableCell><AttributeInputTypeBadge type={pa.attribute.inputType} /></TableCell>
      <TableCell className="font-mono text-xs">{pa.sortOrder}</TableCell>
      {canManage && (
        <TableCell>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => onUnassign(pa.attribute.id)}
            disabled={unassigning}
            aria-label={`Unassign ${pa.attribute.name}`}
          >
            <Trash2Icon className="size-3.5" />
          </Button>
        </TableCell>
      )}
    </TableRow>
  )
}

function DraggableAttributeRow({ pa, canManage, unassigning, onUnassign }: AttributeRowProps) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: pa.attribute.id })

  return (
    <TableRow
      ref={setNodeRef}
      className={isDragging ? 'opacity-40' : ''}
    >
      <TableCell className="w-8 pr-0">
        <button
          className="flex cursor-grab touch-none items-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
          aria-label="Drag to move to another group"
          {...listeners}
          {...attributes}
        >
          <GripVerticalIcon className="size-4" />
        </button>
      </TableCell>
      <TableCell>
        <Link to={`/catalog/attributes/${pa.attribute.id}`} className="font-medium hover:underline">
          {pa.attribute.name}
        </Link>
      </TableCell>
      <TableCell><AttributeInputTypeBadge type={pa.attribute.inputType} /></TableCell>
      <TableCell className="font-mono text-xs">{pa.sortOrder}</TableCell>
      {canManage && (
        <TableCell>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => onUnassign(pa.attribute.id)}
            disabled={unassigning}
            aria-label={`Unassign ${pa.attribute.name}`}
          >
            <Trash2Icon className="size-3.5" />
          </Button>
        </TableCell>
      )}
    </TableRow>
  )
}

// ---------- Inline general settings ----------

// The product type's own fields - a form staged into the page's registry, with its own stored draft.
// The attribute and group rows above and below are explicit actions, so they're not in here
function GeneralProductTypeSection({ productType, canManage }: { productType: ExistingProductType; canManage: boolean }) {
  const refetchQueries = [{ query: ProductTypesDocument }, { query: ProductTypeDocument, variables: { id: productType.id } }]
  const [updateProductType] = useMutation(UpdateProductTypeDocument, { refetchQueries })
  const [setUrlTemplate] = useMutation(SetProductTypeUrlPathTemplateDocument, { refetchQueries })
  const [setDeliveryKinds] = useMutation(SetProductTypeDeliveryKindsDocument, { refetchQueries })

  const baseline = useMemo(() => toProductTypeValues(productType), [productType])
  const { form, draft } = useSectionForm({
    id: 'general',
    title: 'General',
    baseline,
    schema: productTypeSchema,
    diff: productTypeDiff,
    routeKey: `product-type:${productType.id}`,
    label: `Product type · ${productType.name}`,
    submit: async (value) => {
      // A changed template goes through its own mutation, which also recomputes the products' paths
      const { input, newTemplate } = toProductTypeUpdateInput(value, productType)
      await updateProductType({ variables: { input } })
      if (newTemplate !== null) await setUrlTemplate({ variables: { productTypeId: productType.id, template: newTemplate } })
      const kinds = deliveryKindsUpdate(value, productType)
      if (kinds !== undefined) await setDeliveryKinds({ variables: { productTypeId: productType.id, kinds } })
    },
  })
  const isDigital = useStore(form.store, (s) => s.values.isDigital)
  const isShippingRequired = useStore(form.store, (s) => s.values.isShippingRequired)

  return (
    <DetailPageSection title="General" description="Slug is immutable after creation. Other settings can be edited at any time.">
      {draft.offer !== null && (
        <div className="mb-4">
          <RestoredDraftBanner draft={draft} labels={PRODUCT_TYPE_LABELS} />
        </div>
      )}
      <fieldset disabled={!canManage} className="grid gap-4">
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="name">{(f) => <f.TextField id="pt-name" label="Name" placeholder="T-shirt" />}</form.AppField>
          <div className="grid gap-1.5">
            <Label htmlFor="pt-slug">Slug</Label>
            <Input id="pt-slug" value={productType.slug} disabled className="font-mono" />
            <p className="text-xs text-muted-foreground">Slug is immutable.</p>
          </div>
        </div>

        <div className="grid gap-3 rounded-md border p-4">
          <form.AppField
            name="isDigital"
            listeners={{
              // Digital goods never ship
              onChange: ({ value }) => {
                if (value) form.setFieldValue('isShippingRequired', false)
              },
            }}
          >
            {(f) => (
              <f.SwitchField
                id="pt-digital"
                label="Digital"
                description="Digital goods have no physical shipment (e.g. downloads, gift cards)."
                bordered={false}
              />
            )}
          </form.AppField>
          <form.AppField name="isShippingRequired">
            {(f) => (
              <f.SwitchField
                id="pt-shipping"
                label="Requires shipping"
                description="Physical goods that need a fulfillment step."
                bordered={false}
                disabled={!canManage || isDigital}
              />
            )}
          </form.AppField>
        </div>

        {isShippingRequired && !isDigital && (
          <form.Field name="deliveryKinds">
            {(f) => (
              <div className="grid gap-2 rounded-md border p-4">
                <div className="grid gap-0.5">
                  <span className="text-sm font-medium">Allowed delivery kinds</span>
                  <span className="text-xs text-muted-foreground">
                    Limit how products of this type can be delivered - a fridge can&apos;t go to a PO box. Choose all offered kinds, or select the kinds allowed for these products.
                  </span>
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <input type="radio" name="delivery-kind-policy" checked={f.state.value.length === 0} onChange={() => f.handleChange([])} />
                  All offered kinds
                </label>
                <label className="flex items-center gap-2 text-sm">
                  <input type="radio" name="delivery-kind-policy" checked={f.state.value.length > 0} onChange={() => { if (!f.state.value.length) f.handleChange(DELIVERY_KINDS.map((k) => k.value)) }} />
                  Selected kinds
                </label>
                {f.state.value.length > 0 && <p className="text-xs text-muted-foreground">Select at least one kind. Choose All offered kinds to remove the restriction.</p>}
                {f.state.value.length > 0 && DELIVERY_KINDS.map((k) => (
                  <label key={k.value} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={f.state.value.includes(k.value)}
                      disabled={f.state.value.includes(k.value) && f.state.value.length === 1}
                      onCheckedChange={(checked) =>
                        f.handleChange(checked ? [...f.state.value, k.value] : f.state.value.filter((v) => v !== k.value))
                      }
                    />
                    {k.label}
                  </label>
                ))}
              </div>
            )}
          </form.Field>
        )}

        <form.AppField name="urlTemplate">
          {(f) => (
            <f.TextField
              id="pt-url-template"
              label="URL path template"
              placeholder="{category}/{attr:brand}/{slug}"
              hint={
                <>
                  Empty disables auto-generation. Tokens: <code>{'{slug}'}</code> (required), <code>{'{category}'}</code>,{' '}
                  <code>{'{categorySlug}'}</code>, <code>{'{attr:<slug>}'}</code>. Multiselect attributes aren&apos;t allowed.
                </>
              }
            />
          )}
        </form.AppField>
      </fieldset>
    </DetailPageSection>
  )
}
