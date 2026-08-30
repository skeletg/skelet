import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ChevronRightIcon, ChevronsDownUpIcon, ChevronsUpDownIcon, SearchIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'

// Permission picker as a TREE (shadcn Collapsible + tri-state Checkbox): groups are collapsible
// nodes, permissions the indented leaves - groups holding a selection open by default so an
// existing grant set reads at a glance, everything else stays folded to one row
// The search box matches names, slugs and group names; matches (and the selected-only view)
// force the surviving groups open - Callers pass the set of selected slugs and receive the
// updated set on each toggle
export function PermissionSelector({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (next: string[]) => void
}) {
  const { data, loading, error } = useQuery(PermissionGroupsDocument)
  const [query, setQuery] = useState('')
  const [selectedOnly, setSelectedOnly] = useState(false)
  // explicit open/closed choices per group - anything untouched falls back to "open when it
  // holds a selection", so checking into a folded group unfolds it without an effect
  const [toggled, setToggled] = useState<Record<string, boolean>>({})
  const selectedSet = new Set(selected)

  const toggle = (slug: string) => {
    const next = new Set(selectedSet)
    if (next.has(slug)) next.delete(slug)
    else next.add(slug)
    onChange([...next])
  }

  if (loading) return <p className="text-sm text-muted-foreground">Loading permissions…</p>
  if (error) return <p className="text-sm text-destructive">Error: {error.message}</p>

  const groups = data?.permissionGroups ?? []
  const total = groups.reduce((n, g) => n + g.permissions.length, 0)

  const q = query.trim().toLowerCase()
  const filtering = q !== '' || selectedOnly
  const visibleGroups = groups
    .map((g) => {
      // a group-name hit keeps the whole group so "orders" surfaces every orders permission
      const groupHit = q !== '' && (g.name.toLowerCase().includes(q) || g.slug.toLowerCase().includes(q))
      let perms = g.permissions
      if (q !== '' && !groupHit) {
        perms = perms.filter(
          (p) => p.name.toLowerCase().includes(q) || p.slug.toLowerCase().includes(q),
        )
      }
      if (selectedOnly) {
        perms = perms.filter((p) => selectedSet.has(p.slug))
      }
      return { group: g, perms }
    })
    .filter(({ perms }) => perms.length > 0)

  const selCountOf = (g: (typeof groups)[number]) =>
    g.permissions.filter((p) => selectedSet.has(p.slug)).length
  const isOpen = (g: (typeof groups)[number]) =>
    filtering ? true : (toggled[g.id] ?? selCountOf(g) > 0)
  const allOpen = visibleGroups.every(({ group: g }) => isOpen(g))

  return (
    <div className="@container grid gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1">
          <SearchIcon className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search permissions…"
            className="pl-8"
          />
        </div>
        <span className="text-sm text-muted-foreground">
          {selected.length} of {total} selected
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={filtering}
          title={filtering ? 'Search results are always expanded' : undefined}
          onClick={() => setToggled(Object.fromEntries(groups.map((g) => [g.id, !allOpen])))}
        >
          {allOpen ? <ChevronsDownUpIcon className="size-4" /> : <ChevronsUpDownIcon className="size-4" />}
          {allOpen ? 'Collapse all' : 'Expand all'}
        </Button>
        <Button
          type="button"
          variant={selectedOnly ? 'secondary' : 'ghost'}
          size="sm"
          disabled={selected.length === 0 && !selectedOnly}
          onClick={() => setSelectedOnly((v) => !v)}
        >
          Selected only
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={selected.length === 0}
          onClick={() => onChange([])}
        >
          Clear
        </Button>
      </div>

      {visibleGroups.length === 0 ? (
        <p className="rounded-md border px-3 py-6 text-center text-sm text-muted-foreground">
          {selectedOnly && selected.length === 0
            ? 'Nothing selected yet.'
            : `No permissions match "${query}".`}
        </p>
      ) : (
        /* multi-column so 38 one-line nodes don't stack into a tall strip of whitespace - container
           queries (not viewport ones) pick the column count, since the same picker renders in the
           wide detail page AND the narrow create dialog */
        <div className="grid items-start gap-x-6 overflow-hidden rounded-md border p-2 @xl:grid-cols-2 @5xl:grid-cols-3">
          {visibleGroups.map(({ group: g, perms }) => {
            // the node checkbox drives the VISIBLE permissions, so with a search active it
            // grants exactly what the user is looking at rather than the group's hidden rest
            const visibleSlugs = perms.map((p) => p.slug)
            const allChecked = visibleSlugs.every((s) => selectedSet.has(s))
            const someChecked = visibleSlugs.some((s) => selectedSet.has(s))
            const selectedCount = selCountOf(g)
            const open = isOpen(g)
            const toggleGroup = () => {
              const next = new Set(selectedSet)
              if (allChecked) visibleSlugs.forEach((s) => next.delete(s))
              else visibleSlugs.forEach((s) => next.add(s))
              onChange([...next])
            }
            return (
              <Collapsible
                key={g.id}
                open={open}
                onOpenChange={(v) => setToggled((t) => ({ ...t, [g.id]: v }))}
                className="min-w-0"
              >
                <div className="flex items-center gap-2 rounded-md px-2 py-2 hover:bg-muted/40">
                  <CollapsibleTrigger asChild>
                    <button
                      type="button"
                      aria-label={`${open ? 'Collapse' : 'Expand'} ${g.name}`}
                      className="flex size-6 shrink-0 items-center justify-center rounded-md hover:bg-muted"
                    >
                      <ChevronRightIcon
                        className={cn('size-4 text-muted-foreground transition-transform', open && 'rotate-90')}
                      />
                    </button>
                  </CollapsibleTrigger>
                  <Checkbox
                    id={`group-${g.id}`}
                    checked={allChecked ? true : someChecked ? 'indeterminate' : false}
                    onCheckedChange={toggleGroup}
                    title={`Toggle every ${g.name} permission`}
                  />
                  <CollapsibleTrigger asChild>
                    <button type="button" className="flex min-w-0 flex-1 items-center gap-2 text-left">
                      <span className="truncate text-sm font-medium">{g.name}</span>
                      <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                        {selectedCount > 0 ? `${selectedCount}/${g.permissions.length}` : g.permissions.length}
                      </span>
                    </button>
                  </CollapsibleTrigger>
                </div>
                <CollapsibleContent>
                  {/* the guide line hangs off the chevron column, tree-style */}
                  <div className="mb-2 ml-[1.05rem] border-l">
                    {/* pl-10 lands the leaf checkbox under the PARENT'S LABEL - the parent row's
                        chevron pushes its own checkbox right, so a shallow indent read as the
                        parent being nested under its children */}
                    {perms.map((p) => (
                      <div
                        key={p.id}
                        title={p.slug}
                        className="flex items-center gap-2 rounded-md py-1.5 pr-2 pl-10 hover:bg-muted/40"
                      >
                        <Checkbox
                          id={`perm-${p.id}`}
                          checked={selectedSet.has(p.slug)}
                          onCheckedChange={() => toggle(p.slug)}
                        />
                        <Label
                          htmlFor={`perm-${p.id}`}
                          className="min-w-0 flex-1 cursor-pointer truncate text-sm font-normal"
                        >
                          {p.name}
                        </Label>
                        <code className="shrink-0 font-mono text-xs text-muted-foreground">{p.slug}</code>
                      </div>
                    ))}
                  </div>
                </CollapsibleContent>
              </Collapsible>
            )
          })}
        </div>
      )}
    </div>
  )
}
