import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { ChevronDownIcon, ChevronUpIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'

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
  type MenuDetailQuery,
} from './Menus.gql'
import { PluginSections } from '@/plugins/detail-sections'
import {
  MENU_LABELS,
  TARGET_KINDS,
  menuChanges,
  menuDiff,
  menuItemSchema,
  menuSchema,
  toMenuItemInput,
  toMenuItemValues,
  toMenuValues,
  translationChanges,
} from './schemas/menu'

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

type Menu = NonNullable<MenuDetailQuery['menu']>

// MenuDetail is the /menus/:id editor - the menu's slug, name and channels are its form, behind the
// header Save; items are a tree of rows edited through their own dialog and written straight away
export function MenuDetail({ menuId }: { menuId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = menuId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery(MenuDetailDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const menu = data?.menu

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
          <LoadError title="Couldn't load menu" message={error.message} onRetry={() => void refetch()} retrying={loading} />
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
  return <MenuForm menu={menu} />
}

function MenuForm({ menu }: { menu: Menu }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.manage')

  const refetchQueries = [{ query: MenuDetailDocument, variables: { id: menu.id } }]
  // Slug, name and channels surface in the menus list too
  const listRefetch = [...refetchQueries, { query: MenusListDocument }]
  const [updateMenu] = useMutation(UpdateMenuDocument, { refetchQueries: listRefetch })
  // setMenuStores answers with the whole hydrated menu but still refetches the detail: it runs after
  // updateMenu in the same save, and updateMenu's bare-row answer (no channels) can otherwise land
  // last through its own refetch
  const [setMenuStores] = useMutation(SetMenuStoresDocument, { refetchQueries: listRefetch })
  const [deleteMenu, { loading: deletingMenu }] = useMutation(DeleteMenuDocument, { refetchQueries: [MenusListDocument] })
  const [createMenuItem] = useMutation(CreateMenuItemDocument, { refetchQueries })
  const [updateMenuItem] = useMutation(UpdateMenuItemDocument, { refetchQueries })
  const [deleteMenuItem] = useMutation(DeleteMenuItemDocument, { refetchQueries })
  // Answers with the whole hydrated menu, the same one the detail query reads
  const [reorderMenuItems] = useMutation(ReorderMenuItemsDocument)
  const [upsertTranslation] = useMutation(UpsertMenuItemTranslationDocument, { refetchQueries })
  const [deleteTranslation] = useMutation(DeleteMenuItemTranslationDocument, { refetchQueries })

  const channelsData = useQuery(ChannelsDocument).data
  const channels = useMemo(() => channelsData?.channels ?? [], [channelsData])
  const locales = useQuery(LocalesPickerDocument).data?.locales ?? []

  const [editing, setEditing] = useState<MenuItem | null>(null)
  const [creating, setCreating] = useState<{ parentId: string | null } | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState<MenuItem | null>(null)
  const [confirmingMenuDelete, setConfirmingMenuDelete] = useState(false)
  const [deleteMenuError, setDeleteMenuError] = useState<string | null>(null)

  const items = useMemo(() => menu.items as MenuItem[], [menu])
  // Flat -> tree, sorted by parent then sortOrder - the render walks it recursively
  const tree = useMemo(() => buildTree(items), [items])

  const baseline = useMemo(() => toMenuValues(menu), [menu])
  const diff = useMemo(() => menuDiff((id) => channels.find((c) => c.id === id)?.name ?? id), [channels])
  const record = useRecordForm({
    id: 'menu',
    title: 'Menu',
    label: `Menu · ${menu.name}`,
    isEdit: true,
    baseline,
    schema: menuSchema,
    diff,
    persist: { routeKey: `menu:${menu.id}` },
    submit: async (v) => {
      const changes = menuChanges(baseline, v)
      if (changes.update) await updateMenu({ variables: { input: { id: menu.id, ...changes.update } } })
      if (changes.channelIds) await setMenuStores({ variables: { menuId: menu.id, channelIds: changes.channelIds } })
    },
  })
  const { form } = record
  const channelIds = useStore(form.store, (s) => s.values.channelIds)

  async function onMove(item: MenuItem, direction: -1 | 1) {
    const siblings = items.filter((i) => (i.parentId ?? null) === (item.parentId ?? null)).sort((a, b) => a.sortOrder - b.sortOrder)
    const idx = siblings.findIndex((i) => i.id === item.id)
    const neighbor = siblings[idx + direction]
    if (!neighbor) return
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

  async function onConfirmMenuDelete() {
    setDeleteMenuError(null)
    try {
      await deleteMenu({ variables: { id: menu.id } })
      navigate('/menus', { replace: true })
    } catch (err) {
      setDeleteMenuError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <RecordFormPage
      record={record}
      backTo="/menus"
      backLabel="Menus"
      title={menu.name}
      meta={<span className="font-mono">{menu.slug}</span>}
      actions={
        canManage && (
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
        )
      }
      createLabel="Save changes"
      labels={MENU_LABELS}
      hideSave={!canManage}
    >
      {deleteMenuError && (
        <div className="px-6 py-4">
          <Alert variant="destructive">
            <AlertTitle>Delete failed</AlertTitle>
            <AlertDescription>{deleteMenuError}</AlertDescription>
          </Alert>
        </div>
      )}

      <DetailPageSection
        title="Metadata"
        description="Channel slug and human-readable name. The slug is what the channel passes when resolving this menu at render time."
      >
        <fieldset disabled={!canManage} className="grid items-start gap-3 sm:grid-cols-2">
          <form.AppField name="slug">{(f) => <f.TextField id="menu-edit-slug" label="Slug" />}</form.AppField>
          <form.AppField name="name">{(f) => <f.TextField id="menu-edit-name" label="Name" />}</form.AppField>
        </fieldset>
      </DetailPageSection>

      <DetailPageSection
        title="Channels"
        description="Channels that serve this menu. Unchecked channels get a null response when they query the slug - keep the menu unassigned to use it as a draft."
      >
        <fieldset disabled={!canManage} className="flex flex-wrap gap-2">
          {channels.map((s) => {
            const checked = channelIds.includes(s.id)
            return (
              <label
                key={s.id}
                className={`flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-sm ${checked ? 'border-foreground/40 bg-muted' : 'border-border'}`}
              >
                <input
                  type="checkbox"
                  className="accent-foreground"
                  checked={checked}
                  onChange={(e) =>
                    form.setFieldValue('channelIds', (prev) => (e.target.checked ? [...prev, s.id] : prev.filter((cid) => cid !== s.id)))
                  }
                />
                <span>
                  {s.name} <span className="text-xs text-muted-foreground">· {s.slug}</span>
                </span>
              </label>
            )
          })}
          {channels.length === 0 && <p className="text-sm text-muted-foreground">No channels configured yet.</p>}
        </fieldset>
      </DetailPageSection>

      <DetailPageSection
        title="Items"
        description="Tree of links served to the channel. Use the arrows to reorder siblings, the plus to add a child, and the pencil to edit labels and translations."
        trailing={
          canManage ? (
            <Button type="button" size="sm" onClick={() => setCreating({ parentId: null })}>
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

      {/* Radix unmounts the content on close, so each open starts from the item it edits */}
      <Dialog open={creating !== null} onOpenChange={(o) => !o && setCreating(null)}>
        <DialogContent>
          {creating && (
            <ItemForm
              title="Add menu item"
              items={items}
              parentId={creating.parentId}
              onCancel={() => setCreating(null)}
              onSaved={() => setCreating(null)}
              submit={async (v) => {
                const input = toMenuItemInput(v)
                await createMenuItem({
                  variables: {
                    input: {
                      menuId: menu.id,
                      ...input,
                      sortOrder: items.filter((i) => (i.parentId ?? null) === input.parentId).length,
                    },
                  },
                })
              }}
            />
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={editing !== null} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent className="max-h-[85vh]">
          {editing && (
            <ItemForm
              title="Edit menu item"
              items={items}
              item={editing}
              locales={locales.map((l) => ({ code: l.code, name: l.nativeName || l.name }))}
              onCancel={() => setEditing(null)}
              onSaved={() => setEditing(null)}
              submit={async (v, before) => {
                const input = toMenuItemInput(v)
                // clearParent is the "null this out" signal - a null parentId alone means "no change"
                const clearParent = editing.parentId !== null && input.parentId === null
                await updateMenuItem({ variables: { input: { id: editing.id, ...input, clearParent } } })
                const t = translationChanges(before, v)
                for (const { locale, label } of t.upserts) await upsertTranslation({ variables: { itemId: editing.id, locale, label } })
                for (const locale of t.deletes) await deleteTranslation({ variables: { itemId: editing.id, locale } })
              }}
            />
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmingDelete !== null}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title="Delete menu item?"
        description="Children of this item are deleted along with it."
        confirmLabel="Delete"
        // Not awaited: the dialog closes at once, as it always has
        onConfirm={() => {
          if (confirmingDelete) void deleteMenuItem({ variables: { id: confirmingDelete.id } })
        }}
      />

      <ConfirmDialog
        open={confirmingMenuDelete}
        onOpenChange={setConfirmingMenuDelete}
        title={<>Delete {menu.name}?</>}
        description="This removes the menu and every item inside it. Channels referencing the slug fall back to their empty state."
        confirmLabel="Delete menu"
        busyLabel="Deleting…"
        destructive
        // Not awaited: the dialog closes at once and a failure shows on the page, as it always has
        onConfirm={() => void onConfirmMenuDelete()}
      />
    </RecordFormPage>
  )
}

type TreeNode = MenuItem & { children: TreeNode[] }

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
    return (byParent.get(parent) ?? []).map((it) => ({ ...it, children: walk(it.id) }))
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
  const targetSummary = node.targetType && node.targetId ? `${node.targetType.toLowerCase()} · ${node.targetId.slice(0, 8)}…` : node.url
  return (
    <li>
      <div className="flex items-center gap-2 rounded-md border border-border bg-card px-3 py-2" style={{ marginLeft: depth * 20 }}>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{node.label}</p>
          <p className="truncate text-xs text-muted-foreground">{targetSummary}</p>
        </div>
        {canManage && (
          <div className="flex items-center gap-1">
            <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => onMove(node, -1)} aria-label="Move up">
              <ChevronUpIcon className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => onMove(node, 1)} aria-label="Move down">
              <ChevronDownIcon className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => onAddChild(node.id)} aria-label="Add child">
              <PlusIcon className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => onEdit(node)} aria-label="Edit">
              <PencilIcon className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-7 text-destructive" onClick={() => onDelete(node)} aria-label="Delete">
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

// A menu item - a CRUD dialog with its own Save, which the convention allows. Translations are part
// of it: they're staged with the rest and written when the dialog saves, not as each row is typed
function ItemForm({
  title,
  items,
  item,
  parentId,
  locales,
  onCancel,
  onSaved,
  submit,
}: {
  title: string
  items: MenuItem[]
  item?: MenuItem
  parentId?: string | null
  /** Edit only - translation rows need the item to exist */
  locales?: { code: string; name: string }[]
  onCancel: () => void
  onSaved: () => void
  submit: (values: ReturnType<typeof toMenuItemValues>, before: ReturnType<typeof toMenuItemValues>) => Promise<void>
}) {
  const [baseline] = useState(() => toMenuItemValues(item, parentId))
  const dialog = useDialogForm({ baseline, schema: menuItemSchema, submit: (v) => submit(v, baseline), onSaved })
  const { form } = dialog
  const kind = useStore(form.store, (s) => s.values.kind)

  // Every item but this one and its descendants - picking either as the parent would form a cycle.
  // The backend re-validates, but pruning the list keeps the UI honest
  const parents = useMemo(() => {
    const banned = new Set<string>()
    if (item) {
      const queue = [item.id]
      banned.add(item.id)
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
  }, [items, item])

  return (
    <FormDialogBody
      dialog={dialog}
      title={title}
      description="Items can link to a typed entity (resolved to its canonical URL by the channel) or to a free-form URL."
      submitLabel="Save"
      onCancel={onCancel}
      scrollable={!!locales}
    >
      <form.AppField name="label">{(f) => <f.TextField id="item-label" label="Label" autoFocus />}</form.AppField>
      <form.AppField name="kind">
        {(f) => <f.SelectField id="item-kind" label="Target" options={TARGET_KINDS.map((k) => ({ value: k.value, label: k.label }))} />}
      </form.AppField>
      {kind === 'URL' ? (
        <form.AppField name="url">
          {(f) => <f.TextField id="item-url" label="URL" placeholder="/contact or https://example.com" />}
        </form.AppField>
      ) : (
        <form.AppField name="targetId">
          {(f) => (
            <f.TextField
              id="item-target-id"
              label="Entity ID"
              placeholder="UUID of the target entity"
              hint="Paste the entity's UUID for now. A proper entity picker is on the roadmap."
            />
          )}
        </form.AppField>
      )}
      <form.AppField name="parentId">
        {(f) => <f.SelectField id="item-parent" label="Parent" noneLabel="- top level -" options={parents.map((p) => ({ value: p.id, label: p.label }))} />}
      </form.AppField>
      {locales && <TranslationsEditor form={form} locales={locales} />}
    </FormDialogBody>
  )
}

// Per-locale label overrides, edit only - create-then-translate is a deliberate two-step, since a
// translation row needs the item's id. Emptying a row removes that translation on save
function TranslationsEditor({
  form,
  locales,
}: {
  form: ReturnType<typeof useDialogForm<ReturnType<typeof toMenuItemValues>>>['form']
  locales: { code: string; name: string }[]
}) {
  const translations = useStore(form.store, (s) => s.values.translations)
  const [adding, setAdding] = useState<string | null>(null)
  const used = Object.keys(translations)
  const remaining = locales.filter((l) => !used.includes(l.code))
  const set = (locale: string, label: string) => form.setFieldValue('translations', (prev) => ({ ...prev, [locale]: label }))

  return (
    <div className="rounded-md border border-border p-3">
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Translations</h3>
        {remaining.length > 0 && adding === null && (
          <Button type="button" size="sm" variant="ghost" onClick={() => setAdding(remaining[0].code)}>
            <PlusIcon className="size-3.5" /> Add locale
          </Button>
        )}
      </div>
      {used.length === 0 && adding === null && (
        <p className="text-xs text-muted-foreground">No translations yet. The base label is rendered for every locale until one is added.</p>
      )}
      <div className="space-y-2">
        {used.map((locale) => (
          <div key={locale} className="flex items-center gap-2">
            <span className="w-12 shrink-0 font-mono text-xs text-muted-foreground">{locale}</span>
            <Input aria-label={`Label (${locale})`} value={translations[locale] ?? ''} onChange={(e) => set(locale, e.target.value)} />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7 text-destructive"
              onClick={() =>
                form.setFieldValue('translations', (prev) => {
                  const next = { ...prev }
                  delete next[locale]
                  return next
                })
              }
              aria-label={`Remove ${locale} translation`}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          </div>
        ))}
        {adding !== null && (
          <div className="flex items-center gap-2">
            <Select value={adding} onValueChange={setAdding}>
              <SelectTrigger className="w-32" aria-label="New translation locale">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {remaining.map((l) => (
                  <SelectItem key={l.code} value={l.code}>
                    {l.code} · {l.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              type="button"
              size="sm"
              className="h-10"
              onClick={() => {
                set(adding, '')
                setAdding(null)
              }}
            >
              Add
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => setAdding(null)}>
              Cancel
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
