import { useCallback, useMemo, useRef, useState, type ChangeEvent, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, arrayMove, rectSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  CheckSquare2Icon,
  GripVerticalIcon,
  ImageIcon,
  MoreHorizontalIcon,
  PlusIcon,
  Square,
  Trash2Icon,
  XIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ProductMediaRole } from '@/graphql/schema.types'
import {
  ClearProductMediaRoleDocument,
  ConfirmProductMediaUploadDocument,
  DeleteProductMediaDocument,
  DeleteProductMediaBulkDocument,
  ProductDocument,
  ReorderProductMediaDocument,
  RequestProductMediaUploadDocument,
  SetProductMediaRoleDocument,
  UpdateProductMediaDocument,
} from '../Catalog.gql'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'
import { type ProductMediaSummary } from './shared'
import { readImageDimensions } from './media-utils'

const MEDIA_ROLE_OPTIONS: Array<{ role: ProductMediaRole; label: string }> = [
  { role: 'MAIN', label: 'Main' },
  { role: 'THUMBNAIL', label: 'Thumbnail' },
  { role: 'OG_IMAGE', label: 'OG image' },
]

type ProductUploadRole = ProductMediaRole | 'GALLERY'

type DeleteIntent = { ids: string[]; deleteFromStorage: boolean }

const ROLE_LABELS: Record<ProductMediaRole, string> = {
  MAIN: 'Main image',
  THUMBNAIL: 'Thumbnail',
  OG_IMAGE: 'OG image',
}

