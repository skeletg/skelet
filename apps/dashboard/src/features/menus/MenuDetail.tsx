import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ChevronDownIcon,
  ChevronUpIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
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
import { Button } from '@/components/ui/button'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'

import type { MenuItemTargetType } from '@/graphql/schema.types'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'

import {
  CreateMenuItemDocument,
  DeleteMenuDocument,
  DeleteMenuItemDocument,
  DeleteMenuItemTranslationDocument,
  MenuDetailDocument,
  MenusListDocument,
  ReorderMenuItemsDocument,
  SetMenuStoresDocument,
  UpdateMenuDocument,
  UpdateMenuItemDocument,
  UpsertMenuItemTranslationDocument,
} from './Menus.gql'
import { PluginSections } from '@/plugins/detail-sections'

type Translation = { locale: string; label: string }

type MenuItem = {
  id: string
  parentId: string | null
  label: string
  targetType: MenuItemTargetType | null
  targetId: string | null
  url: string | null
  sortOrder: number
  translations: Translation[]
}

// Item editor mode: "URL" for free-form links, anything else for typed entity references - The select renders both as one flat dropdown so
// admins don't have to remember the XOR rule - picking a kind from the dropdown auto-clears the other side on save
const TARGET_KINDS: { value: 'URL' | MenuItemTargetType; label: string }[] = [
  { value: 'URL', label: 'External / custom URL' },
  { value: 'PRODUCT', label: 'Product' },
  { value: 'CATEGORY', label: 'Category' },
  { value: 'COLLECTION', label: 'Collection' },
  { value: 'BLOG_POST', label: 'Blog post' },
  { value: 'BLOG_CATEGORY', label: 'Blog category' },
  { value: 'BLOG_TAG', label: 'Blog tag' },
  { value: 'BLOG_AUTHOR', label: 'Blog author' },
  { value: 'CMS_PAGE', label: 'CMS page' },
]

