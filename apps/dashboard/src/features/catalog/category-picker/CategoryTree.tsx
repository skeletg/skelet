import { useEffect, useId, useMemo, useState, type KeyboardEvent, type ReactNode } from 'react'
import { CheckIcon, ChevronDownIcon, ChevronRightIcon, SearchIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { PATH_SEPARATOR, searchCategories, type CategoryIndex, type CategoryNode } from './category-options'

// The one category list every picker renders - the tree to browse, flat hits with their path while
// searching. It only reports activations; the host decides whether one ticks, selects or stages.
// Focus stays in the search box: ↑/↓ move, Enter picks, and ←/→ fold the tree when the box is empty

export type CategoryTreeProps = {
  index: CategoryIndex
  loading?: boolean
  // Checkboxes for multi pickers, a check mark for single ones
  multi?: boolean
  isChecked: (id: string) => boolean
  // Already linked - shown checked and locked, with `takenLabel` beside the name
  isTaken?: (id: string) => boolean
  takenLabel?: string
  // Never offered, and neither is anything beneath it
  excludeIds?: ReadonlySet<string>
  // A first row standing for "no category", shown while not searching
  none?: { label: ReactNode; checked: boolean; onSelect: () => void }
  onActivate: (id: string) => void
  autoFocus?: boolean
  listClassName?: string
}

const NONE_ID = '__none__'
const INDENT_REM = 1.125

export function CategoryTree({
  index,
  loading = false,
  multi = false,
  isChecked,
  isTaken,
  takenLabel = 'Already added',
  excludeIds,
  none,
  onActivate,
  autoFocus = false,
  listClassName,
}: CategoryTreeProps) {
  const listId = useId()
  const [search, setSearch] = useState('')
  const query = search.trim()

  const offered = useMemo(
    () =>
      index.ordered.filter(
        (n) => !excludeIds?.has(n.id) && !n.ancestorIds.some((a) => excludeIds?.has(a)),
      ),
    [index, excludeIds],
  )
  const offeredIds = useMemo(() => new Set(offered.map((n) => n.id)), [offered])

  // Opens on the ancestors of whatever is already checked or taken, so the current picks are in view
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set())
  const [seededFrom, setSeededFrom] = useState<CategoryIndex | null>(null)
  if (seededFrom !== index && index.ordered.length > 0) {
    setSeededFrom(index)
    const open = new Set<string>()
    for (const n of index.ordered) {
      if (isChecked(n.id) || isTaken?.(n.id)) for (const a of n.ancestorIds) open.add(a)
    }
    setExpanded(open)
  }

  const rows = useMemo(() => {
    if (query) return searchCategories(index, query).filter((n) => offeredIds.has(n.id))
    return offered.filter((n) => n.ancestorIds.every((a) => expanded.has(a)))
  }, [query, index, offered, offeredIds, expanded])

  const rowIds = useMemo(() => [...(none && !query ? [NONE_ID] : []), ...rows.map((r) => r.id)], [none, query, rows])
  const [activeId, setActiveId] = useState<string | null>(null)
  const active = activeId !== null && rowIds.includes(activeId) ? activeId : (rowIds[0] ?? null)

  // Keyboard moves keep the active row in view
  useEffect(() => {
    if (active) document.getElementById(`${listId}-${active}`)?.scrollIntoView?.({ block: 'nearest' })
  }, [active, listId])

  const toggleExpand = (id: string, open?: boolean) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (open ?? !next.has(id)) next.add(id)
      else next.delete(id)
      return next
    })

  const activate = (id: string) => {
    setActiveId(id)
    if (id === NONE_ID) none?.onSelect()
    else if (!isTaken?.(id)) onActivate(id)
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    const at = active ? rowIds.indexOf(active) : -1
    const node = active ? index.byId.get(active) : undefined
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        if (rowIds.length) setActiveId(rowIds[Math.min(at + 1, rowIds.length - 1)])
        break
      case 'ArrowUp':
        e.preventDefault()
        if (rowIds.length) setActiveId(rowIds[Math.max(at - 1, 0)])
        break
      case 'Enter':
        e.preventDefault()
        if (active) activate(active)
        break
      case 'ArrowRight':
        if (search || !node?.childIds.length) break
        e.preventDefault()
        if (!expanded.has(node.id)) toggleExpand(node.id, true)
        else setActiveId(node.childIds.find((id) => offeredIds.has(id)) ?? node.id)
        break
      case 'ArrowLeft':
        if (search || !node) break
        e.preventDefault()
        if (expanded.has(node.id) && node.childIds.length) toggleExpand(node.id, false)
        else if (node.ancestorIds.length) setActiveId(node.ancestorIds[node.ancestorIds.length - 1])
        break
    }
  }

  const branches = offered.filter((n) => n.childIds.length > 0)

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              setActiveId(null)
            }}
            onKeyDown={onKeyDown}
            placeholder="Search categories…"
            aria-label="Search categories"
            role="combobox"
            aria-expanded
            aria-controls={listId}
            aria-activedescendant={active ? `${listId}-${active}` : undefined}
            autoFocus={autoFocus}
            className="h-8 pl-8 text-sm"
          />
        </div>
        {!query && branches.length > 0 ? (
          <div className="flex shrink-0 items-center">
            <Button type="button" variant="ghost" size="xs" onClick={() => setExpanded(new Set(branches.map((n) => n.id)))}>
              Expand all
            </Button>
            <Button type="button" variant="ghost" size="xs" onClick={() => setExpanded(new Set())}>
              Collapse all
            </Button>
          </div>
        ) : null}
      </div>

      <div
        id={listId}
        role={query ? 'listbox' : 'tree'}
        aria-label="Categories"
        aria-multiselectable={multi || undefined}
        className={cn('scrollbar-accent min-h-0 flex-1 overflow-y-auto rounded-md border', listClassName)}
      >
        {none && !query ? (
          <Row
            id={`${listId}-${NONE_ID}`}
            role={query ? 'option' : 'treeitem'}
            active={active === NONE_ID}
            checked={none.checked}
            multi={false}
            onClick={() => activate(NONE_ID)}
            onHover={() => setActiveId(NONE_ID)}
          >
            <span className="flex-1 truncate text-muted-foreground">{none.label}</span>
          </Row>
        ) : null}
        {rows.length === 0 ? (
          <p className="px-3 py-6 text-center text-xs text-muted-foreground">
            {loading
              ? 'Loading categories…'
              : query
                ? <>No categories match &ldquo;{query}&rdquo;</>
                : 'No categories defined yet.'}
          </p>
        ) : (
          rows.map((node) => {
            const taken = isTaken?.(node.id) ?? false
            const isOpen = expanded.has(node.id)
            const hasKids = node.childIds.some((id) => offeredIds.has(id))
            return (
              <Row
                key={node.id}
                id={`${listId}-${node.id}`}
                role={query ? 'option' : 'treeitem'}
                level={query ? undefined : node.depth + 1}
                expanded={query || !hasKids ? undefined : isOpen}
                active={active === node.id}
                checked={taken || isChecked(node.id)}
                disabled={taken}
                multi={multi}
                label={node.label}
                indent={query ? 0 : node.depth}
                onClick={() => activate(node.id)}
                onHover={() => setActiveId(node.id)}
                chevron={
                  query ? null : (
                    <button
                      type="button"
                      tabIndex={-1}
                      className={cn('grid size-4 shrink-0 place-items-center text-muted-foreground hover:text-foreground', !hasKids && 'invisible')}
                      onClick={(e) => {
                        e.stopPropagation()
                        toggleExpand(node.id)
                      }}
                      aria-label={isOpen ? `Collapse ${node.label}` : `Expand ${node.label}`}
                    >
                      {isOpen ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />}
                    </button>
                  )
                }
              >
                <span className="min-w-0 flex-1 truncate">
                  {query ? <SearchLabel node={node} query={query} /> : node.label}
                </span>
                {taken ? (
                  <Badge variant="outline" className="shrink-0 font-normal">{takenLabel}</Badge>
                ) : null}
                {!node.isPublished ? (
                  <Badge variant="outline" className="shrink-0 font-normal text-muted-foreground" title="Draft - hidden on every store">
                    Draft
                  </Badge>
                ) : null}
                <span
                  className="w-10 shrink-0 text-right text-xs tabular-nums text-muted-foreground"
                  title={`${node.productCount} product${node.productCount === 1 ? '' : 's'} assigned directly`}
                >
                  {node.productCount}
                </span>
              </Row>
            )
          })
        )}
      </div>
    </div>
  )
}