export function ProductMediaPanel({
  productId,
  media,
  roles,
  canManage,
  productVariables,
}: {
  productId: string
  media: ProductMediaSummary[]
  roles: Record<ProductMediaRole, ProductMediaSummary | null>
  canManage: boolean
  productVariables: { id: string }
}) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [error, setError] = useState<string | null>(null)
  const [activeRole, setActiveRole] = useState<ProductMediaRole | null>(null)
  const [uploadRole, setUploadRole] = useState<ProductUploadRole>('GALLERY')
  const [uploading, setUploading] = useState(false)
  const [showAllMedia, setShowAllMedia] = useState(false)
  // Select mode
  const [selectMode, setSelectMode] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  // Delete confirmation: holds the pending delete intent (single or bulk)
  const [deleteIntent, setDeleteIntent] = useState<DeleteIntent | null>(null)

  // Draft role assignments - clicks update this map; the global Save flushes the deltas - Uploads + deletes stay
  // immediate (uploads transfer bytes, deletes are explicit destructive actions)
  type DraftRoles = Record<ProductMediaRole, string | null>
  const originalRoleIds = useMemo<DraftRoles>(
    () => ({
      MAIN: roles.MAIN?.id ?? null,
      THUMBNAIL: roles.THUMBNAIL?.id ?? null,
      OG_IMAGE: roles.OG_IMAGE?.id ?? null,
    }),
    [roles],
  )
  const [draftRoleIds, setDraftRoleIds] = useState<DraftRoles>(originalRoleIds)
  const originalRoleKey = `${originalRoleIds.MAIN ?? ''}|${originalRoleIds.THUMBNAIL ?? ''}|${originalRoleIds.OG_IMAGE ?? ''}`
  const [prevRoleKey, setPrevRoleKey] = useState(originalRoleKey)
  if (prevRoleKey !== originalRoleKey) {
    setPrevRoleKey(originalRoleKey)
    setDraftRoleIds(originalRoleIds)
  }

  // Optimistic gallery order - set on drop, cleared when the server order
  // lands via refetch (media prop changes)
  const [orderOverride, setOrderOverride] = useState<string[] | null>(null)
  const [prevMedia, setPrevMedia] = useState(media)
  if (prevMedia !== media) {
    setPrevMedia(media)
    setOrderOverride(null)
  }

  const sortedMedia = useMemo(() => {
    const base = [...media].sort((a, b) => a.sortOrder - b.sortOrder)
    if (!orderOverride) return base
    const pos = new Map(orderOverride.map((id, i) => [id, i]))
    return base.sort((a, b) => (pos.get(a.id) ?? 0) - (pos.get(b.id) ?? 0))
  }, [media, orderOverride])
  const visibleMedia = showAllMedia ? sortedMedia : sortedMedia.slice(0, 3)
  const hiddenMediaCount = Math.max(0, sortedMedia.length - visibleMedia.length)
  const refetchQueries = [{ query: ProductDocument, variables: productVariables }]
  const [requestUpload] = useMutation(RequestProductMediaUploadDocument)
  const [confirmUpload] = useMutation(ConfirmProductMediaUploadDocument, { refetchQueries })
  const [setRole] = useMutation(SetProductMediaRoleDocument, { refetchQueries })
  const [clearRole] = useMutation(ClearProductMediaRoleDocument, { refetchQueries })
  const [deleteMedia, { loading: deletingMedia }] = useMutation(DeleteProductMediaDocument, { refetchQueries })
  const [deleteMediaBulk, { loading: deletingBulk }] = useMutation(DeleteProductMediaBulkDocument, { refetchQueries })
  const [reorderMedia] = useMutation(ReorderProductMediaDocument, { refetchQueries })
  const [updateMedia] = useMutation(UpdateProductMediaDocument, { refetchQueries })
  const isDeleting = deletingMedia || deletingBulk

  const dndSensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const canReorder = canManage && !selectMode && sortedMedia.length > 1

  function onDragEnd(e: DragEndEvent) {
    const { active, over } = e
    if (!over || active.id === over.id) return
    const ids = sortedMedia.map((m) => m.id)
    const next = arrayMove(ids, ids.indexOf(String(active.id)), ids.indexOf(String(over.id)))
    setOrderOverride(next)
    setError(null)
    reorderMedia({ variables: { productId, mediaIds: next } }).catch((err: unknown) => {
      setOrderOverride(null)
      setError(err instanceof Error ? err.message : 'Reorder failed.')
    })
  }

  function onEditAltText(item: ProductMediaSummary) {
    const next = window.prompt('Alt text', item.altText ?? '')
    if (next === null) return
    setError(null)
    updateMedia({ variables: { input: { id: item.id, altText: next } } }).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : 'Alt text update failed.')
    })
  }

  const mediaLabelById = useMemo(() => {
    const m = new Map<string, string>()
    // Fall back to the media's position, never its UUID - the confirm
    // dialog is for humans
    media.forEach((item, i) => m.set(item.id, item.altText || `image #${i + 1}`))
    return m
  }, [media])

  const roleDiff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    for (const role of ['MAIN', 'THUMBNAIL', 'OG_IMAGE'] as ProductMediaRole[]) {
      if (draftRoleIds[role] !== originalRoleIds[role]) {
        const before = originalRoleIds[role] ? mediaLabelById.get(originalRoleIds[role]!) ?? originalRoleIds[role]! : '-'
        const after = draftRoleIds[role] ? mediaLabelById.get(draftRoleIds[role]!) ?? draftRoleIds[role]! : '-'
        entries.push({ label: ROLE_LABELS[role], before, after })
      }
    }
    return entries
  }, [draftRoleIds, originalRoleIds, mediaLabelById])

  const saveRoles = useCallback(async () => {
    for (const role of ['MAIN', 'THUMBNAIL', 'OG_IMAGE'] as ProductMediaRole[]) {
      const draftId = draftRoleIds[role]
      if (draftId === originalRoleIds[role]) continue
      if (draftId) {
        await setRole({ variables: { productId, role, mediaId: draftId } })
      } else {
        await clearRole({ variables: { productId, role } })
      }
    }
  }, [draftRoleIds, originalRoleIds, productId, setRole, clearRole])

  useProductSaveSection({ id: 'media-roles', title: 'Media roles', diff: roleDiff, save: saveRoles })

  function exitSelectMode() {
    setSelectMode(false)
    setSelected(new Set())
  }

  function toggleSelect(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function uploadMedia(file: File) {
    if (!file.type.startsWith('image/')) {
      setError('Only image files can be uploaded here.')
      return
    }
    setError(null)
    setUploading(true)
    const role = uploadRole === 'GALLERY' ? undefined : uploadRole
    if (role) setActiveRole(role)
    try {
      const [{ width, height }, upload] = await Promise.all([
        readImageDimensions(file),
        requestUpload({
          variables: {
            input: { productId, filename: file.name, mimeType: file.type, sizeBytes: file.size, role: undefined },
          },
        }),
      ])
      const payload = upload.data?.requestProductMediaUpload
      if (!payload) throw new Error('Upload request did not return a signed URL.')
      const uploaded = await fetch(payload.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': file.type },
        body: file,
      })
      if (!uploaded.ok) throw new Error(`Image upload failed with HTTP ${uploaded.status}.`)
      await confirmUpload({ variables: { mediaId: payload.media.id, width, height } })
      // If the user picked a target role, queue the assignment in the draft
      // (it'll be applied by the global Save)
      if (role) {
        setDraftRoleIds((prev) => ({ ...prev, [role]: payload.media.id }))
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Image upload failed.')
    } finally {
      setUploading(false)
      setActiveRole(null)
    }
  }

  function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (file) uploadMedia(file)
  }

  function assignRole(role: ProductMediaRole, mediaId: string) {
    setDraftRoleIds((prev) => ({ ...prev, [role]: mediaId }))
  }

  function removeRole(role: ProductMediaRole) {
    setDraftRoleIds((prev) => ({ ...prev, [role]: null }))
  }

  async function confirmDelete() {
    if (!deleteIntent) return
    setError(null)
    try {
      if (deleteIntent.ids.length === 1) {
        await deleteMedia({
          variables: { id: deleteIntent.ids[0], deleteFromStorage: deleteIntent.deleteFromStorage },
        })
      } else {
        await deleteMediaBulk({
          variables: { productId, ids: deleteIntent.ids, deleteFromStorage: deleteIntent.deleteFromStorage },
        })
      }
      setDeleteIntent(null)
      exitSelectMode()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Image removal failed.')
    }
  }

  const selectedCount = selected.size

  return (
    <section className="grid gap-4">
      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3 rounded-md border bg-muted/25 p-4">
        <div className="grid max-w-2xl gap-1">
          <p className="text-sm font-medium">Image library and channel roles</p>
          <p className="text-sm text-muted-foreground">
            Add gallery images, then mark which ready image should represent the product in each
            channel context. Pending images can be reviewed here but cannot receive roles until
            processing finishes.
          </p>
          <div className="flex flex-wrap gap-2 pt-1">
            <Badge variant="secondary" className="font-normal">{sortedMedia.length} image{sortedMedia.length === 1 ? '' : 's'}</Badge>
            {MEDIA_ROLE_OPTIONS.map(({ role, label }) => {
              const draftId = draftRoleIds[role]
              const dirty = draftId !== originalRoleIds[role]
              return (
                <Badge
                  key={role}
                  variant={draftId ? 'outline' : 'secondary'}
                  className={`font-normal ${dirty ? 'ring-1 ring-primary/40' : ''}`}
                >
                  {label}: {draftId ? 'set' : 'not set'}{dirty ? ' (pending)' : ''}
                </Badge>
              )
            })}
          </div>
        </div>
        {canManage && (
          <div className="flex flex-wrap items-center justify-end gap-2">
            {!selectMode ? (
              <>
                <select
                  value={uploadRole}
                  onChange={(event) => setUploadRole(event.currentTarget.value as ProductUploadRole)}
                  disabled={uploading}
                  className="h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm"
                  aria-label="Upload destination"
                >
                  <option value="GALLERY">Gallery</option>
                  {MEDIA_ROLE_OPTIONS.map(({ role, label }) => (
                    <option key={role} value={role}>{label}</option>
                  ))}
                </select>
                <input ref={fileInputRef} type="file" accept="image/*" className="hidden" onChange={onFileChange} />
                <Button type="button" variant="outline" disabled={uploading} onClick={() => fileInputRef.current?.click()}>
                  <PlusIcon className="size-4" />
                  {uploading ? 'Uploading...' : 'Add media'}
                </Button>
                {sortedMedia.length > 0 && (
                  <Button type="button" variant="outline" onClick={() => setSelectMode(true)}>
                    <CheckSquare2Icon className="size-4" />
                    Select
                  </Button>
                )}
              </>
            ) : (
              <>
                <span className="text-sm text-muted-foreground">{selectedCount} selected</span>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="destructive" size="sm" disabled={selectedCount === 0 || isDeleting}>
                      <Trash2Icon className="size-4" />
                      Remove selected
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onSelect={() => setDeleteIntent({ ids: [...selected], deleteFromStorage: false })}
                    >
                      Remove from gallery (keep files)
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      variant="destructive"
                      onSelect={() => setDeleteIntent({ ids: [...selected], deleteFromStorage: true })}
                    >
                      Remove and delete files
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
                <Button type="button" variant="ghost" onClick={exitSelectMode}>
                  <XIcon className="size-4" />
                  Cancel
                </Button>
              </>
            )}
          </div>
        )}
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Media update failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {sortedMedia.length === 0 ? (
        <div className="flex min-h-48 items-center justify-center rounded-md border border-dashed bg-muted/20 p-8 text-center">
          <div className="grid max-w-md justify-items-center gap-2">
            <ImageIcon className="size-10 text-muted-foreground" />
            <p className="text-sm font-medium">No media attached.</p>
            <p className="text-sm text-muted-foreground">
              Upload the product gallery and assign a main, thumbnail, or OG image role from this section.
            </p>
          </div>
        </div>
      ) : (
        <div className="grid gap-3">
          <DndContext sensors={dndSensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
          <SortableContext items={visibleMedia.map((m) => m.id)} strategy={rectSortingStrategy}>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {visibleMedia.map((item) => {
              const isSelected = selected.has(item.id)
              return (
                <SortableMediaCard key={item.id} id={item.id} disabled={!canReorder}>
                <div
                  className={`overflow-hidden rounded-md border bg-card transition-colors ${selectMode && isSelected ? 'ring-2 ring-primary' : ''}`}
                >
                  <div
                    className={`relative aspect-square bg-muted ${selectMode ? 'cursor-pointer' : ''}`}
                    onClick={() => selectMode && toggleSelect(item.id)}
                  >
                    {item.status === 'READY' ? (
                      <img
                        src={item.url}
                        alt={item.altText || 'Product media'}
                        className="size-full object-cover"
                      />
                    ) : (
                      <div className="flex size-full items-center justify-center text-sm text-muted-foreground">
                        Pending
                      </div>
                    )}
                    {selectMode && (
                      <div className="absolute left-2 top-2 flex size-6 items-center justify-center rounded border bg-background/90 shadow-sm">
                        {isSelected
                          ? <CheckSquare2Icon className="size-4 text-primary" />
                          : <Square className="size-4 text-muted-foreground" />
                        }
                      </div>
                    )}
                  </div>
                  <div className="grid gap-2 p-3 text-xs">
                    <div className="flex items-start justify-between gap-2">
                      <div className="truncate text-sm font-medium">{item.altText || 'Untitled media'}</div>
                      <div className="flex shrink-0 flex-wrap justify-end gap-1">
                        {MEDIA_ROLE_OPTIONS.filter(({ role }) => draftRoleIds[role] === item.id).map(({ role, label }) => {
                          const dirty = draftRoleIds[role] !== originalRoleIds[role]
                          return (
                            <Badge key={role} variant="secondary" className={`text-[10px] ${dirty ? 'ring-1 ring-primary/40' : ''}`}>
                              {label}{dirty ? ' (pending)' : ''}
                            </Badge>
                          )
                        })}
                      </div>
                    </div>
                    <div className="flex items-center justify-between gap-2 text-muted-foreground">
                      <span>{item.width}x{item.height}</span>
                      <Badge variant={item.status === 'READY' ? 'secondary' : 'outline'} className="text-[10px]">
                        {item.status}
                      </Badge>
                    </div>
                    {canManage && !selectMode && (
                      <div className="mt-2 grid gap-2">
                        <div className="grid grid-cols-3 gap-1.5">
                          {MEDIA_ROLE_OPTIONS.map(({ role, label }) => {
                            const assignedHere = draftRoleIds[role] === item.id
                            return (
                              <Button
                                key={role}
                                type="button"
                                size="sm"
                                variant={assignedHere ? 'secondary' : 'outline'}
                                className="h-8 px-1 text-[11px]"
                                disabled={item.status !== 'READY' || activeRole === role || isDeleting}
                                onClick={() => { if (assignedHere) removeRole(role); else assignRole(role, item.id) }}
                                title={item.status !== 'READY' ? 'Only ready media can receive roles' : undefined}
                              >
                                {activeRole === role ? '...' : label}
                              </Button>
                            )
                          })}
                        </div>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              type="button"
                              size="sm"
                              variant="ghost"
                              className="h-8 justify-start gap-1 px-1 text-[11px]"
                              disabled={isDeleting}
                            >
                              <MoreHorizontalIcon className="size-3" />
                              More
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="start">
                            <DropdownMenuItem onSelect={() => onEditAltText(item)}>
                              Edit alt text
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              onSelect={() => setDeleteIntent({ ids: [item.id], deleteFromStorage: false })}
                            >
                              Remove from gallery (keep file)
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              onSelect={() => setDeleteIntent({ ids: [item.id], deleteFromStorage: true })}
                            >
                              Remove and delete file
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    )}
                  </div>
                </div>
                </SortableMediaCard>
              )
            })}
          </div>
          </SortableContext>
          </DndContext>

          {sortedMedia.length > 3 && (
            <div className="flex justify-center">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setShowAllMedia((current) => !current)}
              >
                {showAllMedia ? 'Show fewer images' : `Show all images (${hiddenMediaCount} more)`}
              </Button>
            </div>
          )}
        </div>
      )}

      {/* Unified delete confirmation */}
      <AlertDialog open={Boolean(deleteIntent)} onOpenChange={(open) => { if (!open) setDeleteIntent(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {deleteIntent?.deleteFromStorage
                ? `Delete ${deleteIntent.ids.length > 1 ? `${deleteIntent.ids.length} images` : 'image'}?`
                : `Remove ${deleteIntent?.ids.length ?? 1 > 1 ? `${deleteIntent?.ids.length} images` : 'image'} from gallery?`
              }
            </AlertDialogTitle>
            <AlertDialogDescription>
              {deleteIntent?.deleteFromStorage
                ? 'The image file will be permanently deleted from storage. This cannot be undone.'
                : 'The image will be removed from this product. The file stays in storage and can be re-used.'
              }
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={isDeleting}
              onClick={(event) => { event.preventDefault(); confirmDelete() }}
              className={deleteIntent?.deleteFromStorage ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90' : ''}
            >
              {isDeleting
                ? 'Removing…'
                : deleteIntent?.deleteFromStorage
                  ? 'Delete permanently'
                  : 'Remove from gallery'
              }
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

// SortableMediaCard wraps a gallery card with dnd-kit sorting + a grip
// handle so the card's buttons stay clickable (drag starts on the handle only)
function SortableMediaCard({ id, disabled, children }: { id: string; disabled: boolean; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id, disabled })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`relative ${isDragging ? 'z-10 opacity-75' : ''}`}
    >
      {!disabled && (
        <button
          type="button"
          {...attributes}
          {...listeners}
          aria-label="Drag to reorder"
          className="absolute right-2 top-2 z-10 flex size-7 cursor-grab items-center justify-center rounded border bg-background/90 text-muted-foreground shadow-sm hover:text-foreground active:cursor-grabbing"
        >
          <GripVerticalIcon className="size-4" />
        </button>
      )}
      {children}
    </div>
  )
}