// MenuDetail renders the embedded edit form for a single menu inside
// the master/detail shell provided by MenusList - The optional menuId
// prop lets the parent pass the URL param down; standalone usage falls
// back to useParams so any direct route still works - Layout mirrors
// CollectionDetail / InventoryLocationForm: sticky compact header +
// flat DetailPageSection bodies, no Card wrappers
export function MenuDetail({ menuId }: { menuId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = menuId ?? params.id ?? ''
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.manage')

  const { data, loading, error, refetch } = useQuery(MenuDetailDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })

  const refetchQueries = [{ query: MenuDetailDocument, variables: { id } }]
  const [updateMenu, { loading: savingMenu }] = useMutation(UpdateMenuDocument, {
    // Slug/name edits surface in the sidebar list, so refetch both
    refetchQueries: [
      { query: MenuDetailDocument, variables: { id } },
      { query: MenusListDocument },
    ],
  })
  const [deleteMenu, { loading: deletingMenu }] = useMutation(DeleteMenuDocument, {
    refetchQueries: [{ query: MenusListDocument }],
  })
  const [createMenuItem, { loading: creatingItem }] = useMutation(
    CreateMenuItemDocument,
    { refetchQueries },
  )
  const [updateMenuItem, { loading: updatingItem }] = useMutation(
    UpdateMenuItemDocument,
    { refetchQueries },
  )
  const [deleteMenuItem, { loading: deletingItem }] = useMutation(
    DeleteMenuItemDocument,
    { refetchQueries },
  )
  const [reorderMenuItems] = useMutation(ReorderMenuItemsDocument, {
    refetchQueries,
  })
  const [setMenuStores, { loading: savingStores }] = useMutation(
    SetMenuStoresDocument,
    {
      refetchQueries: [
        { query: MenuDetailDocument, variables: { id } },
        { query: MenusListDocument },
      ],
    },
  )
  const [upsertTranslation] = useMutation(UpsertMenuItemTranslationDocument, {
    refetchQueries,
  })
  const [deleteTranslation] = useMutation(DeleteMenuItemTranslationDocument, {
    refetchQueries,
  })

  // Picker data for the Stores section and the per-locale label editor
  // Each loads once and is shared across all items rendered here
  const storesQuery = useQuery(ChannelsDocument)
  const localesQuery = useQuery(LocalesPickerDocument)
  const allStores = storesQuery.data?.channels ?? []
  const allLocales = localesQuery.data?.locales ?? []

  const menu = data?.menu

  // Editable copies of the metadata so unsaved edits don't trample the displayed state mid-render - Hydrated from
  // `menu` and re-synced whenever it changes (state adjusted during render, not in an effect)
  const [slug, setSlug] = useState(menu?.slug ?? '')
  const [name, setName] = useState(menu?.name ?? '')
  const [storeIds, setStoreIds] = useState<string[]>(menu?.channelIds ?? [])
  const [prevMenu, setPrevMenu] = useState(menu)
  if (prevMenu !== menu) {
    setPrevMenu(menu)
    if (menu) {
      setSlug(menu.slug)
      setName(menu.name)
      setStoreIds(menu.channelIds ?? [])
    }
  }

  // Diffed against the loaded menu, not against what this mounted with - the block above re-seeds
  // these three the moment the query lands, which a mount snapshot would read as an edit
  useUnsavedChangesGuard(
    !!menu &&
      (slug !== menu.slug ||
        name !== menu.name ||
        !sameStringSet(storeIds, menu.channelIds ?? [])),
  )

  const [editing, setEditing] = useState<MenuItem | null>(null)
  const [creating, setCreating] = useState<{ parentId: string | null } | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState<MenuItem | null>(null)
  const [confirmingMenuDelete, setConfirmingMenuDelete] = useState(false)
  const [headerError, setHeaderError] = useState<string | null>(null)
  const [deleteMenuError, setDeleteMenuError] = useState<string | null>(null)

  const items = useMemo(() => (menu?.items ?? []) as MenuItem[], [menu])

  // Flat → tree, sorted by parent then sortOrder - The render walks
  // recursively so deeply-nested items indent automatically
  const tree = useMemo(() => buildTree(items), [items])

  async function onSaveMenu() {
    if (!menu) return
    setHeaderError(null)
    try {
      await updateMenu({
        variables: { input: { id: menu.id, slug, name } },
      })
    } catch (err) {
      setHeaderError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function onMove(item: MenuItem, direction: -1 | 1) {
    if (!menu) return
    const siblings = items
      .filter((i) => (i.parentId ?? null) === (item.parentId ?? null))
      .sort((a, b) => a.sortOrder - b.sortOrder)
    const idx = siblings.findIndex((i) => i.id === item.id)
    const neighborIdx = idx + direction
    if (neighborIdx < 0 || neighborIdx >= siblings.length) return
    const neighbor = siblings[neighborIdx]
    await reorderMenuItems({
      variables: {
        menuId: menu.id,
        items: [
          { id: item.id, parentId: item.parentId, sortOrder: neighbor.sortOrder },
          { id: neighbor.id, parentId: neighbor.parentId, sortOrder: item.sortOrder },
        ],
      },
    })
  }

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteMenuItem({ variables: { id: confirmingDelete.id } })
    setConfirmingDelete(null)
  }

  async function onConfirmMenuDelete() {
    if (!menu) return
    setDeleteMenuError(null)
    try {
      await deleteMenu({ variables: { id: menu.id } })
      navigate('/menus', { replace: true })
    } catch (err) {
      setDeleteMenuError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (loading && !menu) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/menus" backLabel="Menus" title="Menu" />
        <LoadingState label="Loading menu…" />
      </DetailPageLayout>
    )
  }
  if (error) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/menus" backLabel="Menus" title="Menu" />
        <div className="px-6 py-6">
          <LoadError
            title="Couldn't load menu"
            message={error.message}
            onRetry={() => void refetch()}
            retrying={loading}
          />
        </div>
      </DetailPageLayout>
    )
  }
  if (!menu) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/menus" backLabel="Menus" title="Menu" />
        <p className="px-6 py-6 text-sm text-muted-foreground">Menu not found.</p>
      </DetailPageLayout>
    )
  }

  const metadataDirty = slug !== menu.slug || name !== menu.name
  const storesDirty = !sameStringSet(storeIds, menu.channelIds ?? [])

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/menus"
        backLabel="Menus"
        title={menu.name}
        meta={<span className="font-mono">{menu.slug}</span>}
        actions={
          canManage ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="text-destructive hover:bg-destructive/10 hover:text-destructive"
              onClick={() => {
                setDeleteMenuError(null)
                setConfirmingMenuDelete(true)
              }}
              disabled={deletingMenu}
            >
              <Trash2Icon className="size-4" />
              Delete
            </Button>
          ) : undefined
        }
      />

      <DetailPageBody>
        {(headerError || deleteMenuError) && (
          <div className="grid gap-3 px-6 py-4">
            {headerError && (
              <Alert variant="destructive">
                <AlertTitle>Save failed</AlertTitle>
                <AlertDescription>{headerError}</AlertDescription>
              </Alert>
            )}
            {deleteMenuError && (
              <Alert variant="destructive">
                <AlertTitle>Delete failed</AlertTitle>
                <AlertDescription>{deleteMenuError}</AlertDescription>
              </Alert>
            )}
          </div>
        )}

        <DetailPageSection
          title="Metadata"
          description="Channel slug and human-readable name. The slug is what the channel passes when resolving this menu at render time."
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="menu-edit-slug">Slug</Label>
              <Input
                id="menu-edit-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                disabled={!canManage || savingMenu}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="menu-edit-name">Name</Label>
              <Input
                id="menu-edit-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={!canManage || savingMenu}
              />
            </div>
          </div>
          {canManage && (
            <div className="flex justify-end">
              <Button
                onClick={onSaveMenu}
                disabled={savingMenu || !metadataDirty}
                size="sm"
              >
                {savingMenu ? 'Saving…' : 'Save'}
              </Button>
            </div>
          )}
        </DetailPageSection>

        <DetailPageSection
          title="Channels"
          description="Channels that serve this menu. Unchecked channels get a null response when they query the slug - keep the menu unassigned to use it as a draft."
          trailing={
            canManage ? (
              <Button
                size="sm"
                onClick={() =>
                  setMenuStores({
                    variables: { menuId: menu.id, channelIds: storeIds },
                  })
                }
                disabled={savingStores || !storesDirty}
              >
                {savingStores ? 'Saving…' : 'Save stores'}
              </Button>
            ) : undefined
          }
        >
          <div className="flex flex-wrap gap-2">
            {allStores.map((s) => {
              const checked = storeIds.includes(s.id)
              return (
                <label
                  key={s.id}
                  className={`flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-sm ${
                    checked
                      ? 'border-foreground bg-accent'
                      : 'border-border'
                  }`}
                >
                  <input
                    type="checkbox"
                    className="accent-foreground"
                    checked={checked}
                    disabled={!canManage}
                    onChange={(e) => {
                      setStoreIds((prev) =>
                        e.target.checked
                          ? [...prev, s.id]
                          : prev.filter((sid) => sid !== s.id),
                      )
                    }}
                  />
                  <span>
                    {s.name}{' '}
                    <span className="text-xs text-muted-foreground">· {s.slug}</span>
                  </span>
                </label>
              )
            })}
            {allStores.length === 0 && (
              <p className="text-sm text-muted-foreground">
                No channels configured yet.
              </p>
            )}
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Items"
          description="Tree of links served to the channel. Use the arrows to reorder siblings, the plus to add a child, and the pencil to edit labels and translations."
          trailing={
            canManage ? (
              <Button size="sm" onClick={() => setCreating({ parentId: null })}>
                <PlusIcon className="size-4" /> Add item
              </Button>
            ) : undefined
          }
        >
          {tree.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No items yet. Click <em>Add item</em> to start.
            </p>
          ) : (
            <ul className="space-y-1">
              {tree.map((node) => (
                <TreeRow
                  key={node.id}
                  node={node}
                  depth={0}
                  canManage={canManage}
                  onAddChild={(parentId) => setCreating({ parentId })}
                  onEdit={(item) => setEditing(item)}
                  onDelete={(item) => setConfirmingDelete(item)}
                  onMove={onMove}
                />
              ))}
            </ul>
          )}
        </DetailPageSection>
        <PluginSections target="menu" entity={menu} mode="edit" />
      </DetailPageBody>

      {/* Create item dialog */}
      <ItemDialog
        title="Add menu item"
        open={creating !== null}
        onOpenChange={(o) => setCreating(o ? creating : null)}
        items={items}
        initialParentId={creating?.parentId ?? null}
        busy={creatingItem}
        onSubmit={async (values) => {
          await createMenuItem({
            variables: {
              input: {
                menuId: menu.id,
                parentId: values.parentId,
                label: values.label,
                targetType: values.targetType,
                targetId: values.targetId,
                url: values.url,
                sortOrder: items.filter(
                  (i) => (i.parentId ?? null) === (values.parentId ?? null),
                ).length,
              },
            },
          })
          setCreating(null)
        }}
      />

      {/* Edit item dialog */}
      <ItemDialog
        title="Edit menu item"
        open={editing !== null}
        onOpenChange={(o) => setEditing(o ? editing : null)}
        items={items}
        initialItem={editing ?? undefined}
        busy={updatingItem}
        availableLocales={allLocales.map((l) => ({
          code: l.code,
          name: l.nativeName || l.name,
        }))}
        onUpsertTranslation={async (locale, label) => {
          if (!editing) return
          await upsertTranslation({
            variables: { itemId: editing.id, locale, label },
          })
        }}
        onDeleteTranslation={async (locale) => {
          if (!editing) return
          await deleteTranslation({
            variables: { itemId: editing.id, locale },
          })
        }}
        onSubmit={async (values) => {
          if (!editing) return
          // ClearParent is the canonical "null this out" signal -
          // distinct from parentId=null which means "no change"
          const clearParent = editing.parentId !== null && values.parentId === null
          await updateMenuItem({
            variables: {
              input: {
                id: editing.id,
                label: values.label,
                targetType: values.targetType,
                targetId: values.targetId,
                url: values.url,
                parentId: clearParent ? null : values.parentId,
                clearParent,
              },
            },
          })
          setEditing(null)
        }}
      />

      <AlertDialog
        open={confirmingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setConfirmingDelete(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete menu item?</AlertDialogTitle>
            <AlertDialogDescription>
              Children of this item are deleted along with it.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletingItem}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={onConfirmDelete} disabled={deletingItem}>
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={confirmingMenuDelete}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmingMenuDelete(false)
            setDeleteMenuError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {menu.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the menu and every item inside it. Channels referencing
              the slug fall back to their empty state.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletingMenu}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={onConfirmMenuDelete}
              disabled={deletingMenu}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deletingMenu ? 'Deleting…' : 'Delete menu'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}

