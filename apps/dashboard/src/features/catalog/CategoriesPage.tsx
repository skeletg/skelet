import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import {
  ChevronDownIcon,
  ChevronRightIcon,
  FolderIcon,
  FolderOpenIcon,
  GripVerticalIcon,
  ImageIcon,
  PlusIcon,
  SearchIcon,
  Trash2Icon,
  XIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { DetailPageLayout } from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope } from '@/lib/config-scope'
import { cn } from '@/lib/utils'
import { useScrollHideHeader } from '@/lib/use-scroll-hide-header'
import {
  AddProductToCategoryDocument,
  ProductsDocument,
  RemoveProductFromCategoryDocument,
} from './Catalog.gql'
import { LexicalRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'
import { TranslationLocales } from '@/components/TranslationLocales'
import {
  CategoryDocument,
  ClearCategoryCoverDocument,
  ClearCategoryListingDocument,
  ConfirmCategoryCoverUploadDocument,
  CreateCategoryDocument,
  DeleteCategoryContentDocument,
  DeleteCategoryDocument,
  RequestCategoryCoverUploadDocument,
  RootCategoriesDocument,
  SetCategoryListingDocument,
  UpdateCategoryDocument,
  UpsertCategoryContentDocument,
  type CategoryDetail,
  type CategorySummary,
} from './Categories.gql'
import { SetCategoryFacetOverridesDocument, type CategoryFacetOverride } from './Categories.gql'
import { AttributesDocument } from './Catalog.gql'
import { FacetOverridesEditor } from './FacetOverridesEditor'
import { sortFacetCandidates, useFacetOverridesDraft } from './facet-overrides-draft'
import { readImageDimensions } from './product-detail/media-utils'
import {
  CategorySaveContext,
  useCategorySaveSection,
  type CategorySaveContextValue,
  type SectionDiffEntry,
  type SectionRegistration,
} from './categorySaveRegistry'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'

// ─── helpers ─────────────────────────────────────────────────────────────────

function slugify(name: string) {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
}

// formatScalar renders a value for the save-confirm modal's diff rows - The modal compares strings, so
// booleans/numbers/null all need a stable human-readable form
function formatScalar(v: unknown): string {
  if (v == null || v === '') return '-'
  if (v === true) return 'yes'
  if (v === false) return 'no'
  if (typeof v === 'number') return String(v)
  return String(v)
}

// summarizeRichText collapses a Lexical JSON value to a short signature for diff comparison - Two equal trees produce equal strings;
// different content produces different strings (any mismatch flips the diff) - A length cap keeps the modal readable
function summarizeRichText(v: unknown): string {
  if (v == null) return '-'
  try {
    const json = JSON.stringify(v)
    if (json.length === 0 || json === '"null"') return '-'
    return json.length > 60 ? `${json.slice(0, 57)}…` : json
  } catch {
    return '(rich text)'
  }
}

function countCategories(cats: CategorySummary[]): number {
  let n = 0
  for (const c of cats) {
    n += 1 + countCategories(c.children)
  }
  return n
}

function findCategoryAncestors(
  cats: CategorySummary[],
  target: string,
  path: string[],
): string[] | null {
  for (const c of cats) {
    if (c.id === target) return path
    const found = findCategoryAncestors(c.children, target, [...path, c.id])
    if (found) return found
  }
  return null
}

type FlatNode = {
  id: string
  slug: string
  name: string
  urlPath: string
  isPublished: boolean
  depth: number
  parentId: string | null
  position: number
  hasChildren: boolean
  expanded: boolean
}

// Display name with slug fallback - The backend's category-list resolver falls back through locales (active → default →
// any) when populating `name`, so this only hits the slug branch for categories with literally no translations
function resolveCategoryDisplayName(cat: CategorySummary): string {
  const resolved = cat.name?.trim()
  if (resolved) return resolved
  return cat.slug
}

function flattenTree(
  cats: CategorySummary[],
  expandedIds: Set<string>,
  depth = 0,
  parentId: string | null = null,
): FlatNode[] {
  const result: FlatNode[] = []
  for (const cat of cats) {
    const expanded = expandedIds.has(cat.id)
    result.push({
      id: cat.id,
      slug: cat.slug,
      name: resolveCategoryDisplayName(cat),
      urlPath: cat.urlPath,
      isPublished: cat.isPublished,
      depth,
      parentId,
      position: cat.position,
      hasChildren: cat.children.length > 0,
      expanded,
    })
    if (expanded && cat.children.length > 0) {
      result.push(...flattenTree(cat.children, expandedIds, depth + 1, cat.id))
    }
  }
  return result
}

// ─── main page ───────────────────────────────────────────────────────────────

export function CategoriesPage() {
  const { id: selectedId } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('catalog.manage')
  // Header drives the tree filter: CHANNEL shows only categories listed on the active store, GLOBAL the full
  // tree - structural edits (slug, parent, position) stay global-only, the tree just hides what the store lacks
  const { isGlobal } = useConfigScope()
  const canEditScope = canManage && isGlobal
  // Keep the tree query locale-agnostic: Apollo normalises Category by id, so passing different locales for one
  // id would overwrite the cached `name` across views - the server falls back through locales anyway, so the
  // tree always shows a real name when one exists in any translation
  // Channel scope rides the `X-Channel-Id` header (see lib/apollo.ts), not a
  // query var: CHANNEL scope returns that store's published tree, GLOBAL the
  // full tree
  const { data, loading, refetch } = useQuery(RootCategoriesDocument)
  const roots = useMemo(() => data?.rootCategories ?? [], [data])
  const totalCount = useMemo(() => countCategories(roots), [roots])

  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set())
  const [dragActiveId, setDragActiveId] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  // expand to selected category on load - open every ancestor so the row is visible, and the category itself so its
  // children are revealed - State is adjusted during render, keyed on the selection + loaded tree data
  const [prevExpandTarget, setPrevExpandTarget] = useState<{
    selectedId: string | undefined
    data: typeof data
  } | null>(null)
  if (
    prevExpandTarget === null ||
    prevExpandTarget.selectedId !== selectedId ||
    prevExpandTarget.data !== data
  ) {
    setPrevExpandTarget({ selectedId, data })
    if (selectedId && data) {
      const ancestors = findCategoryAncestors(roots, selectedId, [])
      if (ancestors) setExpandedIds((prev) => new Set([...prev, ...ancestors, selectedId]))
    }
  }

  // When the catalog is a single top-level category with children, start it expanded on load - a lone collapsed root
  // just hides the whole tree behind one click - One-shot (guarded) so it doesn't undo a manual collapse
  const [didAutoExpandRoot, setDidAutoExpandRoot] = useState(false)
  if (!didAutoExpandRoot && roots.length === 1 && roots[0].children.length > 0) {
    setDidAutoExpandRoot(true)
    setExpandedIds((prev) => new Set([...prev, roots[0].id]))
  }

  // When searching, auto-expand any branch containing a match
  const matchExpand = useMemo(() => {
    if (!search.trim()) return null
    const q = search.toLowerCase()
    const ids = new Set<string>()
    function walk(cats: CategorySummary[], path: string[]): boolean {
      let any = false
      for (const c of cats) {
        const self =
          resolveCategoryDisplayName(c).toLowerCase().includes(q) ||
          c.slug.toLowerCase().includes(q)
        const child = walk(c.children, [...path, c.id])
        if (self || child) {
          for (const p of path) ids.add(p)
          any = true
        }
      }
      return any
    }
    walk(roots, [])
    return ids
  }, [search, roots])

  const effectiveExpanded = useMemo(
    () => (matchExpand ? new Set([...expandedIds, ...matchExpand]) : expandedIds),
    [matchExpand, expandedIds],
  )

  const flat = useMemo(
    () => flattenTree(roots, effectiveExpanded),
    [roots, effectiveExpanded],
  )

  const visibleFlat = useMemo(() => {
    if (!search.trim()) return flat
    const q = search.toLowerCase()
    return flat.filter(
      (n) => n.name.toLowerCase().includes(q) || n.slug.toLowerCase().includes(q),
    )
  }, [flat, search])

  const [updateCategory] = useMutation(UpdateCategoryDocument, {
    refetchQueries: [{ query: RootCategoriesDocument }],
  })

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }))

  // ── drag state ──
  const [dropTarget, setDropTarget] = useState<{ id: string; zone: 'before' | 'on' | 'after' } | null>(null)

  function onDragStart({ active }: DragStartEvent) {
    setDragActiveId(active.id as string)
  }

  async function onDragEnd({ active, over }: DragEndEvent) {
    setDragActiveId(null)
    setDropTarget(null)
    if (!over || active.id === over.id || !dropTarget) return

    const draggedNode = flat.find((n) => n.id === active.id)
    const targetNode = flat.find((n) => n.id === dropTarget.id)
    if (!draggedNode || !targetNode) return

    try {
      if (dropTarget.zone === 'on') {
        await updateCategory({
          variables: { input: { id: draggedNode.id, parentId: targetNode.id } },
        })
      } else {
        const newPosition = dropTarget.zone === 'before' ? targetNode.position : targetNode.position + 1
        await updateCategory({
          variables: {
            input: {
              id: draggedNode.id,
              parentId: targetNode.parentId ?? undefined,
              clearParent: targetNode.parentId === null,
              position: newPosition,
            },
          },
        })
      }
      void refetch()
    } catch {
      // ignore - tree will stay unchanged
    }
  }

  function toggleExpand(id: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const activeNode = dragActiveId ? flat.find((n) => n.id === dragActiveId) : null

  return (
    <DetailPageLayout
      leftSidebar={
        <aside className="flex w-80 shrink-0 flex-col border-r bg-card">
        <div className="flex flex-col gap-3 border-b px-4 py-3">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-baseline gap-2">
              <h2 className="text-sm font-semibold tracking-tight">Categories</h2>
              <span className="text-xs text-muted-foreground">{totalCount}</span>
            </div>
            {canManage ? (
              <Button
                size="sm"
                variant="outline"
                className="h-7 gap-1 px-2 text-xs"
                onClick={() => navigate('/categories/new')}
                disabled={!canEditScope}
                title={canEditScope ? undefined : 'Switch the header to GLOBAL to add a category.'}
              >
                <PlusIcon className="size-3.5" />
                New
              </Button>
            ) : null}
          </div>
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search categories…"
              className="h-8 pl-8 text-xs"
            />
          </div>
        </div>

        {loading ? (
          <p className="px-4 py-3 text-xs text-muted-foreground">Loading…</p>
        ) : roots.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 py-12 text-center">
            <FolderIcon className="size-6 text-muted-foreground/60" />
            <p className="text-sm font-medium">No categories yet</p>
            {canManage ? (
              <Button size="sm" variant="outline" onClick={() => navigate('/categories/new')}>
                <PlusIcon className="size-3.5" /> Create your first
              </Button>
            ) : null}
          </div>
        ) : (
          <DndContext
            sensors={sensors}
            onDragStart={onDragStart}
            onDragEnd={onDragEnd}
            onDragCancel={() => { setDragActiveId(null); setDropTarget(null) }}
          >
            <div className="scrollbar-accent flex-1 overflow-y-auto py-1">
              {visibleFlat.length === 0 ? (
                <p className="px-4 py-6 text-center text-xs text-muted-foreground">
                  No matches for &ldquo;{search}&rdquo;
                </p>
              ) : (
                visibleFlat.map((node) => (
                  <TreeNode
                    key={node.id}
                    node={node}
                    isSelected={node.id === selectedId}
                    dropTarget={dropTarget}
                    onSelect={() => navigate(`/categories/${node.id}`)}
                    onToggle={() => toggleExpand(node.id)}
                    onDropTargetChange={setDropTarget}
                    canManage={canManage}
                  />
                ))
              )}
            </div>
            <DragOverlay>
              {activeNode ? (
                <div className="flex items-center gap-2 rounded-md border bg-card px-3 py-2 text-xs shadow-lg ring-1 ring-primary/20">
                  <FolderIcon className="size-3.5 text-primary" />
                  <span className="font-medium">{activeNode.name}</span>
                </div>
              ) : null}
            </DragOverlay>
          </DndContext>
        )}
      </aside>
      }
    >
      {selectedId === 'new' ? (
        <CategoryForm
          key="new"
          isNew
          roots={roots}
          canManage={canManage}
          onCreated={(id) => navigate(`/categories/${id}`)}
        />
      ) : selectedId ? (
        <CategoryForm
          key={selectedId}
          categoryId={selectedId}
          roots={roots}
          canManage={canManage}
          onDeleted={() => navigate('/categories')}
        />
      ) : (
        <EmptyState canManage={canManage} onNew={() => navigate('/categories/new')} />
      )}
    </DetailPageLayout>
  )
}

