import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
} from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { toast } from 'sonner'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragMoveEvent,
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
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FieldError } from '@/components/form/fields'
import { fieldErrorText } from '@/components/form/field-errors'
import { Switch } from '@/components/ui/switch'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope, type ChannelScopeOption } from '@/lib/config-scope'
import { cn } from '@/lib/utils'
import { useScrollHideHeader } from '@/lib/use-scroll-hide-header'
import { FormRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'
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
  type CategoryDetailFieldsFragment,
} from './Categories.gql'
import { SetCategoryFacetOverridesDocument } from './Categories.gql'
import { dropZone, isDescendant, type CategorySummary, type DropZone } from './category-tree'
import { CategoryOptionsDocument } from './category-picker/CategoryOptions.gql'
import { formatPath, subtreeOf, useCategoryIndex } from './category-picker/category-options'
import { CategorySelect } from './category-picker/CategorySelect'
import { CategoryProductsSection } from './CategoryProductsSection'
import { Section } from './CategorySection'
import { AttributesDocument } from './Catalog.gql'
import { FacetOverridesEditor } from './FacetOverridesEditor'
import { sortFacetCandidates, useFacetOverridesDraft } from './facet-overrides-draft'
import { readImageDimensions } from '@/lib/image'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { SaveRegistryContext, useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { useStore } from '@tanstack/react-form'
import {
  CATEGORY_LABELS,
  categoryChanges,
  categoryDiff,
  categorySchema,
  emptyCategory,
  toCategoryCreateInput,
  toCategoryValues,
} from './schemas/category'
import { slugify } from '@/lib/slug'
import { putToSignedUrl } from '@/lib/upload'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { onTreeItemKeyDown } from '@/lib/tree-keys'

type CategoryDetail = CategoryDetailFieldsFragment
type CategoryFacetOverride = CategoryDetail['facetOverrides'][number]

// ─── helpers ─────────────────────────────────────────────────────────────────

function countCategories(cats: CategorySummary[]): number {
  let n = 0
  for (const c of cats) {
    n += 1 + countCategories(c.children ?? [])
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
    const found = findCategoryAncestors(c.children ?? [], target, [...path, c.id])
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
      hasChildren: !!cat.children?.length,
      expanded,
    })
    if (expanded && cat.children?.length) {
      result.push(...flattenTree(cat.children ?? [], expandedIds, depth + 1, cat.id))
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
  // tree - creating from CHANNEL also lists the new category on that store, so both scopes can create
  const { isGlobal, selectedChannel } = useConfigScope()
  const storeName = selectedChannel?.name ?? 'this store'
  // Keep the tree query locale-agnostic: Apollo normalises Category by id, so passing different locales for one
  // id would overwrite the cached `name` across views - the server falls back through locales anyway, so the
  // tree always shows a real name when one exists in any translation
  // Channel scope rides the `X-Channel-Id` header (see lib/apollo.ts), not a
  // query var: CHANNEL scope returns every category listed on that store,
  // published or not, GLOBAL the full tree
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
  if (!didAutoExpandRoot && roots.length === 1 && roots[0].children?.length) {
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
        const child = walk(c.children ?? [], [...path, c.id])
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
    refetchQueries: [{ query: RootCategoriesDocument }, { query: CategoryOptionsDocument }],
  })

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }))

  // ── drag state ──
  // Structure is global-only, and a filtered tree hides the siblings a drop would reorder against
  const canDrag = canManage && isGlobal && !search.trim()
  const [dropTarget, setDropTarget] = useState<{ id: string; zone: DropZone } | null>(null)

  function onDragStart({ active }: DragStartEvent) {
    setDragActiveId(active.id as string)
  }

  function onDragMove({ active, over, activatorEvent, delta }: DragMoveEvent) {
    // Nothing droppable under the pointer, or a drop that would nest a category inside itself
    if (!over || isDescendant(roots, String(active.id), String(over.id))) {
      if (dropTarget) setDropTarget(null)
      return
    }
    const startY = activatorEvent instanceof PointerEvent || activatorEvent instanceof MouseEvent ? activatorEvent.clientY : 0
    const zone = dropZone(startY + delta.y, over.rect.top, over.rect.height)
    if (dropTarget?.id !== over.id || dropTarget.zone !== zone) setDropTarget({ id: String(over.id), zone })
  }

  async function onDragEnd({ active, over }: DragEndEvent) {
    setDragActiveId(null)
    setDropTarget(null)
    if (!over || !dropTarget || isDescendant(roots, String(active.id), dropTarget.id)) return

    const draggedNode = flat.find((n) => n.id === active.id)
    const targetNode = flat.find((n) => n.id === dropTarget.id)
    if (!draggedNode || !targetNode) return

    try {
      if (dropTarget.zone === 'on') {
        await updateCategory({
          variables: { input: { id: draggedNode.id, parentId: targetNode.id } },
        })
        // Open the new parent so the moved category is visible where it landed
        setExpandedIds((prev) => new Set([...prev, targetNode.id]))
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
    } catch (err) {
      // The tree stays as it was - say so, rather than leaving the admin wondering why nothing moved
      toast.error(`Could not move ${draggedNode.name}`, {
        description: err instanceof Error ? err.message : undefined,
      })
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
          {!isGlobal ? (
            <p className="text-xs text-muted-foreground">
              Showing categories added to {storeName}. Switch the header to Global default to see every
              category.
            </p>
          ) : null}
        </div>

        {loading ? (
          <p className="px-4 py-3 text-xs text-muted-foreground">Loading…</p>
        ) : roots.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 py-12 text-center">
            <FolderIcon className="size-6 text-muted-foreground/60" />
            <p className="text-sm font-medium">
              {isGlobal ? 'No categories yet' : `No categories on ${storeName}`}
            </p>
            {!isGlobal ? (
              <p className="text-xs text-muted-foreground">
                Categories that exist but aren&rsquo;t added to this store only show under Global default.
              </p>
            ) : null}
            {canManage ? (
              <Button size="sm" variant="outline" onClick={() => navigate('/categories/new')}>
                <PlusIcon className="size-3.5" /> {isGlobal ? 'Create your first' : 'New category'}
              </Button>
            ) : null}
          </div>
        ) : (
          <DndContext
            sensors={sensors}
            onDragStart={onDragStart}
            onDragMove={onDragMove}
            onDragEnd={onDragEnd}
            onDragCancel={() => { setDragActiveId(null); setDropTarget(null) }}
          >
            <div role="tree" aria-label="Category tree" className="scrollbar-accent flex-1 overflow-y-auto py-1">
              {visibleFlat.length === 0 ? (
                <p className="px-4 py-6 text-center text-xs text-muted-foreground">
                  No matches for &ldquo;{search}&rdquo;
                </p>
              ) : (
                visibleFlat.map((node) => (
                  <TreeNode
                    key={node.id}
                    node={node}
                    // One tab stop for the whole tree - the selected row when it's showing, else the first; arrows move within
                    tabbable={node.id === (visibleFlat.some((n) => n.id === selectedId) ? selectedId : visibleFlat[0]?.id)}
                    isSelected={node.id === selectedId}
                    dropTarget={dropTarget}
                    onSelect={() => navigate(`/categories/${node.id}`)}
                    onToggle={() => toggleExpand(node.id)}
                    canDrag={canDrag}
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
          canManage={canManage}
          onCreated={(id) => navigate(`/categories/${id}`)}
        />
      ) : selectedId ? (
        <CategoryForm
          key={selectedId}
          categoryId={selectedId}
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

// ─── tree node ───────────────────────────────────────────────────────────────

function TreeNode({
  node,
  tabbable,
  isSelected,
  dropTarget,
  onSelect,
  onToggle,
  canDrag,
}: {
  node: FlatNode
  tabbable: boolean
  isSelected: boolean
  dropTarget: { id: string; zone: DropZone } | null
  onSelect: () => void
  onToggle: () => void
  canDrag: boolean
}) {
  // The handle drags, the whole row is the drop target - the zone within it comes from the page's onDragMove
  const { attributes, listeners, setNodeRef: setDragRef, isDragging } = useDraggable({ id: node.id, disabled: !canDrag })
  const { setNodeRef: setDropRef } = useDroppable({ id: node.id, disabled: !canDrag })

  const isDropBefore = dropTarget?.id === node.id && dropTarget.zone === 'before'
  const isDropOn = dropTarget?.id === node.id && dropTarget.zone === 'on'
  const isDropAfter = dropTarget?.id === node.id && dropTarget.zone === 'after'

  return (
    <div className="relative">
      {isDropBefore ? (
        <div className="pointer-events-none absolute left-2 right-2 top-0 h-0.5 -translate-y-1/2 rounded-full bg-primary" />
      ) : null}
      <div
        ref={setDropRef}
        data-node-id={node.id}
        role="treeitem"
        tabIndex={tabbable ? 0 : -1}
        aria-level={node.depth + 1}
        aria-selected={isSelected}
        aria-expanded={node.hasChildren ? node.expanded : undefined}
        onKeyDown={(e) => onTreeItemKeyDown(e, { expanded: node.expanded, hasChildren: node.hasChildren, onToggle, onOpen: onSelect })}
        style={{ paddingLeft: `${node.depth * 1.125 + 0.5}rem` }}
        className={`group relative mx-1 flex cursor-pointer items-center gap-1 rounded-md py-1.5 pr-2 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring
          ${isSelected
            ? 'bg-primary/10 text-foreground'
            : 'text-foreground/90 hover:bg-muted/60'
          }
          ${isDropOn ? 'ring-2 ring-inset ring-primary' : ''}
          ${isDragging ? 'opacity-40' : ''}
        `}
        onClick={onSelect}
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

        {canDrag ? (
          <span
            ref={setDragRef}
            {...attributes}
            {...listeners}
            className="cursor-grab touch-none text-muted-foreground/60 transition-colors hover:text-muted-foreground active:cursor-grabbing"
            onClick={(e) => e.stopPropagation()}
            title="Drag to move"
            aria-label={`Move ${node.name}`}
          >
            <GripVerticalIcon className="size-3.5 opacity-0 group-hover:opacity-100" />
          </span>
        ) : (
          <span className="size-3.5" />
        )}
        {/* The row's arrow keys expand and collapse, so this stays a mouse target out of the tab order */}
        <button
          type="button"
          tabIndex={-1}
          className="grid size-4 shrink-0 place-items-center text-muted-foreground hover:text-foreground"
          onClick={(e) => { e.stopPropagation(); onToggle() }}
          aria-label={node.expanded ? `Collapse ${node.name}` : `Expand ${node.name}`}
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
            node.isPublished ? 'bg-success' : 'bg-muted-foreground/40'
          }`}
          title={node.isPublished ? 'Published' : 'Draft - hidden on every store'}
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
  canManage,
  onCreated,
  onDeleted,
}: {
  categoryId?: string
  isNew?: boolean
  canManage: boolean
  onCreated?: (id: string) => void
  onDeleted?: () => void
}) {
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  // Locale this form edits - falls back to the system-wide default when the scope hasn't resolved
  // one (GLOBAL with no scope.locale set). Apollo's `localeLink` attaches the same value as the
  // `Locale` request header, so the server resolves the same translation we render against
  const locale = effectiveLocale || globalDefaultLocale
  const { data, loading } = useQuery(CategoryDocument, {
    variables: { id: categoryId! },
    skip: isNew || !categoryId,
  })
  const category = data?.category

  if (!isNew && loading && !category) return <LoadingState label="Loading category…" />
  if (!isNew && !category) return <p className="px-6 py-6 text-sm text-muted-foreground">Category not found.</p>
  // Keyed on the locale - the name, descriptions and SEO fields are that locale's copy
  return (
    <CategoryRecordForm
      key={locale}
      category={category}
      locale={locale}
      canManage={canManage}
      onCreated={onCreated}
      onDeleted={onDeleted}
    />
  )
}

// Create and edit in one: a new category writes straight through, an existing one reviews the
// general fields together with the banner, availability and filter sections staged beside them
function CategoryRecordForm({
  category,
  locale,
  canManage,
  onCreated,
  onDeleted,
}: {
  category?: CategoryDetail
  locale: string
  canManage: boolean
  onCreated?: (id: string) => void
  onDeleted?: () => void
}) {
  const isNew = !category
  const categoryId = category?.id
  const { isGlobal, effectiveChannelId, selectedChannel, channels } = useConfigScope()
  // Stores a new category is listed on at create - null until the admin ticks a box, so the default follows the header
  // (the active store in CHANNEL, every active store in GLOBAL) and still fills in when the channels query lands late
  const [pickedStores, setPickedStores] = useState<Set<string> | null>(null)
  const storeIds = useMemo(
    () =>
      pickedStores ??
      new Set(isGlobal ? channels.filter((c) => c.isActive).map((c) => c.id) : effectiveChannelId ? [effectiveChannelId] : []),
    [pickedStores, isGlobal, channels, effectiveChannelId],
  )
  const { ref: headerRef, hidden: headerHidden } = useScrollHideHeader<HTMLDivElement>()

  // The tree query takes no channel var - scope rides the request header - so refetching it re-reads
  // under the current scope automatically
  // The pickers' flat list is cached separately, so it's refreshed alongside the tree
  const refetchQueries = [{ query: RootCategoriesDocument }, { query: CategoryOptionsDocument }]
  const detailRefetch = categoryId ? [{ query: CategoryDocument, variables: { id: categoryId } }] : []
  const [createCategory] = useMutation(CreateCategoryDocument, { refetchQueries })
  const [setCategoryListing] = useMutation(SetCategoryListingDocument, { refetchQueries })
  const [updateCategory] = useMutation(UpdateCategoryDocument, { refetchQueries: [...refetchQueries, ...detailRefetch] })
  const [upsertContent] = useMutation(UpsertCategoryContentDocument, { refetchQueries: [...refetchQueries, ...detailRefetch] })
  const [deleteCategory] = useMutation(DeleteCategoryDocument, { refetchQueries })
  // Answers with a Boolean, so the detail is fetched again - the category is normalised, so the tree
  // loses the removed locale too
  const [deleteContent] = useMutation(DeleteCategoryContentDocument, { refetchQueries: detailRefetch, awaitRefetchQueries: true })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  // UI state, not the admin's work - an existing slug is never re-derived from the name
  const [slugTouched, setSlugTouched] = useState(!isNew)
  const createdId = useRef('')

  // The parent picker never offers this category or anything beneath it - either would make the
  // category its own ancestor
  const { index: categoryIndex } = useCategoryIndex()
  const notParents = useMemo(() => (categoryId ? subtreeOf(categoryIndex, categoryId) : undefined), [categoryIndex, categoryId])
  // The saved parent's name, for while the list loads or when it sits outside the admin's scope
  const savedParentName = category?.parent ? (category.parent.name ?? category.parent.slug) : undefined
  const parentLabel = useCallback(
    (id: string) => formatPath(categoryIndex, id, id === category?.parent?.id ? savedParentName : undefined),
    [categoryIndex, category?.parent?.id, savedParentName],
  )

  const baseline = useMemo(() => (category ? toCategoryValues(category, locale) : emptyCategory), [category, locale])
  const diff = useMemo(() => categoryDiff(parentLabel), [parentLabel])

  const record = useRecordForm({
    id: 'general',
    title: 'General',
    label: isNew ? 'New category' : `Category · ${category.name ?? category.slug}`,
    isEdit: !isNew,
    baseline,
    schema: categorySchema,
    diff,
    persist: { routeKey: `category:${categoryId ?? 'new'}:${locale}` },
    submit: async (v) => {
      if (!category) {
        const result = await createCategory({ variables: { input: toCategoryCreateInput(v, locale) } })
        const created = result.data?.createCategory
        if (!created) throw new Error('Create succeeded but no category id was returned.')
        createdId.current = created.id
        // Non-fatal per store - the category exists, so a failed listing is reported and added by hand afterwards
        const failed: string[] = []
        for (const channelId of storeIds) {
          try {
            await setCategoryListing({
              variables: { input: { categoryId: created.id, channelId, isPublished: true, sortOrderOverride: null } },
            })
          } catch {
            failed.push(channels.find((c) => c.id === channelId)?.name ?? channelId)
          }
        }
        if (failed.length > 0) {
          toast.error(`Category created, but it couldn't be added to ${failed.join(', ')}. Add it under Per-channel availability.`)
        }
        return
      }
      const changes = categoryChanges(baseline, v)
      if (changes.update) await updateCategory({ variables: { input: { id: category.id, ...changes.update } } })
      if (changes.content) await upsertContent({ variables: { categoryId: category.id, input: { locale, ...changes.content } } })
    },
    onSaved: isNew ? () => onCreated?.(createdId.current) : undefined,
  })
  const { form, registry, confirm, draft, submitError } = record
  const values = useStore(form.store, (s) => s.values)
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  const totalChanges = registry.dirtySections.reduce((n, s) => n + s.diff.length, 0)

  async function onDelete() {
    if (!categoryId) return
    await deleteCategory({ variables: { id: categoryId } })
    onDeleted?.()
  }

  const headerTitle = isNew ? 'New category' : values.name || category.slug || 'Category'
  const headerUrl = !isNew ? (category.urlPath ?? values.slug) : null
  // Surfaces the scope and the locale the write lands on, so the admin can sanity-check before confirming
  const scopeLabel = isGlobal
    ? `Global · ${locale || '(default locale)'}`
    : `${selectedChannel?.name ?? 'Selected store'} · ${locale || '(store default locale)'}`

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <div className="flex flex-col">
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
              {headerUrl ? <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">/{headerUrl}</p> : null}
            </div>
            {canManage ? (
              <div className="flex shrink-0 items-center gap-2">
                <form.Field name="isPublished">
                  {(f) => (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <label htmlFor="cat-published-header" className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1 text-sm">
                          <Switch id="cat-published-header" checked={f.state.value} onCheckedChange={f.handleChange} />
                          <span className={f.state.value ? 'font-medium' : 'text-muted-foreground'}>
                            {f.state.value ? 'Published globally' : 'Draft (all stores)'}
                          </span>
                        </label>
                      </TooltipTrigger>
                      <TooltipContent className="max-w-xs">
                        A store shows this category only when it is published globally AND listed and published on that
                        store - see Per-channel availability.
                      </TooltipContent>
                    </Tooltip>
                  )}
                </form.Field>
                {!isNew ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                    onClick={() => setConfirmingDelete(true)}
                  >
                    <Trash2Icon className="size-4" />
                    Delete
                  </Button>
                ) : null}
                <Button type="button" size="sm" disabled={submitting || (!isNew && !registry.isDirty)} onClick={record.save}>
                  {submitting
                    ? 'Saving…'
                    : isNew
                      ? 'Create'
                      : registry.isDirty
                        ? `Save changes${totalChanges > 0 ? ` (${totalChanges})` : ''}`
                        : 'No changes'}
                </Button>
              </div>
            ) : null}
          </div>
        </div>

        {/* ── Body ────────────────────────────────────────────── */}
        <div className="divide-y pb-16">
          {draft.offer !== null && (
            <div className="px-6 py-4">
              <RestoredDraftBanner draft={draft} labels={CATEGORY_LABELS} />
            </div>
          )}
          {submitError ? (
            <div className="px-6 py-4">
              <Alert variant="destructive">
                <AlertTitle>Create failed</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          ) : null}

          {isNew ? (
            <NewCategoryStores
              channels={channels}
              selected={storeIds}
              currentChannelId={effectiveChannelId}
              isPublished={values.isPublished}
              disabled={!canManage}
              onChange={setPickedStores}
            />
          ) : null}

          <fieldset disabled={!canManage} className="contents">
            {/* ── General ── */}
            <Section title="General" description="Basic info, hierarchy, and visibility.">
              <div className="grid items-start gap-4 md:grid-cols-2">
                <form.AppField
                  name="name"
                  listeners={{
                    onChange: ({ value }) => {
                      if (!slugTouched) form.setFieldValue('slug', slugify(value))
                    },
                  }}
                >
                  {(f) => <f.TextField id="cat-name" label="Name" placeholder="Women's Shoes" />}
                </form.AppField>
                <form.AppField name="slug">
                  {(f) => (
                    <f.TextField id="cat-slug" label="Slug" placeholder="womens-shoes" className="font-mono" onValueChange={() => setSlugTouched(true)} />
                  )}
                </form.AppField>
              </div>

              <div className="grid items-start gap-4 md:grid-cols-[1fr_8rem]">
                <form.AppField name="parentId">
                  {(f) => (
                    <div className="grid gap-1.5">
                      <Label htmlFor="cat-parent">Parent category</Label>
                      <CategorySelect
                        id="cat-parent"
                        value={f.state.value}
                        onChange={(id) => {
                          f.handleChange(id)
                          f.handleBlur()
                        }}
                        noneLabel="None (root)"
                        fallbackLabel={f.state.value === category?.parent?.id ? savedParentName : undefined}
                        excludeIds={notParents}
                        invalid={!!fieldErrorText(f)}
                      />
                      <FieldError field={f} />
                    </div>
                  )}
                </form.AppField>
                <form.AppField name="position">{(f) => <f.NumberField id="cat-position" label="Position" />}</form.AppField>
              </div>

              {/* Name, descriptions and SEO are all per-locale - this is where a locale's copy gets
                  dropped again. An immediate action, like the Delete button in the header */}
              {category ? (
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
          </fieldset>

          {/* ── Products ── the section admins work in most, so it sits right under General */}
          {category ? <CategoryProductsSection category={category} canManage={canManage} /> : null}

          <fieldset disabled={!canManage} className="contents">
            {/* ── Short description ── */}
            <Section title="Short description" description="Brief summary shown in listings.">
              <div className="overflow-hidden rounded-md border">
                <form.Field name="shortDescription">
                  {(f) => (
                    <FormRichTextEditor
                      value={f.state.value as RichTextValue}
                      fallbackText=""
                      disabled={!canManage}
                      placeholder="Brief category summary shown in listings…"
                      onChange={(next) => f.handleChange(next)}
                    />
                  )}
                </form.Field>
              </div>
            </Section>

            {/* ── Description ── */}
            <Section title="Description" description="Full description rendered on the category page.">
              <div className="overflow-hidden rounded-md border">
                <form.Field name="description">
                  {(f) => (
                    <FormRichTextEditor
                      value={f.state.value as RichTextValue}
                      fallbackText=""
                      disabled={!canManage}
                      placeholder="Full category description…"
                      onChange={(next) => f.handleChange(next)}
                    />
                  )}
                </form.Field>
              </div>
            </Section>
          </fieldset>

          {/* ── Banner image ── */}
          {category ? <CategoryCoverSection category={category} canManage={canManage} /> : null}

          {/* ── SEO ── */}
          <fieldset disabled={!canManage} className="contents">
            <Section title="SEO" description="Search metadata and indexing controls.">
              <form.AppField name="metaTitle">
                {(f) => (
                  <f.TextField
                    id="cat-meta-title"
                    label="Meta title"
                    placeholder={values.name || 'Category name'}
                    note={<LengthHint length={values.metaTitle.length} max={60} />}
                  />
                )}
              </form.AppField>
              <form.AppField name="metaDescription">
                {(f) => (
                  <f.TextareaField
                    id="cat-meta-desc"
                    label="Meta description"
                    rows={3}
                    placeholder="Search result description…"
                    note={<LengthHint length={values.metaDescription.length} max={160} />}
                  />
                )}
              </form.AppField>
              <form.AppField name="seoNoindex">
                {(f) => <f.SwitchField id="cat-noindex" label="Noindex" description="Ask search engines not to index this category." bordered={false} />}
              </form.AppField>
            </Section>
          </fieldset>

          {/* ── Per-channel availability ── */}
          {category ? (
            <CategoryAvailabilitySection category={category} canManage={canManage} globalPublished={values.isPublished} />
          ) : null}

          {/* ── Storefront filters ── */}
          {category ? <CategoryFacetOverridesSection category={category} canManage={canManage} /> : null}

        </div>

        {/* ── Delete confirmation ─────────────────────────────── */}
        <ConfirmDialog
          open={confirmingDelete}
          onOpenChange={setConfirmingDelete}
          title={<>Delete {values.name || category?.slug || 'this category'}?</>}
          description="This permanently removes the category. Child categories will be detached from it."
          confirmLabel="Delete category"
          busyLabel="Deleting…"
          destructive
          onConfirm={onDelete}
        />

        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={scopeLabel} sections={registry.dirtySections} />
      </div>
    </SaveRegistryContext.Provider>
  )
}

// Search engines truncate past these lengths - a count beside the field, red once over
function LengthHint({ length, max }: { length: number; max: number }) {
  return (
    <span className={`text-xs tabular-nums ${length > max ? 'text-destructive' : 'text-muted-foreground'}`}>
      {length}/{max}
    </span>
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
    await putToSignedUrl(payload.uploadUrl, file, 'Banner upload')
    await confirmUpload({ variables: { categoryId: category.id, width, height } })
    setPendingFile(null)
  }, [pendingClear, pendingFile, altText, category.id, clearCover, requestUpload, confirmUpload])

  useSaveSection({ id: 'cover', title: 'Banner image', diff, save })

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

  useSaveSection({
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

// Past this many stores the picker grows a search box, bulk select and a scroll cap
const STORE_PICKER_COMPACT_MAX = 8

// Create-only store picker - categories are global rows, so this is where the admin sees (and changes) which stores
// the new one is listed on, instead of the header scope deciding silently
function NewCategoryStores({
  channels,
  selected,
  currentChannelId,
  isPublished,
  disabled,
  onChange,
}: {
  channels: ChannelScopeOption[]
  selected: Set<string>
  currentChannelId: string | undefined
  isPublished: boolean
  disabled: boolean
  onChange: (next: Set<string>) => void
}) {
  const [query, setQuery] = useState('')
  const large = channels.length > STORE_PICKER_COMPACT_MAX
  // The store the admin is working in leads, so it stays in view however long the list gets
  const ordered = useMemo(
    () => [...channels].sort((a, b) => Number(b.id === currentChannelId) - Number(a.id === currentChannelId)),
    [channels, currentChannelId],
  )
  const q = query.trim().toLowerCase()
  const shown = q ? ordered.filter((c) => c.name.toLowerCase().includes(q)) : ordered

  function toggle(id: string, on: boolean) {
    const next = new Set(selected)
    if (on) next.add(id)
    else next.delete(id)
    onChange(next)
  }
  // Bulk actions act on what the search shows, so "select all" under a filter means "all of these"
  function setShown(on: boolean) {
    const next = new Set(selected)
    for (const c of shown) {
      if (on) next.add(c.id)
      else next.delete(c.id)
    }
    onChange(next)
  }

  const picked = channels.filter((c) => selected.has(c.id))
  const where = picked.length <= 3 ? picked.map((c) => c.name).join(', ') : `${picked.length} stores`
  const summary =
    picked.length === 0
      ? 'Not added to any store - it will only show under Global default.'
      : isPublished
        ? `Will be live on ${where}.`
        : `Will be added to ${where}, but hidden until you switch it to Published.`

  return (
    <Section title="Stores" description="Categories are shared by every store. Pick the stores this one appears on.">
      {channels.length === 0 ? (
        <p className="text-sm text-muted-foreground">No stores are configured yet.</p>
      ) : (
        <div className="grid gap-2">
          {large ? (
            <div className="flex flex-wrap items-center gap-2">
              <div className="relative min-w-48 flex-1">
                <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search stores…"
                  aria-label="Search stores"
                  className="h-8 pl-8 text-sm"
                />
              </div>
              <Button type="button" variant="ghost" size="sm" className="h-8" onClick={() => setShown(true)} disabled={disabled || shown.length === 0}>
                {q ? 'Select shown' : 'Select all'}
              </Button>
              <Button type="button" variant="ghost" size="sm" className="h-8" onClick={() => setShown(false)} disabled={disabled || shown.length === 0}>
                {q ? 'Clear shown' : 'Select none'}
              </Button>
              <span className="ml-auto text-xs text-muted-foreground tabular-nums">
                {picked.length} of {channels.length} selected
              </span>
            </div>
          ) : null}
          <div className={cn('divide-y rounded-md border', large && 'scrollbar-accent max-h-64 overflow-y-auto')}>
            {shown.length === 0 ? (
              <p className="px-3 py-4 text-center text-sm text-muted-foreground">No stores match &ldquo;{query}&rdquo;</p>
            ) : (
              shown.map((c) => (
                <label key={c.id} htmlFor={`cat-store-${c.id}`} className="flex cursor-pointer items-center gap-3 px-3 py-2.5 text-sm">
                  <Checkbox
                    id={`cat-store-${c.id}`}
                    checked={selected.has(c.id)}
                    onCheckedChange={(v) => toggle(c.id, v === true)}
                    disabled={disabled}
                  />
                  <span className="flex-1 truncate font-medium">{c.name}</span>
                  {c.id === currentChannelId ? <Badge variant="secondary">You&rsquo;re working here</Badge> : null}
                  {!c.isActive ? <Badge variant="outline">Inactive</Badge> : null}
                </label>
              ))
            )}
          </div>
        </div>
      )}
      {currentChannelId && !selected.has(currentChannelId) ? (
        <p className="text-xs text-warning">
          It won&rsquo;t appear in this store&rsquo;s category list - you&rsquo;ll find it under Global default.
        </p>
      ) : null}
      <p className="rounded-md border bg-muted/40 px-3 py-2 text-sm">{summary}</p>
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
  globalPublished,
}: {
  category: {
    id: string
    // The global flag - a store listing only puts the category on that storefront while this is on too
    isPublished: boolean
    listings: {
      channelId: string
      channel: { id: string; name: string }
      isPublished: boolean
      sortOrderOverride?: number | null
    }[]
  }
  canManage: boolean
  // The header's staged Published/Draft switch - a Draft hides the category on every store whatever its listings say
  globalPublished: boolean
}) {
  const { isChannel, effectiveChannelId, selectedChannel, channels } = useConfigScope()
  // Follows the staged header switch, so the warning appears the moment the save would take the category off every store
  const draftNotice = globalPublished ? null : (
    <Alert variant="warning">
      <AlertTitle>Draft</AlertTitle>
      <AlertDescription>
        This category is a Draft, so it is hidden on every storefront even where it is published below. Switch it to
        Published in the header to make it live.
      </AlertDescription>
    </Alert>
  )
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

  // What a shopper on that store would see once the staged changes are saved - the global flag and
  // the store's own listing both gate it, and this is the one place the two are read together
  function storefrontStatus(d: AvailDraft): { label: string; className: string } {
    const listed = d.pendingRemove ? false : d.serverListing != null || dirtyOp(d) === 'create'
    if (!listed) return { label: 'Not listed', className: 'text-muted-foreground' }
    if (!globalPublished) return { label: 'Hidden: global draft', className: 'text-warning' }
    if (!d.isPublished) return { label: 'Hidden: unpublished here', className: 'text-muted-foreground' }
    return { label: 'Live', className: 'text-success' }
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

  useSaveSection({
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
        {draftNotice}
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
          <p className="text-xs">
            <span className="text-muted-foreground">On storefront: </span>
            <span className={cn('font-medium', storefrontStatus(draft).className)}>{storefrontStatus(draft).label}</span>
          </p>
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
                <span className="text-xs text-warning">
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
      {draftNotice}
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">Channel</th>
              <th className="px-3 py-2 font-medium">Published</th>
              <th className="px-3 py-2 font-medium">Sort override</th>
              <th className="px-3 py-2 font-medium">On storefront</th>
              <th className="px-3 py-2 font-medium text-right">Change</th>
            </tr>
          </thead>
          <tbody>
            {drafts.map((d) => {
              const op = dirtyOp(d)
              const status = storefrontStatus(d)
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
                  <td className={cn('px-3 py-2 text-xs font-medium', status.className)}>{status.label}</td>
                  <td className="px-3 py-2 text-right">
                    {op === 'create' ? (
                      <span className="text-xs text-warning">will create</span>
                    ) : op === 'update' ? (
                      <span className="text-xs text-warning">modified</span>
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
        <p className="text-xs text-warning">
          {dirtyCount} pending change{dirtyCount === 1 ? '' : 's'}. Save with the
          header button - the confirm modal lists every channel change before it
          commits.
        </p>
      ) : null}
    </Section>
  )
}