type TreeNode = MenuItem & { children: TreeNode[] }

// sameStringSet returns true when two string arrays carry the same values regardless of order - Used to dim the Save
// button when the in-progress edit set matches the persisted state
function sameStringSet(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const sa = new Set(a)
  for (const x of b) {
    if (!sa.has(x)) return false
  }
  return true
}

function buildTree(flat: MenuItem[]): TreeNode[] {
  const byParent = new Map<string | null, MenuItem[]>()
  for (const it of flat) {
    const key = it.parentId ?? null
    const bucket = byParent.get(key) ?? []
    bucket.push(it)
    byParent.set(key, bucket)
  }
  for (const bucket of byParent.values()) {
    bucket.sort((a, b) => a.sortOrder - b.sortOrder)
  }
  function walk(parent: string | null): TreeNode[] {
    return (byParent.get(parent) ?? []).map((it) => ({
      ...it,
      children: walk(it.id),
    }))
  }
  return walk(null)
}

function TreeRow({
  node,
  depth,
  canManage,
  onAddChild,
  onEdit,
  onDelete,
  onMove,
}: {
  node: TreeNode
  depth: number
  canManage: boolean
  onAddChild: (parentId: string) => void
  onEdit: (item: MenuItem) => void
  onDelete: (item: MenuItem) => void
  onMove: (item: MenuItem, direction: -1 | 1) => void
}) {
  const targetSummary =
    node.targetType && node.targetId
      ? `${node.targetType.toLowerCase()} · ${node.targetId.slice(0, 8)}…`
      : node.url
  return (
    <li>
      <div
        className="flex items-center gap-2 rounded-md border border-border bg-card px-3 py-2"
        style={{ marginLeft: depth * 20 }}
      >
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{node.label}</p>
          <p className="truncate text-xs text-muted-foreground">{targetSummary}</p>
        </div>
        {canManage && (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              onClick={() => onMove(node, -1)}
              aria-label="Move up"
            >
              <ChevronUpIcon className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              onClick={() => onMove(node, 1)}
              aria-label="Move down"
            >
              <ChevronDownIcon className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              onClick={() => onAddChild(node.id)}
              aria-label="Add child"
            >
              <PlusIcon className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              onClick={() => onEdit(node)}
              aria-label="Edit"
            >
              <PencilIcon className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-7 text-destructive"
              onClick={() => onDelete(node)}
              aria-label="Delete"
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          </div>
        )}
      </div>
      {node.children.length > 0 && (
        <ul className="mt-1 space-y-1">
          {node.children.map((child) => (
            <TreeRow
              key={child.id}
              node={child}
              depth={depth + 1}
              canManage={canManage}
              onAddChild={onAddChild}
              onEdit={onEdit}
              onDelete={onDelete}
              onMove={onMove}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

type ItemFormValues = {
  label: string
  targetType: MenuItemTargetType | null
  targetId: string | null
  url: string | null
  parentId: string | null
}

function ItemDialog({
  title,
  open,
  onOpenChange,
  items,
  initialItem,
  initialParentId,
  busy,
  availableLocales,
  onUpsertTranslation,
  onDeleteTranslation,
  onSubmit,
}: {
  title: string
  open: boolean
  onOpenChange: (open: boolean) => void
  items: MenuItem[]
  initialItem?: MenuItem
  initialParentId?: string | null
  busy: boolean
  // When provided (edit mode), the dialog renders a translations editor under the main form - The handlers fire
  // immediately on add / remove so the parent's query refetch picks them up
  availableLocales?: { code: string; name: string }[]
  onUpsertTranslation?: (locale: string, label: string) => Promise<void>
  onDeleteTranslation?: (locale: string) => Promise<void>
  onSubmit: (values: ItemFormValues) => Promise<void>
}) {
  const [label, setLabel] = useState('')
  // Single dropdown for "URL or entity-kind"; storage is split on save
  // into targetType+targetId vs url, honouring the backend's XOR rule
  const [kind, setKind] = useState<'URL' | MenuItemTargetType>('URL')
  const [targetId, setTargetId] = useState('')
  const [url, setUrl] = useState('')
  const [parentId, setParentId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Reset whenever the dialog opens or its inputs change - We sync from either initialItem (edit) or initialParentId (create-as-child) -
  // never both, since `initialItem` always wins when present - State is adjusted during render instead of in an effect
  const [prevSource, setPrevSource] = useState({ open, initialItem, initialParentId })
  if (
    prevSource.open !== open ||
    prevSource.initialItem !== initialItem ||
    prevSource.initialParentId !== initialParentId
  ) {
    setPrevSource({ open, initialItem, initialParentId })
    if (open) {
      if (initialItem) {
        setLabel(initialItem.label)
        setKind(initialItem.targetType ?? 'URL')
        setTargetId(initialItem.targetId ?? '')
        setUrl(initialItem.url ?? '')
        setParentId(initialItem.parentId)
      } else {
        setLabel('')
        setKind('URL')
        setTargetId('')
        setUrl('')
        setParentId(initialParentId ?? null)
      }
      setError(null)
    }
  }

  async function onClickSave() {
    setError(null)
    const trimmedLabel = label.trim()
    if (!trimmedLabel) {
      setError('Label is required.')
      return
    }
    if (kind === 'URL') {
      if (!url.trim()) {
        setError('URL is required.')
        return
      }
    } else if (!targetId.trim()) {
      setError('Target ID is required.')
      return
    }
    try {
      await onSubmit({
        label: trimmedLabel,
        targetType: kind === 'URL' ? null : kind,
        targetId: kind === 'URL' ? null : targetId.trim(),
        url: kind === 'URL' ? url.trim() : null,
        parentId,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  // Parent select: every item except this one (avoids picking yourself as your parent) and its descendants (would form
  // a cycle) - The backend re-validates, but pruning the dropdown keeps the UI honest
  const parentChoices = useMemo(() => {
    const banned = new Set<string>()
    if (initialItem) {
      banned.add(initialItem.id)
      // Walk descendants
      const queue = [initialItem.id]
      while (queue.length) {
        const cur = queue.shift()!
        for (const it of items) {
          if (it.parentId === cur) {
            banned.add(it.id)
            queue.push(it.id)
          }
        }
      }
    }
    return items.filter((i) => !banned.has(i.id))
  }, [items, initialItem])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            Items can link to a typed entity (resolved to its canonical URL by
            the channel) or to a free-form URL.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="item-label">Label</Label>
            <Input
              id="item-label"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              autoFocus
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="item-kind">Target</Label>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as typeof kind)}
            >
              <SelectTrigger id="item-kind">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TARGET_KINDS.map((k) => (
                  <SelectItem key={k.value} value={k.value}>
                    {k.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {kind === 'URL' ? (
            <div className="grid gap-1.5">
              <Label htmlFor="item-url">URL</Label>
              <Input
                id="item-url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="/contact or https://example.com"
              />
            </div>
          ) : (
            <div className="grid gap-1.5">
              <Label htmlFor="item-target-id">Entity ID</Label>
              <Input
                id="item-target-id"
                value={targetId}
                onChange={(e) => setTargetId(e.target.value)}
                placeholder="UUID of the target entity"
              />
              <p className="text-xs text-muted-foreground">
                Paste the entity's UUID for now. A proper entity picker is on
                the roadmap.
              </p>
            </div>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="item-parent">Parent</Label>
            <Select
              value={parentId ?? '__root__'}
              onValueChange={(v) =>
                setParentId(v === '__root__' ? null : v)
              }
            >
              <SelectTrigger id="item-parent">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__root__">- top level -</SelectItem>
                {parentChoices.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          {initialItem && availableLocales && (
            <TranslationsEditor
              translations={initialItem.translations}
              availableLocales={availableLocales}
              onUpsert={onUpsertTranslation}
              onDelete={onDeleteTranslation}
            />
          )}
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={busy}
          >
            Cancel
          </Button>
          <Button onClick={onClickSave} disabled={busy}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// seedDrafts maps the persisted translations into the per-row draft
// record the editor below binds its inputs to
function seedDrafts(translations: Translation[]): Record<string, string> {
  const next: Record<string, string> = {}
  for (const t of translations) {
    next[t.locale] = t.label
  }
  return next
}

// Per-locale label override editor - Only rendered inside the edit
// dialog; create-then-translate is a deliberate two-step (the
// translation rows need an item_id, which only exists post-create)
//
// Each row is a label input + remove button - Adding picks a locale
// from the remaining "not yet translated" list - Both mutations call
// back through to the parent so the menu query refetches and the
// translation list refreshes
function TranslationsEditor({
  translations,
  availableLocales,
  onUpsert,
  onDelete,
}: {
  translations: Translation[]
  availableLocales: { code: string; name: string }[]
  onUpsert?: (locale: string, label: string) => Promise<void>
  onDelete?: (locale: string) => Promise<void>
}) {
  const [drafts, setDrafts] = useState<Record<string, string>>(() => seedDrafts(translations))
  const [adding, setAdding] = useState<string | null>(null)
  const [newLabel, setNewLabel] = useState('')
  const [pending, setPending] = useState<string | null>(null)

  // Re-seed the per-row inputs whenever the source translations change
  // (state adjusted during render instead of in an effect)
  const [prevTranslations, setPrevTranslations] = useState(translations)
  if (prevTranslations !== translations) {
    setPrevTranslations(translations)
    setDrafts(seedDrafts(translations))
  }

  const existingLocales = new Set(translations.map((t) => t.locale))
  const remainingLocales = availableLocales.filter(
    (l) => !existingLocales.has(l.code),
  )

  return (
    <div className="rounded-md border border-border p-3">
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Translations
        </h3>
        {remainingLocales.length > 0 && adding === null && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setAdding(remainingLocales[0].code)
              setNewLabel('')
            }}
          >
            <PlusIcon className="size-3.5" /> Add locale
          </Button>
        )}
      </div>
      {translations.length === 0 && adding === null && (
        <p className="text-xs text-muted-foreground">
          No translations yet. The base label is rendered for every locale until
          one is added.
        </p>
      )}
      <div className="space-y-2">
        {translations.map((t) => (
          <div key={t.locale} className="flex items-center gap-2">
            <span className="w-12 shrink-0 font-mono text-xs text-muted-foreground">
              {t.locale}
            </span>
            <Input
              value={drafts[t.locale] ?? ''}
              onChange={(e) =>
                setDrafts((prev) => ({ ...prev, [t.locale]: e.target.value }))
              }
              onBlur={async () => {
                const value = (drafts[t.locale] ?? '').trim()
                if (value && value !== t.label && onUpsert) {
                  setPending(t.locale)
                  try {
                    await onUpsert(t.locale, value)
                  } finally {
                    setPending(null)
                  }
                }
              }}
              disabled={pending === t.locale}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7 text-destructive"
              onClick={async () => {
                if (!onDelete) return
                setPending(t.locale)
                try {
                  await onDelete(t.locale)
                } finally {
                  setPending(null)
                }
              }}
              aria-label={`Remove ${t.locale} translation`}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          </div>
        ))}
        {adding !== null && (
          <div className="flex items-center gap-2">
            <Select value={adding} onValueChange={(v) => setAdding(v)}>
              <SelectTrigger className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {remainingLocales.map((l) => (
                  <SelectItem key={l.code} value={l.code}>
                    {l.code} · {l.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              value={newLabel}
              onChange={(e) => setNewLabel(e.target.value)}
              placeholder="Translated label"
              autoFocus
            />
            <Button
              type="button"
              size="sm"
              onClick={async () => {
                if (!onUpsert || !newLabel.trim()) return
                setPending(adding)
                try {
                  await onUpsert(adding, newLabel.trim())
                  setAdding(null)
                  setNewLabel('')
                } finally {
                  setPending(null)
                }
              }}
              disabled={pending === adding || !newLabel.trim()}
            >
              Add
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setAdding(null)
                setNewLabel('')
              }}
            >
              Cancel
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
