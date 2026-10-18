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
  CheckIcon,
  CheckSquare2Icon,
  ChevronDownIcon,
  GripVerticalIcon,
  ImageIcon,
  LayoutGridIcon,
  MoreHorizontalIcon,
  PlusIcon,
  Share2Icon,
  Square,
  StarIcon,
  Trash2Icon,
  XIcon,
  type LucideIcon,
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
import { readImageDimensions } from '@/lib/image'
import { effectiveMediaRoles } from './media-utils'

const MEDIA_ROLE_OPTIONS: Array<{ role: ProductMediaRole; label: string; Icon: LucideIcon; fallbackHint: string }> = [
  { role: 'MAIN', label: 'Main', Icon: StarIcon, fallbackHint: 'Uses the first image until one is chosen' },
  { role: 'THUMBNAIL', label: 'Thumbnail', Icon: LayoutGridIcon, fallbackHint: 'Uses the main image until one is chosen' },
  { role: 'OG_IMAGE', label: 'OG image', Icon: Share2Icon, fallbackHint: 'Uses the first image until one is chosen' },
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
  const effectiveRoles = useMemo(() => effectiveMediaRoles(sortedMedia, draftRoleIds), [sortedMedia, draftRoleIds])
  const visibleMedia = showAllMedia ? sortedMedia : sortedMedia.slice(0, 3)
  const hiddenMediaCount = Math.max(0, sortedMedia.length - visibleMedia.length)
  const mediaById = useMemo(() => new Map(sortedMedia.map((m) => [m.id, m])), [sortedMedia])

  // Header slot click - expand the gallery first when the card sits past the first three, then bring it into view
  function focusMedia(id: string) {
    if (!visibleMedia.some((m) => m.id === id)) setShowAllMedia(true)
    requestAnimationFrame(() => {
      document.getElementById(`product-media-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
  }
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
            processing finishes. Unassigned roles fall back to the first image.
          </p>
          <div className="flex flex-wrap items-center gap-2 pt-1">
            <Badge variant="secondary" className="font-normal">{sortedMedia.length} image{sortedMedia.length === 1 ? '' : 's'}</Badge>
            {MEDIA_ROLE_OPTIONS.map(({ role, label, Icon, fallbackHint }) => {
              const { id, auto } = effectiveRoles[role]
              const item = id ? mediaById.get(id) : undefined
              const dirty = draftRoleIds[role] !== originalRoleIds[role]
              const state = !id ? 'not set' : auto ? 'automatic' : 'set'
              return (
                <button
                  key={role}
                  type="button"
                  disabled={!item}
                  onClick={() => id && focusMedia(id)}
                  title={auto ? fallbackHint : item ? `Show the ${label.toLowerCase()} image` : undefined}
                  className={`flex items-center gap-2 rounded-md border bg-background p-1.5 pr-3 text-left transition-colors enabled:hover:bg-muted/60 ${auto || !item ? 'border-dashed border-foreground/30' : 'border-foreground/40'}`}
                >
                  {item ? (
                    <img src={item.url} alt="" className="size-10 rounded object-cover" />
                  ) : (
                    <span className="flex size-10 items-center justify-center rounded border border-dashed text-muted-foreground">
                      <Icon className="size-4" />
                    </span>
                  )}
                  <span className="grid leading-tight">
                    <span className="flex items-center gap-1 text-xs font-medium">
                      <Icon className="size-3" />
                      {label}
                    </span>
                    <span className="text-[11px] text-muted-foreground">
                      {state}{dirty ? ' · unsaved' : ''}
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
        </div>
        {canManage && (
          <div className="flex flex-wrap items-center justify-end gap-2">
            {!selectMode ? (
              <>
                {/* Native arrow hugs the border, so draw our own and match the button height */}
                <div className="relative">
                  <select
                    value={uploadRole}
                    onChange={(event) => setUploadRole(event.currentTarget.value as ProductUploadRole)}
                    disabled={uploading}
                    className="h-8 appearance-none rounded-lg border border-input bg-background pl-2.5 pr-8 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50"
                    aria-label="Upload destination"
                  >
                    <option value="GALLERY">Gallery</option>
                    {MEDIA_ROLE_OPTIONS.map(({ role, label }) => (
                      <option key={role} value={role}>{label}</option>
                    ))}
                  </select>
                  <ChevronDownIcon className="pointer-events-none absolute right-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                </div>
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
                  id={`product-media-${item.id}`}
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
                    {/* Role pills - solid when assigned, dashed when the image only stands in by fallback. Right-hand side in select mode so the checkbox keeps its corner */}
                    <div className={`absolute top-2 flex flex-col gap-1 ${selectMode ? 'right-2 items-end' : 'left-2 items-start'}`}>
                      {MEDIA_ROLE_OPTIONS.filter(({ role }) => effectiveRoles[role].id === item.id).map(({ role, label, Icon, fallbackHint }) => {
                        if (effectiveRoles[role].auto) {
                          return (
                            <span
                              key={role}
                              title={`Not assigned. ${fallbackHint}.`}
                              className="flex items-center gap-1 rounded-full border border-dashed border-foreground/40 bg-background/90 px-2 py-0.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur-sm"
                            >
                              <Icon className="size-3" />
                              {label} · auto
                            </span>
                          )
                        }
                        const dirty = draftRoleIds[role] !== originalRoleIds[role]
                        return (
                          <span
                            key={role}
                            className="flex items-center gap-1 rounded-full border border-background/80 bg-foreground px-2 py-0.5 text-[11px] font-medium text-background shadow-md ring-1 ring-foreground/40"
                          >
                            <Icon className="size-3" />
                            {label}{dirty ? ' · unsaved' : ''}
                          </span>
                        )
                      })}
                    </div>
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
                    <div className="truncate text-sm font-medium">{item.altText || 'Untitled media'}</div>
                    <div className="flex items-center justify-between gap-2 text-muted-foreground">
                      <span>{item.width}x{item.height}</span>
                      <Badge variant={item.status === 'READY' ? 'secondary' : 'outline'} className="text-[10px]">
                        {item.status}
                      </Badge>
                    </div>
                    {canManage && !selectMode && (
                      <div className="mt-2 grid gap-2">
                        <div className="grid grid-cols-3 gap-1.5">
                          {MEDIA_ROLE_OPTIONS.map(({ role, label, Icon }) => {
                            const assignedHere = draftRoleIds[role] === item.id
                            const autoHere = effectiveRoles[role].auto && effectiveRoles[role].id === item.id
                            const title = item.status !== 'READY'
                              ? 'Only ready media can receive roles'
                              : assignedHere ? 'Click to unset'
                              : autoHere ? 'Used automatically. Click to set explicitly'
                              : `Set as ${label.toLowerCase()}`
                            return (
                              <Button
                                key={role}
                                type="button"
                                size="sm"
                                variant="outline"
                                aria-pressed={assignedHere}
                                className={`h-8 gap-1 px-1 text-[11px] ${
                                  assignedHere
                                    ? 'border-transparent bg-foreground text-background hover:bg-foreground/85 hover:text-background dark:border-transparent dark:bg-foreground dark:hover:bg-foreground/85'
                                    : autoHere ? 'border-dashed border-foreground/40 dark:border-foreground/40' : ''
                                }`}
                                disabled={item.status !== 'READY' || activeRole === role || isDeleting}
                                onClick={() => { if (assignedHere) removeRole(role); else assignRole(role, item.id) }}
                                title={title}
                              >
                                {assignedHere
                                  ? <CheckIcon className="size-3" />
                                  : <Icon className={`size-3 ${autoHere ? '' : 'text-muted-foreground'}`} />}
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
