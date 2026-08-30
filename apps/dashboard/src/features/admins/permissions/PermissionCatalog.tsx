import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import {
  CheckIcon,
  ChevronRightIcon,
  ChevronsDownUpIcon,
  ChevronsUpDownIcon,
  SearchIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { RolesDocument } from '@/features/admins/roles/Roles.gql'
import { usePermissions } from '@/shared/auth/usePermissions'
import { PermissionGroupsDocument } from './Permissions.gql'

type RoleRef = { id: string; slug: string; name: string }

// PermissionCatalog: the read-only catalog as the same collapsible TREE the integration picker
// uses - groups fold to one row each in a multi-column grid, leaves answer the two questions
// operators actually ask: "do I have this?" (the green check, via your roles) and "who hands it
// out?" (the roles popover - a 0 marks an orphaned slug)
export function PermissionCatalog() {
  const groupsQuery = useQuery(PermissionGroupsDocument)
  const rolesQuery = useQuery(RolesDocument, { fetchPolicy: 'cache-first' })
  const permissions = usePermissions()

  const [query, setQuery] = useState('')
  const [heldOnly, setHeldOnly] = useState(false)
  const [toggled, setToggled] = useState<Record<string, boolean>>({})

  // permission-id → granting roles, built once per RolesQuery update so each leaf is O(1)
  const rolesByPermissionID = useMemo(() => {
    const map = new Map<string, RoleRef[]>()
    for (const role of rolesQuery.data?.roles ?? []) {
      const seen = new Set<string>()
      for (const s of role.scopes) {
        if (seen.has(s.permission.id)) continue
        seen.add(s.permission.id)
        const list = map.get(s.permission.id) ?? []
        list.push({ id: role.id, slug: role.slug, name: role.name })
        map.set(s.permission.id, list)
      }
    }
    return map
  }, [rolesQuery.data])

  if (groupsQuery.loading) {
    return <p className="text-sm text-muted-foreground">Loading permissions…</p>
  }
  if (groupsQuery.error) {
    return <p className="text-sm text-destructive">Error: {groupsQuery.error.message}</p>
  }

  const groups = groupsQuery.data?.permissionGroups ?? []
  const total = groups.reduce((n, g) => n + g.permissions.length, 0)
  const heldTotal = groups.reduce(
    (n, g) => n + g.permissions.filter((p) => permissions.has(p.slug)).length,
    0,
  )

  const q = query.trim().toLowerCase()
  const filtering = q !== '' || heldOnly
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
      if (heldOnly) {
        perms = perms.filter((p) => permissions.has(p.slug))
      }
      return { group: g, perms }
    })
    .filter(({ perms }) => perms.length > 0)

  const isOpen = (id: string) => (filtering ? true : (toggled[id] ?? false))
  const allOpen = visibleGroups.every(({ group: g }) => isOpen(g.id))

  return (
    <div className="@container grid content-start gap-2">
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
          {total} permissions · {heldTotal} held by you
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
          variant={heldOnly ? 'secondary' : 'ghost'}
          size="sm"
          onClick={() => setHeldOnly((v) => !v)}
        >
          Held by you
        </Button>
      </div>

      {visibleGroups.length === 0 ? (
        <p className="rounded-md border px-3 py-6 text-center text-sm text-muted-foreground">
          {heldOnly && q === '' ? 'You hold no permissions.' : `No permissions match "${query}".`}
        </p>
      ) : (
        /* multi-column so 38 one-line nodes don't stack into a tall strip of whitespace -
           container queries pick the column count so the grid tracks the content pane, and the
           leaves carry more per-row than the picker's, so this maxes out at two columns */
        <div className="grid items-start gap-x-6 overflow-hidden rounded-md border p-2 @3xl:grid-cols-2">
          {visibleGroups.map(({ group: g, perms }) => {
            const heldCount = g.permissions.filter((p) => permissions.has(p.slug)).length
            const open = isOpen(g.id)
            return (
              <Collapsible
                key={g.id}
                open={open}
                onOpenChange={(v) => setToggled((t) => ({ ...t, [g.id]: v }))}
                className="min-w-0"
              >
                <CollapsibleTrigger asChild>
                  <button
                    type="button"
                    className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left hover:bg-muted/40"
                  >
                    <ChevronRightIcon
                      className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-90')}
                    />
                    <span className="truncate text-sm font-medium">{g.name}</span>
                    <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                      {heldCount > 0 ? `${heldCount}/${g.permissions.length}` : g.permissions.length}
                    </span>
                  </button>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  {/* leaves indent past the parent label so the nesting reads the right way round */}
                  <div className="mb-2 ml-[0.95rem] border-l">
                    {perms.map((p) => {
                      const held = permissions.has(p.slug)
                      return (
                        <div
                          key={p.id}
                          className="flex items-center gap-2 rounded-md py-1.5 pr-2 pl-7 hover:bg-muted/40"
                        >
                          <span className="min-w-0 flex-1 truncate text-sm" title={p.name}>
                            {p.name}
                          </span>
                          {held && (
                            <CheckIcon
                              className="size-3.5 shrink-0 text-emerald-600 dark:text-emerald-400"
                              aria-label="Held by you"
                            >
                              <title>You hold this permission via your roles.</title>
                            </CheckIcon>
                          )}
                          <code className="shrink-0 font-mono text-xs text-muted-foreground">{p.slug}</code>
                          <GrantedInRolesBadge roles={rolesByPermissionID.get(p.id) ?? []} />
                        </div>
                      )
                    })}
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

// GrantedInRolesBadge - clickable badge whose popover lists every role that grants this slug, each linked to its detail
// page - renders as a plain "0" pill (no popover) when nothing grants it, so operators can spot orphans at a glance
function GrantedInRolesBadge({ roles }: { roles: RoleRef[] }) {
  if (roles.length === 0) {
    return (
      <Badge
        variant="outline"
        className="shrink-0 font-normal text-muted-foreground"
        title="No role grants this permission yet."
      >
        0 roles
      </Badge>
    )
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="inline-flex shrink-0 items-center"
          aria-label={`Granted in ${roles.length} role${roles.length === 1 ? '' : 's'}`}
        >
          <Badge variant="outline" className="cursor-pointer font-normal hover:bg-muted">
            {roles.length} role{roles.length === 1 ? '' : 's'}
          </Badge>
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 p-2">
        <p className="px-2 pb-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          Granted in
        </p>
        <ul className="grid gap-0.5">
          {roles.map((r) => (
            <li key={r.id}>
              <Link
                to={`/admins/roles/${r.id}`}
                className="grid gap-0.5 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
              >
                <span className="font-medium leading-tight">{r.name}</span>
                <code className="font-mono text-[11px] text-muted-foreground">{r.slug}</code>
              </Link>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