// ─── empty state ─────────────────────────────────────────────────────────────

function EmptyState({ canManage, onNew }: { canManage: boolean; onNew: () => void }) {
  return (
    <div className="flex h-full items-center justify-center p-8">
      <div className="flex max-w-sm flex-col items-center gap-3 text-center">
        <div className="grid size-12 place-items-center rounded-full bg-muted">
          <FolderIcon className="size-6 text-muted-foreground" />
        </div>
        <h2 className="text-base font-semibold">Pick a category</h2>
        <p className="text-sm text-muted-foreground">
          Select a category from the tree to view and edit its details, or start a new one.
        </p>
        {canManage ? (
          <Button size="sm" onClick={onNew}>
            <PlusIcon className="size-4" /> New category
          </Button>
        ) : null}
      </div>
    </div>
  )
}

// ─── section helpers ─────────────────────────────────────────────────────────

function Section({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: React.ReactNode
}) {
  return (
    <section className="grid gap-4 px-6 py-6">
      <div>
        <h3 className="text-lg font-semibold tracking-tight">
          <span className="mr-1.5 text-primary">#</span>{title}
        </h3>
        {description ? (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
      <div className="grid gap-4">{children}</div>
    </section>
  )
}

function ToggleRow({
  id,
  label,
  description,
  checked,
  onCheckedChange,
  disabled,
}: {
  id: string
  label: string
  description?: string
  checked: boolean
  onCheckedChange: (v: boolean) => void
  disabled?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-1">
      <div>
        <Label htmlFor={id} className="text-sm">{label}</Label>
        {description ? (
          <p className="text-xs text-muted-foreground">{description}</p>
        ) : null}
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  )
}

// ─── tree node ───────────────────────────────────────────────────────────────

function TreeNode({
  node,
  isSelected,
  dropTarget,
  onSelect,
  onToggle,
  onDropTargetChange,
  canManage,
}: {
  node: FlatNode
  isSelected: boolean
  dropTarget: { id: string; zone: 'before' | 'on' | 'after' } | null
  onSelect: () => void
  onToggle: () => void
  onDropTargetChange: (t: { id: string; zone: 'before' | 'on' | 'after' } | null) => void
  canManage: boolean
}) {
  const rowRef = useRef<HTMLDivElement>(null)

  const isDropBefore = dropTarget?.id === node.id && dropTarget.zone === 'before'
  const isDropOn = dropTarget?.id === node.id && dropTarget.zone === 'on'
  const isDropAfter = dropTarget?.id === node.id && dropTarget.zone === 'after'

  return (
    <div className="relative">
      {isDropBefore ? (
        <div className="pointer-events-none absolute left-2 right-2 top-0 h-0.5 -translate-y-1/2 rounded-full bg-primary" />
      ) : null}
      <div
        ref={rowRef}
        data-node-id={node.id}
        style={{ paddingLeft: `${node.depth * 1.125 + 0.5}rem` }}
        className={`group relative mx-1 flex cursor-pointer items-center gap-1 rounded-md py-1.5 pr-2 text-sm transition-colors
          ${isSelected
            ? 'bg-primary/10 text-foreground'
            : 'text-foreground/90 hover:bg-muted/60'
          }
          ${isDropOn ? 'ring-2 ring-inset ring-primary' : ''}
        `}
        onClick={onSelect}
        onDragOver={(e) => {
          e.preventDefault()
          if (!rowRef.current) return
          const rect = rowRef.current.getBoundingClientRect()
          const y = e.clientY - rect.top
          const pct = y / rect.height
          const zone = pct < 0.25 ? 'before' : pct > 0.75 ? 'after' : 'on'
          onDropTargetChange({ id: node.id, zone })
        }}
        onDragLeave={() => onDropTargetChange(null)}
      >
        {/* indent guides */}
        {node.depth > 0
          ? Array.from({ length: node.depth }).map((_, i) => (
              <span
                key={i}
                aria-hidden
                className="pointer-events-none absolute top-0 bottom-0 w-px bg-border/60"
                style={{ left: `${i * 1.125 + 0.875}rem` }}
              />
            ))
          : null}

        {canManage ? (
          <span
            className="cursor-grab text-muted-foreground/60 transition-colors hover:text-muted-foreground active:cursor-grabbing"
            draggable
            onDragStart={(e) => e.dataTransfer.setData('text/plain', node.id)}
            onClick={(e) => e.stopPropagation()}
            title="Drag to reorder"
          >
            <GripVerticalIcon className="size-3.5 opacity-0 group-hover:opacity-100" />
          </span>
        ) : (
          <span className="size-3.5" />
        )}
        <button
          type="button"
          className="grid size-4 shrink-0 place-items-center text-muted-foreground hover:text-foreground"
          onClick={(e) => { e.stopPropagation(); onToggle() }}
          aria-label={node.expanded ? 'Collapse' : 'Expand'}
        >
          {node.hasChildren
            ? node.expanded
              ? <ChevronDownIcon className="size-3.5" />
              : <ChevronRightIcon className="size-3.5" />
            : null}
        </button>
        {node.expanded && node.hasChildren ? (
          <FolderOpenIcon className={`size-4 shrink-0 ${isSelected ? 'text-primary' : 'text-muted-foreground'}`} />
        ) : (
          <FolderIcon className={`size-4 shrink-0 ${isSelected ? 'text-primary' : 'text-muted-foreground'}`} />
        )}
        <span className={`flex-1 truncate ${isSelected ? 'font-medium' : ''}`}>{node.name}</span>
        <span
          className={`size-1.5 shrink-0 rounded-full ${
            node.isPublished ? 'bg-emerald-500' : 'bg-muted-foreground/40'
          }`}
          title={node.isPublished ? 'Published' : 'Draft'}
        />
      </div>
      {isDropAfter ? (
        <div className="pointer-events-none absolute left-2 right-2 bottom-0 h-0.5 translate-y-1/2 rounded-full bg-primary" />
      ) : null}
    </div>
  )
}

// ─── category form (right panel) ─────────────────────────────────────────────

function CategoryForm({
  categoryId,
  isNew = false,
  roots,
  canManage,
  onCreated,
  onDeleted,
}: {
  categoryId?: string
  isNew?: boolean
  roots: CategorySummary[]
  canManage: boolean
  onCreated?: (id: string) => void
  onDeleted?: () => void
}) {
  // Header drives the per-store auto-list on create - When the user is
  // working in CHANNEL=US and creates a category, the new category is
  // immediately listed on US (isPublished=true) so the user sees it where
  // they expect - In GLOBAL mode no listing is added - the operator can
  // pick stores explicitly afterwards
  const { isGlobal, effectiveChannelId, effectiveLocale, globalDefaultLocale, selectedChannel } = useConfigScope()
  // Locale this form edits - Falls back to the system-wide default when the
  // scope hasn't resolved one (GLOBAL with no scope.locale set) - Apollo's
  // `localeLink` attaches the same value as the `Locale` request header,
  // so the server resolves the same translation we render against
  const locale = effectiveLocale || globalDefaultLocale
  const { data, loading } = useQuery(CategoryDocument, {
    variables: { id: categoryId! },
    skip: isNew || !categoryId,
  })
  const category = data?.category

  const [name, setName] = useState('')
  const [slugManual, setSlugManual] = useState(false)
  const [slug, setSlug] = useState('')
  const [parentId, setParentId] = useState<string | null>(null)
  const [position, setPosition] = useState(0)
  const [isPublished, setIsPublished] = useState(false)
  const [seoNoindex, setSeoNoindex] = useState(false)
  const [shortDescription, setShortDescription] = useState<RichTextValue>(null)
  const [description, setDescription] = useState<RichTextValue>(null)
  const [metaTitle, setMetaTitle] = useState('')
  const [metaDescription, setMetaDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [ready, setReady] = useState(isNew)
  const { ref: headerRef, hidden: headerHidden } = useScrollHideHeader<HTMLDivElement>()

  // Hydrate the form when the category loads/refetches or the edited locale
  // changes (state adjusted during render, keyed on both)
  const [prevHydration, setPrevHydration] = useState<{
    category: typeof category
    locale: typeof locale
  } | null>(null)
  if (
    prevHydration === null ||
    prevHydration.category !== category ||
    prevHydration.locale !== locale
  ) {
    setPrevHydration({ category, locale })
    if (category) {
      // Resolve the content row for the locale this form edits, not whichever entry the server returned first in `content[0]` - Falling
      // back to `content[0]` keeps existing-category forms usable when no translation exists yet for `locale`
      const c =
        category.content.find((entry) => entry.locale === locale) ?? category.content[0]
      setName(c?.name ?? category.name ?? '')
      setSlug(category.slug)
      setSlugManual(true)
      setParentId(category.parent?.id ?? null)
      setPosition(category.position)
      setIsPublished(category.isPublished)
      setSeoNoindex(category.seoNoindex)
      // No row at all (last translation removed) - blank the fields rather
      // than leave the deleted copy on screen
      setShortDescription((c?.shortDescription as RichTextValue) ?? null)
      setDescription((c?.description as RichTextValue) ?? null)
      setMetaTitle(c?.metaTitle ?? '')
      setMetaDescription(c?.metaDescription ?? '')
      setReady(true)
    }
  }

  function handleNameChange(value: string) {
    setName(value)
    if (!slugManual) setSlug(slugify(value))
  }

  // The tree query takes no channel var - scope rides the request header -
  // so refetching it re-reads under the current scope automatically
  const refetchQueries = [{ query: RootCategoriesDocument }]
  const [createCategory] = useMutation(CreateCategoryDocument, { refetchQueries })
  const [setCategoryListing] = useMutation(SetCategoryListingDocument, { refetchQueries })
  const [updateCategory] = useMutation(UpdateCategoryDocument, { refetchQueries })
  const [upsertContent] = useMutation(UpsertCategoryContentDocument, {
    refetchQueries: categoryId
      ? [...refetchQueries, { query: CategoryDocument, variables: { id: categoryId } }]
      : refetchQueries,
  })
  const [deleteCategory] = useMutation(DeleteCategoryDocument, { refetchQueries })
  // Detail query only - rootCategories returns content: [], so pulling the
  // tree in here would blank the locale pills
  const [deleteContent] = useMutation(DeleteCategoryContentDocument, {
    refetchQueries: categoryId ? [{ query: CategoryDocument, variables: { id: categoryId } }] : [],
    awaitRefetchQueries: true,
  })
  const [addToCategory] = useMutation(AddProductToCategoryDocument, {
    refetchQueries: categoryId
      ? [{ query: CategoryDocument, variables: { id: categoryId } }]
      : [],
  })
  const [removeFromCategory] = useMutation(RemoveProductFromCategoryDocument, {
    refetchQueries: categoryId
      ? [{ query: CategoryDocument, variables: { id: categoryId } }]
      : [],
  })

  // ────────────────────────────────────────────────────────────────────
  // Save registry: each section registers its (diff, save) pair so the
  // global Save button reflects every pending change in one count, the
  // confirm modal lists them, and confirming runs them sequentially
  //
  // The "general" section below covers everything inside the main form
  // (name, slug, parent, position, visibility, descriptions, SEO meta)
  // Per-channel availability is a separate section registered from inside
  // CategoryAvailabilitySection
  // ────────────────────────────────────────────────────────────────────
  const [sections, setSections] = useState<Record<string, SectionRegistration>>({})
  const registerSection = useCallback((reg: SectionRegistration) => {
    setSections((prev) => ({ ...prev, [reg.id]: reg }))
  }, [])
  const unregisterSection = useCallback((id: string) => {
    setSections((prev) => {
      if (!(id in prev)) return prev
      const next = { ...prev }
      delete next[id]
      return next
    })
  }, [])
  const saveContextValue = useMemo<CategorySaveContextValue>(
    () => ({ register: registerSection, unregister: unregisterSection }),
    [registerSection, unregisterSection],
  )
  // dirtySections is computed below after generalDiff + generalSave have been declared (they depend on form state,
  // which is initialised after this block) - See "General section diff" further down
  const registeredDirty = useMemo(
    () => Object.values(sections).filter((s) => s.diff.length > 0),
    [sections],
  )
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveAllError, setSaveAllError] = useState<string | null>(null)

  const onSave = useCallback(
    async (e?: FormEvent) => {
      e?.preventDefault()
      if (!canManage) return
      setError(null)
      setSaving(true)
      try {
        if (isNew) {
          const result = await createCategory({
            variables: {
              input: {
                parentId,
                slug,
                position,
                isPublished,
                seoNoindex,
                locale,
                name,
                shortDescription: shortDescription ?? null,
                description: description ?? null,
                metaTitle: metaTitle || null,
                metaDescription: metaDescription || null,
              },
            },
          })
          const created = result.data?.createCategory
          // CHANNEL: immediately list the new category on the active store so the user sees it where they're working - Without this step
          // the category exists globally but is invisible to every channel until the operator manually adds a listing
          if (created && effectiveChannelId) {
            try {
              await setCategoryListing({
                variables: {
                  input: {
                    categoryId: created.id,
                    channelId: effectiveChannelId,
                    isPublished: true,
                    sortOrderOverride: null,
                  },
                },
              })
            } catch {
              // Non-fatal - the category was created - Operator can list manually
            }
          }
          if (created) onCreated?.(created.id)
        } else {
          await updateCategory({
            variables: {
              input: {
                id: categoryId!,
                parentId: parentId ?? undefined,
                clearParent: parentId === null,
                slug,
                position,
                isPublished,
                seoNoindex,
              },
            },
          })
          await upsertContent({
            variables: {
              categoryId: categoryId!,
              input: {
                locale,
                name,
                shortDescription: shortDescription ?? null,
                description: description ?? null,
                metaTitle: metaTitle || null,
                metaDescription: metaDescription || null,
              },
            },
          })
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Save failed.')
      } finally {
        setSaving(false)
      }
    },
    [
      canManage, isNew, parentId, slug, position, isPublished, seoNoindex, locale, name,
      shortDescription, description, metaTitle, metaDescription, categoryId,
      effectiveChannelId,
      createCategory, setCategoryListing, updateCategory, upsertContent, onCreated,
    ],
  )

  async function onDelete() {
    if (!categoryId) return
    setDeleting(true)
    setDeleteError(null)
    try {
      await deleteCategory({ variables: { id: categoryId } })
      setConfirmingDelete(false)
      onDeleted?.()
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    } finally {
      setDeleting(false)
    }
  }

  // ────────────────────────────────────────────────────────────────────
  // General section diff against the loaded category - empty when nothing changed, so the "Save changes (N)"
  // pill counts only real edits, and only for an existing category since create goes through the form's onSave
  // ────────────────────────────────────────────────────────────────────
  // Matches the locale-resolved content row that seeded the form state - comparing against `content[0]` flags a
  // fake Name change whenever the first entry's locale differs from the form's `locale`
  const originalContent = useMemo(
    () =>
      category?.content.find((entry) => entry.locale === locale) ??
      category?.content[0],
    [category, locale],
  )
  const generalDiff = useMemo<SectionDiffEntry[]>(() => {
    if (isNew || !category) return []
    const entries: SectionDiffEntry[] = []
    const push = (label: string, before: unknown, after: unknown) => {
      const b = formatScalar(before)
      const a = formatScalar(after)
      if (b !== a) entries.push({ label, before: b, after: a })
    }
    push('Name', originalContent?.name ?? category.name ?? '', name)
    push('Slug', category.slug, slug)
    push('Parent', category.parent?.id ?? null, parentId)
    push('Position', category.position, position)
    push('Published', category.isPublished, isPublished)
    push('Noindex', category.seoNoindex, seoNoindex)
    push('Short description', summarizeRichText(originalContent?.shortDescription ?? null), summarizeRichText(shortDescription))
    push('Description', summarizeRichText(originalContent?.description ?? null), summarizeRichText(description))
    push('Meta title', originalContent?.metaTitle ?? '', metaTitle)
    push('Meta description', originalContent?.metaDescription ?? '', metaDescription)
    return entries
  }, [
    isNew, category, originalContent,
    name, slug, parentId, position, isPublished, seoNoindex,
    shortDescription, description, metaTitle, metaDescription,
  ])

  // Save fn for the general section: writes all the structural + content fields together - Reuses the same mutations
  // the form's old onSave used, just without the create branch (the registry only fires on edit)
  const generalSave = useCallback(async () => {
    if (!categoryId) return
    await updateCategory({
      variables: {
        input: {
          id: categoryId,
          parentId: parentId ?? undefined,
          clearParent: parentId === null,
          slug,
          position,
          isPublished,
          seoNoindex,
        },
      },
    })
    await upsertContent({
      variables: {
        categoryId,
        input: {
          locale,
          name,
          shortDescription: shortDescription ?? null,
          description: description ?? null,
          metaTitle: metaTitle || null,
          metaDescription: metaDescription || null,
        },
      },
    })
  }, [
    categoryId, parentId, slug, position, isPublished, seoNoindex,
    locale, name, shortDescription, description, metaTitle, metaDescription,
    updateCategory, upsertContent,
  ])

  // The general section is owned by this component (not registered via the hook), so we splice it onto dirtySections from registered child
  // sections - Calling useCategorySaveSection here would no-op - the provider is in JSX below, so the host can't consume its own context
  const generalSection = useMemo<SectionRegistration | null>(
    () =>
      generalDiff.length
        ? { id: 'general', title: 'General', diff: generalDiff, save: generalSave }
        : null,
    [generalDiff, generalSave],
  )

  const dirtySections = useMemo(
    () => (generalSection ? [generalSection, ...registeredDirty] : registeredDirty),
    [generalSection, registeredDirty],
  )
  const isDirty = dirtySections.length > 0
  const totalChanges = dirtySections.reduce((n, s) => n + s.diff.length, 0)
  // Registered here rather than inside useSaveRegistry - this page's dirty set includes the general
  // section it owns itself, which the hook can't see
  useUnsavedChangesGuard(isDirty)

  // Confirm-modal commit - Runs each dirty section's save sequentially so
  // a later failure doesn't hide an earlier success
  async function onConfirmSave() {
    setSaveAllError(null)
    setSavingAll(true)
    try {
      await duringCommit(async () => {
        for (const section of dirtySections) {
          await section.save()
        }
      })
      setConfirmingSave(false)
    } catch (err) {
      setSaveAllError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  // Flatten all categories (excluding current) for parent dropdown
  const flatAll: Array<{ id: string; label: string; depth: number }> = []
  function flatten(cats: CategorySummary[], depth = 0) {
    for (const c of cats) {
      if (c.id === categoryId) continue
      flatAll.push({ id: c.id, label: resolveCategoryDisplayName(c), depth })
      flatten(c.children, depth + 1)
    }
  }
  flatten(roots)

  // Fallback: if the category's actual parent isn't in the tree (e.g. deeper than the rootCategories fragment fetches,
  // or roots is still loading), include it explicitly so the Select can render the current selection
  if (category?.parent && !flatAll.some((c) => c.id === category.parent!.id)) {
    flatAll.unshift({
      id: category.parent.id,
      label: category.parent.name ?? category.parent.slug,
      depth: 0,
    })
  }

  if (!isNew && (loading || !ready)) {
    return <LoadingState label="Loading category…" />
  }

  const headerTitle = isNew ? 'New category' : (name || category?.slug || 'Category')
  const headerUrl = !isNew ? (category?.urlPath ?? slug) : null
  // Create flow: uses the "submit the whole form" path because
  // there's nothing to diff against yet - every field is conceptually new
  // Edit flow: button opens the confirm-changes modal, which on confirm
  // runs every dirty section's save() in turn (general + per-store
  // availability + anything else registered)
  const canSubmit = canManage && !!name && !!slug && !saving && !savingAll
  const editSaveLabel = savingAll
    ? 'Saving…'
    : isDirty
      ? `Save changes${totalChanges > 0 ? ` (${totalChanges})` : ''}`
      : 'No changes'

  // Save context for child sections (e.g - CategoryAvailabilitySection) Surface scope mode, the active store name (when
  // in CHANNEL), and the locale that the write will land on so admins can sanity-check before confirming the save
  const scopeLabel = isGlobal
    ? `Global · ${locale || '(default locale)'}`
    : `${selectedChannel?.name ?? 'Selected store'} · ${locale || '(store default locale)'}`

  return (
    <CategorySaveContext.Provider value={saveContextValue}>
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (isNew) {
          // Direct submit - no diff to confirm against on create
          onSave(e)
        } else if (isDirty) {
          setSaveAllError(null)
          setConfirmingSave(true)
        }
      }}
      className="flex flex-col"
    >
      {/* ── Sticky action header ────────────────────────────── */}
      <div
        ref={headerRef}
        className={cn(
          'sticky top-0 z-10 border-b bg-background/95 backdrop-blur transition-transform duration-200 supports-[backdrop-filter]:bg-background/80',
          headerHidden && '-translate-y-full',
        )}
      >
        <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-lg font-semibold tracking-tight">{headerTitle}</h1>
            {headerUrl ? (
              <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                /{headerUrl}
              </p>
            ) : null}
          </div>
          {canManage ? (
            <div className="flex shrink-0 items-center gap-2">
              <label
                htmlFor="cat-published-header"
                className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1 text-sm"
              >
                <Switch
                  id="cat-published-header"
                  checked={isPublished}
                  onCheckedChange={setIsPublished}
                />
                <span className={isPublished ? 'font-medium' : 'text-muted-foreground'}>
                  {isPublished ? 'Published' : 'Draft'}
                </span>
              </label>
              {!isNew ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => { setDeleteError(null); setConfirmingDelete(true) }}
                  disabled={deleting}
                >
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
              ) : null}
              {isNew ? (
                <Button type="submit" size="sm" disabled={!canSubmit}>
                  {saving ? 'Saving…' : 'Create'}
                </Button>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  disabled={!canSubmit || !isDirty}
                  onClick={() => { setSaveAllError(null); setConfirmingSave(true) }}
                >
                  {editSaveLabel}
                </Button>
              )}
            </div>
          ) : null}
        </div>
      </div>

      {/* ── Body ────────────────────────────────────────────── */}
      <div className="divide-y pb-16">
        {error ? (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        ) : null}

        {/* ── General ── */}
        <Section title="General" description="Basic info, hierarchy, and visibility.">
          <div className="grid gap-4 md:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="cat-name">Name <span className="text-destructive">*</span></Label>
                <Input
                  id="cat-name"
                  value={name}
                  onChange={(e) => handleNameChange(e.target.value)}
                  disabled={!canManage}
                  required
                  placeholder="Women's Shoes"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="cat-slug">Slug <span className="text-destructive">*</span></Label>
                <Input
                  id="cat-slug"
                  value={slug}
                  onChange={(e) => { setSlugManual(true); setSlug(e.target.value) }}
                  disabled={!canManage}
                  required
                  placeholder="womens-shoes"
                  className="font-mono"
                />
              </div>
            </div>

            <div className="grid gap-4 md:grid-cols-[1fr_8rem]">
              <div className="grid gap-2">
                <Label htmlFor="cat-parent">Parent category</Label>
                <Select
                  value={parentId ?? '__root__'}
                  onValueChange={(v) => setParentId(v === '__root__' ? null : v)}
                  disabled={!canManage}
                >
                  <SelectTrigger className="w-full" id="cat-parent">
                    <SelectValue placeholder="None (root)" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__root__">None (root)</SelectItem>
                    {flatAll.map((c) => (
                      <SelectItem key={c.id} value={c.id}>
                        {'  '.repeat(c.depth)}{c.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="cat-position">Position</Label>
                <Input
                  id="cat-position"
                  type="number"
                  value={position}
                  onChange={(e) => setPosition(Number(e.target.value))}
                  disabled={!canManage}
                />
              </div>
            </div>

            {/* Name, descriptions and SEO above are all per-locale - this is
                where a locale's copy gets dropped again - Immediate action,
                like the Delete button in the header */}
            {!isNew && category ? (
              <TranslationLocales
                locale={locale}
                locales={category.content.map((entry) => entry.locale)}
                entityLabel="category"
                canManage={canManage}
                defaultLocaleLocked
                onRemove={async () => {
                  await deleteContent({ variables: { categoryId: category.id, locale } })
                }}
              />
            ) : null}

        </Section>

        {/* ── Short description ── */}
        <Section title="Short description" description="Brief summary shown in listings.">
          <div className="overflow-hidden rounded-md border">
            <LexicalRichTextEditor
              key={`short-${categoryId ?? 'new'}-${locale}-${originalContent?.updatedAt ?? 'none'}`}
              value={shortDescription}
              fallbackText=""
              disabled={!canManage}
              placeholder="Brief category summary shown in listings…"
              onChange={(val) => setShortDescription(val)}
            />
          </div>
        </Section>

        {/* ── Description ── */}
        <Section title="Description" description="Full description rendered on the category page.">
          <div className="overflow-hidden rounded-md border">
            <LexicalRichTextEditor
              key={`desc-${categoryId ?? 'new'}-${locale}-${originalContent?.updatedAt ?? 'none'}`}
              value={description}
              fallbackText=""
              disabled={!canManage}
              placeholder="Full category description…"
              onChange={(val) => setDescription(val)}
            />
          </div>
        </Section>

        {/* ── Banner image ── */}
        {!isNew && category ? (
          <CategoryCoverSection category={category} canManage={canManage} />
        ) : null}

        {/* ── SEO ── */}
        <Section title="SEO" description="Search metadata and indexing controls.">
          <div className="grid gap-2">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="cat-meta-title">Meta title</Label>
              <span className={`text-xs tabular-nums ${metaTitle.length > 60 ? 'text-destructive' : 'text-muted-foreground'}`}>
                {metaTitle.length}/60
              </span>
            </div>
            <Input
              id="cat-meta-title"
              value={metaTitle}
              onChange={(e) => setMetaTitle(e.target.value)}
              disabled={!canManage}
              placeholder={name || 'Category name'}
            />
          </div>
          <div className="grid gap-2">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="cat-meta-desc">Meta description</Label>
              <span className={`text-xs tabular-nums ${metaDescription.length > 160 ? 'text-destructive' : 'text-muted-foreground'}`}>
                {metaDescription.length}/160
              </span>
            </div>
            <Textarea
              id="cat-meta-desc"
              value={metaDescription}
              onChange={(e) => setMetaDescription(e.target.value)}
              disabled={!canManage}
              rows={3}
              placeholder="Search result description…"
            />
          </div>
          <ToggleRow
            id="cat-noindex"
            label="Noindex"
            description="Ask search engines not to index this category."
            checked={seoNoindex}
            onCheckedChange={setSeoNoindex}
            disabled={!canManage}
          />
        </Section>

        {/* ── Per-channel availability ── */}
        {!isNew && category ? (
          <CategoryAvailabilitySection category={category} canManage={canManage} />
        ) : null}

        {/* ── Storefront filters ── */}
        {!isNew && category ? (
          <CategoryFacetOverridesSection category={category} canManage={canManage} />
        ) : null}

        {/* ── Assigned products ── */}
        {!isNew && category ? (
          <AssignedProductsCard
            categoryId={category.id}
            products={category.products ?? []}
            canManage={canManage}
            onAdd={(productId) => addToCategory({ variables: { productId, categoryId: category.id } })}
            onRemove={(productId) => removeFromCategory({ variables: { productId, categoryId: category.id } })}
          />
        ) : null}
      </div>

      {/* ── Delete confirmation ─────────────────────────────── */}
      <AlertDialog
        open={confirmingDelete}
        onOpenChange={(v) => { if (!v) { setConfirmingDelete(false); setDeleteError(null) } }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {name || category?.slug || 'this category'}?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the category. Child categories will be detached from it.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError ? (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => { e.preventDefault(); onDelete() }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete category'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={scopeLabel}
        sections={dirtySections}
        saving={savingAll}
        error={saveAllError}
        onCancel={() => { setConfirmingSave(false); setSaveAllError(null) }}
        onConfirm={onConfirmSave}
      />
    </form>
    </CategorySaveContext.Provider>
  )
}

// ─── assigned products card ───────────────────────────────────────────────────

type ProductItem = { id: string; name?: string | null; sku?: string | null }

function AssignedProductsCard({
  products,
  canManage,
  onAdd,
  onRemove,
}: {
  categoryId: string
  products: ProductItem[]
  canManage: boolean
  onAdd: (productId: string) => Promise<unknown>
  onRemove: (productId: string) => Promise<unknown>
}) {
  const { data: productsData } = useQuery(ProductsDocument, { variables: {} })
  const allProducts = productsData?.products ?? []
  const assignedIds = new Set(products.map((p) => p.id))
  const available = allProducts.filter((p) => !assignedIds.has(p.id))

  const [search, setSearch] = useState('')
  const [adding, setAdding] = useState(false)
  const [selectedProductId, setSelectedProductId] = useState('')

  const filtered = available.filter((p) => {
    const q = search.toLowerCase()
    return (p.name ?? '').toLowerCase().includes(q) || (p.sku ?? '').toLowerCase().includes(q)
  })

  async function handleAdd() {
    if (!selectedProductId) return
    setAdding(true)
    try {
      await onAdd(selectedProductId)
      setSelectedProductId('')
      setSearch('')
    } finally {
      setAdding(false)
    }
  }

  return (
    <Section title={`Products (${products.length})`} description="Products assigned to this category.">
      {products.length === 0 ? (
        <p className="text-xs text-muted-foreground">No products assigned to this category.</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {products.map((p) => (
            <li key={p.id} className="flex items-center justify-between px-3 py-2 text-sm">
              <div className="min-w-0">
                <p className="truncate font-medium">{p.name ?? p.sku ?? p.id}</p>
                {p.sku ? <p className="text-xs text-muted-foreground">{p.sku}</p> : null}
              </div>
              {canManage ? (
                <button
                  type="button"
                  className="ml-2 shrink-0 text-muted-foreground hover:text-destructive"
                  onClick={() => onRemove(p.id)}
                >
                  <XIcon className="size-4" />
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {canManage ? (
        <div className="grid gap-2">
          <Label className="text-xs">Add product</Label>
          <Input
            placeholder="Search products…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-8 text-xs"
          />
          {search && filtered.length > 0 ? (
            <ul className="scrollbar-accent max-h-40 overflow-y-auto rounded-md border text-xs">
              {filtered.map((p) => (
                <li key={p.id}>
                  <button
                    type="button"
                    className={`flex w-full items-center justify-between px-3 py-2 hover:bg-muted ${selectedProductId === p.id ? 'bg-muted font-medium' : ''}`}
                    onClick={() => setSelectedProductId(p.id)}
                  >
                    <span>{p.name ?? p.sku ?? ''}</span>
                    {p.sku ? <span className="text-muted-foreground">{p.sku}</span> : null}
                  </button>
                </li>
              ))}
            </ul>
          ) : null}
          {selectedProductId ? (
            <Button type="button" size="sm" disabled={adding} onClick={handleAdd}>
              {adding ? 'Adding…' : 'Add selected product'}
            </Button>
          ) : null}
        </div>
      ) : null}
    </Section>
  )
}

// Banner edits stage like every other section: picking a file or hitting "Clear banner" only marks the change locally;
// the signed-URL upload (or clear mutation) runs from save() when the page-level Save is confirmed
function CategoryCoverSection({
  category,
  canManage,
}: {
  category: Pick<CategoryDetail, 'id' | 'name' | 'coverImage'>
  canManage: boolean
}) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [altText, setAltText] = useState(category.coverImage?.altText ?? category.name ?? '')
  const [pendingFile, setPendingFile] = useState<File | null>(null)
  const [pendingClear, setPendingClear] = useState(false)
  const [fileError, setFileError] = useState<string | null>(null)
  const refetchQueries = [{ query: CategoryDocument, variables: { id: category.id } }]
  const [requestUpload] = useMutation(RequestCategoryCoverUploadDocument)
  const [confirmUpload] = useMutation(ConfirmCategoryCoverUploadDocument, { refetchQueries })
  const [clearCover] = useMutation(ClearCategoryCoverDocument, { refetchQueries })

  const previewUrl = useMemo(
    () => (pendingFile ? URL.createObjectURL(pendingFile) : null),
    [pendingFile],
  )
  useEffect(() => () => { if (previewUrl) URL.revokeObjectURL(previewUrl) }, [previewUrl])

  const currentLabel = category.coverImage
    ? category.coverImage.url.split('/').pop() || 'current banner'
    : ''

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    if (pendingFile) {
      out.push({ label: 'Banner image', before: currentLabel, after: pendingFile.name })
      const baseAlt = category.coverImage?.altText ?? ''
      if (altText.trim() !== baseAlt) {
        out.push({ label: 'Banner alt text', before: baseAlt, after: altText.trim() })
      }
    } else if (pendingClear) {
      out.push({ label: 'Banner image', before: currentLabel, after: '' })
    }
    return out
  }, [pendingFile, pendingClear, currentLabel, altText, category.coverImage?.altText])

  const save = useCallback(async () => {
    if (pendingClear) {
      await clearCover({ variables: { categoryId: category.id } })
      setPendingClear(false)
      return
    }
    if (!pendingFile) return
    const file = pendingFile
    const [{ width, height }, upload] = await Promise.all([
      readImageDimensions(file),
      requestUpload({
        variables: {
          input: {
            categoryId: category.id,
            filename: file.name,
            mimeType: file.type,
            sizeBytes: file.size,
            altText: altText.trim() || undefined,
          },
        },
      }),
    ])
    const payload = upload.data?.requestCategoryCoverUpload
    if (!payload) throw new Error('Upload request did not return a signed URL.')
    const uploaded = await fetch(payload.uploadUrl, {
      method: 'PUT',
      headers: { 'Content-Type': file.type },
      body: file,
    })
    if (!uploaded.ok) throw new Error(`Banner upload failed with HTTP ${uploaded.status}.`)
    await confirmUpload({ variables: { categoryId: category.id, width, height } })
    setPendingFile(null)
  }, [pendingClear, pendingFile, altText, category.id, clearCover, requestUpload, confirmUpload])

  useCategorySaveSection({ id: 'cover', title: 'Banner image', diff, save })

  function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file) return
    if (!file.type.startsWith('image/')) {
      setFileError('Only image files can be uploaded here.')
      return
    }
    setFileError(null)
    setPendingClear(false)
    setPendingFile(file)
  }

  function discardStaged() {
    setPendingFile(null)
    setPendingClear(false)
    setFileError(null)
  }

  const displayedImage = previewUrl ?? (pendingClear ? null : category.coverImage?.url ?? null)
  const displayedAlt = previewUrl ? altText : category.coverImage?.altText ?? ''

  return (
    <Section
      title="Banner image"
      description="Hero image shown at the top of the category page. A staged image uploads when you save the page."
    >
      {displayedImage ? (
        <div className="overflow-hidden rounded-md border bg-muted/20">
          <img src={displayedImage} alt={displayedAlt} className="max-h-56 w-full object-cover" />
        </div>
      ) : (
        <div className="flex h-40 w-full items-center justify-center rounded-md border border-dashed bg-muted/30 text-sm text-muted-foreground">
          <div className="flex items-center gap-2">
            <ImageIcon className="size-4" />
            {pendingClear ? 'Banner removal staged' : 'No banner image'}
          </div>
        </div>
      )}

      {pendingFile ? (
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Badge variant="outline">Staged</Badge>
          <span>{pendingFile.name}</span>
        </div>
      ) : category.coverImage && !pendingClear ? (
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Badge variant="outline">{category.coverImage.status}</Badge>
          <span>{category.coverImage.width}×{category.coverImage.height}</span>
          <span>·</span>
          <span>{category.coverImage.mimeType}</span>
        </div>
      ) : null}

      <div className="grid gap-2">
        <Label htmlFor="cat-cover-alt">Alt text</Label>
        <Input
          id="cat-cover-alt"
          value={altText}
          onChange={(event) => setAltText(event.target.value)}
          disabled={!canManage}
        />
        <p className="text-xs text-muted-foreground">
          Alt text is saved when a new banner image is uploaded.
        </p>
      </div>

      {fileError && (
        <Alert variant="destructive">
          <AlertTitle>Banner image</AlertTitle>
          <AlertDescription>{fileError}</AlertDescription>
        </Alert>
      )}

      {canManage && (
        <div className="flex flex-wrap justify-end gap-2">
          {(pendingFile || pendingClear) && (
            <Button type="button" variant="ghost" onClick={discardStaged}>
              Discard staged change
            </Button>
          )}
          {category.coverImage && !pendingClear && (
            <Button
              type="button"
              variant="outline"
              onClick={() => { setPendingFile(null); setPendingClear(true) }}
            >
              <XIcon className="size-4" />
              Clear banner
            </Button>
          )}
          <Button type="button" onClick={() => fileInputRef.current?.click()}>
            <ImageIcon className="size-4" />
            {pendingFile ? 'Choose different image' : category.coverImage ? 'Replace banner' : 'Upload banner'}
          </Button>
        </div>
      )}

      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={onFileChange}
      />
    </Section>
  )
}

type AvailDraft = {
  channelId: string
  name: string
  serverListing: { isPublished: boolean; sortOrderOverride: number | null } | null
  isPublished: boolean
  sortOverrideText: string
  pendingRemove: boolean
}

/**
 * CategoryFacetOverridesSection - per-category storefront filter presentation,
 * rendered by the shared drag-and-drop FacetOverridesEditor. Saving writes the FULL
 * list as overrides (list order = storefront order); reset saves an empty set,
 * which falls back to attribute sortOrder.
 */
function CategoryFacetOverridesSection({
  category,
  canManage,
}: {
  category: { id: string; facetOverrides: CategoryFacetOverride[] }
  canManage: boolean
}) {
  const { data: attrData } = useQuery(AttributesDocument)
  const [setOverrides] = useMutation(SetCategoryFacetOverridesDocument, {
    refetchQueries: [{ query: CategoryDocument, variables: { id: category.id } }],
  })

  // Facet candidates = what the storefront can actually render as a filter panel
  const candidates = useMemo(
    () =>
      sortFacetCandidates(
        (attrData?.attributes ?? []).filter((a) => a.filterable && a.isPublic),
      ),
    [attrData],
  )

  const draft = useFacetOverridesDraft(category.facetOverrides, candidates)

  const saveSection = useCallback(async () => {
    await setOverrides({
      variables: { categoryId: category.id, overrides: draft.payload },
    })
  }, [setOverrides, category.id, draft.payload])

  useCategorySaveSection({
    id: 'category-facet-overrides',
    title: 'Storefront filters',
    diff: draft.sectionDiff,
    save: saveSection,
  })

  if (candidates.length === 0) {
    return (
      <Section
        title="Storefront filters"
        description="Order and visibility of this category's filter sidebar."
      >
        <p className="text-sm text-muted-foreground">
          No filterable attributes yet. Mark attributes as filterable to build the sidebar.
        </p>
      </Section>
    )
  }

  return (
    <Section
      title="Storefront filters"
      description="Order and visibility of the filter sidebar on this category's page. Hidden filters stay filterable through direct links."
    >
      <FacetOverridesEditor
        rows={draft.rows}
        customized={draft.customized}
        canManage={canManage}
        onReorder={draft.reorder}
        onToggleHidden={draft.toggleHidden}
        onReset={draft.reset}
      />
    </Section>
  )
}

/**
 * CategoryAvailabilitySection lets the operator attach/detach a category
 * from channels
 *
 * Two modes, both header-driven:
 *  - GLOBAL: full grid, one row per channel, save-all diffs at once
 *    This is the entry point for "publish on EU too" since EU's tree
 *    hides categories that aren't yet listed there
 *  - CHANNEL: collapses to a single-row editor for the active store
 *    so editing what you currently see is a one-click action
 */
function CategoryAvailabilitySection({
  category,
  canManage,
}: {
  category: {
    id: string
    listings: {
      channelId: string
      channel: { id: string; name: string }
      isPublished: boolean
      sortOrderOverride?: number | null
    }[]
  }
  canManage: boolean
}) {
  const { isChannel, effectiveChannelId, selectedChannel, channels } = useConfigScope()
  const refetch = [{ query: CategoryDocument, variables: { id: category.id } }]
  const [setListing] = useMutation(SetCategoryListingDocument, { refetchQueries: refetch })
  const [clearListing] = useMutation(ClearCategoryListingDocument, { refetchQueries: refetch })

  const buildInitial = useCallback((): AvailDraft[] => {
    const byStore = new Map(category.listings.map((l) => [l.channelId, l]))
    return channels.map((sv) => {
      const existing = byStore.get(sv.id)
      return {
        channelId: sv.id,
        name: sv.name,
        serverListing: existing
          ? {
              isPublished: existing.isPublished,
              sortOrderOverride: existing.sortOrderOverride ?? null,
            }
          : null,
        isPublished: existing?.isPublished ?? false,
        sortOverrideText:
          existing?.sortOrderOverride != null ? String(existing.sortOrderOverride) : '',
        pendingRemove: false,
      }
    })
  }, [category.listings, channels])

  const [drafts, setDrafts] = useState<AvailDraft[]>(() => buildInitial())
  // Rebuild the drafts when the listings/channels source data changes
  // (buildInitial is memoized on exactly those inputs)
  const [prevBuildInitial, setPrevBuildInitial] = useState(() => buildInitial)
  if (prevBuildInitial !== buildInitial) {
    setPrevBuildInitial(() => buildInitial)
    setDrafts(buildInitial())
  }

  function patch(channelId: string, partial: Partial<AvailDraft>) {
    setDrafts((prev) =>
      prev.map((d) =>
        d.channelId === channelId ? { ...d, ...partial, pendingRemove: false } : d,
      ),
    )
  }

  function markRemove(channelId: string) {
    setDrafts((prev) =>
      prev.map((d) =>
        d.channelId === channelId
          ? { ...d, pendingRemove: !d.pendingRemove, isPublished: false }
          : d,
      ),
    )
  }

  function dirtyOp(d: AvailDraft): 'create' | 'update' | 'remove' | null {
    const parsed = d.sortOverrideText.trim() === '' ? null : Number(d.sortOverrideText)
    if (d.pendingRemove) return d.serverListing ? 'remove' : null
    if (!d.serverListing) {
      if (d.isPublished || parsed != null) return 'create'
      return null
    }
    const sortChanged = (parsed ?? null) !== (d.serverListing.sortOrderOverride ?? null)
    if (d.isPublished !== d.serverListing.isPublished || sortChanged) return 'update'
    return null
  }

  const dirtyCount = drafts.filter((d) => dirtyOp(d) != null).length
  const anyInvalid = drafts.some((d) => {
    if (d.sortOverrideText.trim() === '') return false
    return !Number.isFinite(Number(d.sortOverrideText))
  })

  // ────────────────────────────────────────────────────────────────────
  // Register with the parent CategoryForm save registry - Each pending
  // channel change becomes a row in the confirm modal, and saveSection
  // commits everything in parallel when the operator hits Save changes
  // The card carries no Save All button of its own - the global save
  // is the only way to commit
  // ────────────────────────────────────────────────────────────────────
  const saveSection = useCallback(async () => {
    if (anyInvalid) throw new Error('Sort override is not a number')
    const ops: Promise<unknown>[] = []
    for (const d of drafts) {
      const op = dirtyOp(d)
      if (op == null) continue
      if (op === 'remove') {
        ops.push(
          clearListing({
            variables: { categoryId: category.id, channelId: d.channelId },
          }),
        )
      } else {
        const sortOverride =
          d.sortOverrideText.trim() === '' ? null : Number(d.sortOverrideText)
        ops.push(
          setListing({
            variables: {
              input: {
                categoryId: category.id,
                channelId: d.channelId,
                isPublished: d.isPublished,
                sortOrderOverride: sortOverride,
              },
            },
          }),
        )
      }
    }
    await Promise.all(ops)
  }, [drafts, anyInvalid, clearListing, setListing, category.id])

  const sectionDiff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    for (const d of drafts) {
      const op = dirtyOp(d)
      if (op == null) continue
      const before = d.serverListing
        ? `${d.serverListing.isPublished ? 'published' : 'unpublished'}${
            d.serverListing.sortOrderOverride != null
              ? ` · sort ${d.serverListing.sortOrderOverride}`
              : ''
          }`
        : 'not listed'
      const after =
        op === 'remove'
          ? 'remove listing'
          : `${d.isPublished ? 'published' : 'unpublished'}${
              d.sortOverrideText.trim() ? ` · sort ${d.sortOverrideText}` : ''
            }`
      out.push({ label: d.name, before, after })
    }
    return out
  }, [drafts])

  useCategorySaveSection({
    id: 'category-availability',
    title: 'Per-channel availability',
    diff: sectionDiff,
    save: saveSection,
  })

  if (channels.length === 0) {
    return (
      <Section title="Per-channel availability" description="Choose which channels carry this category.">
        <p className="text-sm text-muted-foreground">No channels are configured yet.</p>
      </Section>
    )
  }

  // CHANNEL: collapse to a single-row editor for the active store
  if (isChannel) {
    const draft = drafts.find((d) => d.channelId === effectiveChannelId)
    if (!draft) return null
    const op = dirtyOp(draft)
    return (
      <Section
        title={`Availability on ${selectedChannel?.name ?? 'this store'}`}
        description="Switch the header store to manage another. Use the global tree (header → Global default) for cross-store assignment."
      >
        <div className="grid gap-3 rounded-md border p-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="cat-store-published">Published on store</Label>
              <p className="text-xs text-muted-foreground">
                Required (along with the global Published flag) for the category to show up.
              </p>
            </div>
            <Switch
              id="cat-store-published"
              checked={draft.isPublished && !draft.pendingRemove}
              onCheckedChange={(v) => patch(draft.channelId, { isPublished: v })}
              disabled={!canManage || draft.pendingRemove}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="cat-sort-override">Sort override</Label>
            <Input
              id="cat-sort-override"
              type="number"
              inputMode="numeric"
              value={draft.sortOverrideText}
              onChange={(e) => patch(draft.channelId, { sortOverrideText: e.target.value })}
              placeholder="(inherit global Position)"
              disabled={!canManage || draft.pendingRemove}
            />
          </div>
          {canManage && draft.serverListing ? (
            <div className="flex flex-wrap items-center gap-2">
              {/* Mark-for-removal toggle - The actual save happens in the
                  global "Save changes" modal; this just stages the intent */}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => markRemove(draft.channelId)}
                disabled={!canManage}
              >
                {draft.pendingRemove ? 'Undo remove' : 'Remove listing'}
              </Button>
              {op != null ? (
                <span className="text-xs text-amber-600">
                  Pending: {op === 'create' ? 'add' : op === 'remove' ? 'remove' : 'update'}.
                  Save with the header button.
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
      </Section>
    )
  }

  // GLOBAL: full multi-store grid
  return (
    <Section
      title="Per-channel availability"
      description="Toggle which channels carry this category. A category is hidden on a store with no listing row."
    >
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">Channel</th>
              <th className="px-3 py-2 font-medium">Published</th>
              <th className="px-3 py-2 font-medium">Sort override</th>
              <th className="px-3 py-2 font-medium text-right">Status</th>
            </tr>
          </thead>
          <tbody>
            {drafts.map((d) => {
              const op = dirtyOp(d)
              return (
                <tr key={d.channelId} className="border-t">
                  <td className="px-3 py-2">
                    <div className="font-medium">{d.name}</div>
                    {d.serverListing == null ? (
                      <div className="text-xs text-muted-foreground">not listed</div>
                    ) : null}
                  </td>
                  <td className="px-3 py-2">
                    <Switch
                      checked={d.isPublished && !d.pendingRemove}
                      onCheckedChange={(v) => patch(d.channelId, { isPublished: v })}
                      disabled={!canManage || d.pendingRemove}
                    />
                  </td>
                  <td className="px-3 py-2">
                    <Input
                      value={d.sortOverrideText}
                      onChange={(e) => patch(d.channelId, { sortOverrideText: e.target.value })}
                      placeholder="(inherit)"
                      type="number"
                      inputMode="numeric"
                      className="h-8 w-24"
                      disabled={!canManage || d.pendingRemove}
                    />
                  </td>
                  <td className="px-3 py-2 text-right">
                    {op === 'create' ? (
                      <span className="text-xs text-amber-600">will create</span>
                    ) : op === 'update' ? (
                      <span className="text-xs text-amber-600">modified</span>
                    ) : op === 'remove' ? (
                      <span className="text-xs text-destructive">will remove</span>
                    ) : (
                      <span className="text-xs text-muted-foreground">unchanged</span>
                    )}
                    {d.serverListing ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="ml-2 h-7 px-2 text-xs"
                        onClick={() => markRemove(d.channelId)}
                        disabled={!canManage}
                      >
                        {d.pendingRemove ? 'Undo' : 'Remove'}
                      </Button>
                    ) : null}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {dirtyCount > 0 ? (
        <p className="text-xs text-amber-600">
          {dirtyCount} pending change{dirtyCount === 1 ? '' : 's'}. Save with the
          header button - the confirm modal lists every channel change before it
          commits.
        </p>
      ) : null}
    </Section>
  )
}