function Row({
  id,
  role,
  level,
  expanded,
  active,
  checked,
  disabled = false,
  multi,
  label,
  indent = 0,
  chevron,
  onClick,
  onHover,
  children,
}: {
  id: string
  role: 'option' | 'treeitem'
  level?: number
  expanded?: boolean
  active: boolean
  checked: boolean
  disabled?: boolean
  multi: boolean
  label?: string
  indent?: number
  chevron?: ReactNode
  onClick: () => void
  onHover: () => void
  children: ReactNode
}) {
  return (
    <div
      id={id}
      role={role}
      aria-level={level}
      aria-expanded={expanded}
      aria-selected={checked}
      aria-disabled={disabled || undefined}
      className={cn(
        'relative flex cursor-pointer items-center gap-2 border-b py-1.5 pr-3 text-sm last:border-b-0',
        active && 'bg-muted',
        checked && !active && 'bg-muted/50',
        disabled && 'cursor-default',
      )}
      style={{ paddingLeft: `${indent * INDENT_REM + 0.5}rem` }}
      onClick={onClick}
      onMouseMove={onHover}
    >
      {Array.from({ length: indent }).map((_, i) => (
        <span
          key={i}
          aria-hidden
          className="pointer-events-none absolute top-0 bottom-0 w-px bg-border/60"
          style={{ left: `${i * INDENT_REM + 0.875}rem` }}
        />
      ))}
      {chevron}
      {multi ? (
        <Checkbox
          checked={checked}
          disabled={disabled}
          tabIndex={-1}
          onClick={(e) => e.stopPropagation()}
          onCheckedChange={onClick}
          aria-label={label ? `Select ${label}` : undefined}
        />
      ) : (
        <CheckIcon className={cn('size-4 shrink-0', checked ? 'opacity-100' : 'opacity-0')} />
      )}
      {children}
    </div>
  )
}

// A hit's ancestors dimmed ahead of its own name, with the matched text marked
function SearchLabel({ node, query }: { node: CategoryNode; query: string }) {
  const parents = node.path.slice(0, -1)
  const at = node.label.toLowerCase().indexOf(query.toLowerCase())
  return (
    <span title={node.path.join(PATH_SEPARATOR)}>
      {parents.length ? <span className="text-muted-foreground">{parents.join(PATH_SEPARATOR) + PATH_SEPARATOR}</span> : null}
      {at < 0 ? (
        <span className="font-medium">{node.label}</span>
      ) : (
        <span className="font-medium">
          {node.label.slice(0, at)}
          <mark className="bg-transparent text-foreground underline decoration-2 underline-offset-2">
            {node.label.slice(at, at + query.length)}
          </mark>
          {node.label.slice(at + query.length)}
        </span>
      )}
    </span>
  )
}
