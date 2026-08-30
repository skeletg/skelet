import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { FlatModeDormancyBanner } from './FlatModeDormancyBanner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  InventoryLocationsDocument,
  SetChannelLocationsDocument,
  ChannelWithLocationsDocument,
} from './Inventory.gql'

type Assignment = { locationId: string; priority: number }

// Editor component for a single channel's inventory location assignments
// Renders an ordered list with up/down/remove controls plus a searchable "add
// location" picker - Persists via setChannelLocations on every change
//
// Mirrors the locale-assignment pattern in ChannelForm except priority is
// significant (lower = preferred for allocation), so we expose explicit
// up/down arrows rather than a single "selected" toggle
export function ChannelLocationsEditor({ channelId }: { channelId: string }) {
  const { has } = usePermissions()
  const canManage = has('inventory.manage')

  const allLocationsQuery = useQuery(InventoryLocationsDocument)
  const channelQuery = useQuery(ChannelWithLocationsDocument, {
    variables: { id: channelId },
  })

  // Memoize the `?? []` fallbacks so the empty-array reference is
  // stable across renders - Without this, every render produces a new
  // `[]` literal, which makes the `useEffect` below treat
  // `channelLocs` as having changed and re-run setAssignments
  // forever (Object.is([], []) is false) - The lint rule
  // react-hooks/exhaustive-deps flags this exact pattern
  const allLocations = useMemo(
    () => allLocationsQuery.data?.inventoryLocations ?? [],
    [allLocationsQuery.data],
  )
  const channelLocs = useMemo(
    () => channelQuery.data?.channel.inventoryLocations ?? [],
    [channelQuery.data],
  )

  // Local assignment draft derived from the server payload - Sort by
  // priority asc (server already does it but the local state needs the same)
  const deriveAssignments = (locs: typeof channelLocs): Assignment[] =>
    [...locs]
      .sort((a, b) => a.priority - b.priority)
      .map((s) => ({ locationId: s.location.id, priority: s.priority }))

  const [assignments, setAssignments] = useState<Assignment[]>(() => deriveAssignments(channelLocs))
  const [picking, setPicking] = useState('')
  const [error, setError] = useState<string | null>(null)

  // Sync local state when the server payload arrives or refreshes -
  // adjust-state-during-render pattern instead of an effect
  const [prevChannelLocs, setPrevChannelLocs] = useState(channelLocs)
  if (prevChannelLocs !== channelLocs) {
    setPrevChannelLocs(channelLocs)
    setAssignments(deriveAssignments(channelLocs))
  }

  const refetchQueries = [
    { query: ChannelWithLocationsDocument, variables: { id: channelId } },
  ]
  const [setLocations, { loading: saving }] = useMutation(SetChannelLocationsDocument, {
    refetchQueries,
  })

  const locationsById = useMemo(() => {
    const m = new Map<string, (typeof allLocations)[number]>()
    for (const l of allLocations) m.set(l.id, l)
    return m
  }, [allLocations])

  const unassignedLocations = useMemo(() => {
    const assigned = new Set(assignments.map((a) => a.locationId))
    return allLocations.filter((l) => !assigned.has(l.id))
  }, [allLocations, assignments])

  // Persist the current assignment order, normalising priorities to 0..n-1
  // so up/down moves don't accumulate gaps over time
  async function persist(next: Assignment[]) {
    setError(null)
    const normalised = next.map((a, i) => ({ locationId: a.locationId, priority: i }))
    setAssignments(normalised)
    try {
      await setLocations({
        variables: { channelId, assignments: normalised },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  function move(idx: number, dir: -1 | 1) {
    const target = idx + dir
    if (target < 0 || target >= assignments.length) return
    const next = [...assignments]
    ;[next[idx], next[target]] = [next[target], next[idx]]
    void persist(next)
  }

  function remove(idx: number) {
    const next = assignments.filter((_, i) => i !== idx)
    void persist(next)
  }

  function add() {
    if (!picking) return
    const next = [...assignments, { locationId: picking, priority: assignments.length }]
    setPicking('')
    void persist(next)
  }

  return (
    <DetailPageSection description="Locations whose stock is salable on this channel, in allocation order (top first). Stock at unassigned locations is invisible to this channel.">
        <FlatModeDormancyBanner />
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Save failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {assignments.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No locations assigned. Stock won't be reserved on this channel until
            at least one location is assigned.
          </p>
        ) : (
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-16">Order</TableHead>
                  <TableHead>Location</TableHead>
                  <TableHead className="w-24">Status</TableHead>
                  {canManage && <TableHead className="w-24" />}
                </TableRow>
              </TableHeader>
              <TableBody>
                {assignments.map((a, idx) => {
                  const loc = locationsById.get(a.locationId)
                  return (
                    <TableRow key={a.locationId}>
                      <TableCell className="font-mono text-xs">{a.priority}</TableCell>
                      <TableCell>
                        <span className="font-medium">{loc?.name ?? a.locationId}</span>
                        {loc && (
                          <code className="ml-2 text-xs text-muted-foreground">{loc.slug}</code>
                        )}
                      </TableCell>
                      <TableCell>
                        {loc?.isActive ? (
                          <Badge variant="secondary">Active</Badge>
                        ) : (
                          <Badge variant="outline" className="text-muted-foreground">Inactive</Badge>
                        )}
                      </TableCell>
                      {canManage && (
                        <TableCell>
                          <div className="flex gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7"
                              disabled={idx === 0 || saving}
                              onClick={() => move(idx, -1)}
                              aria-label="Move up"
                            >
                              <ArrowUpIcon className="size-3.5" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7"
                              disabled={idx === assignments.length - 1 || saving}
                              onClick={() => move(idx, 1)}
                              aria-label="Move down"
                            >
                              <ArrowDownIcon className="size-3.5" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive"
                              disabled={saving}
                              onClick={() => remove(idx)}
                              aria-label="Remove"
                            >
                              <Trash2Icon className="size-3.5" />
                            </Button>
                          </div>
                        </TableCell>
                      )}
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}

        {canManage && unassignedLocations.length > 0 && (
          <div className="flex items-end gap-2">
            <div className="grid grow gap-1.5">
              <AutocompletePicker
                options={unassignedLocations.map((l) => ({ value: l.id, label: l.name, hint: l.slug }))}
                value={picking}
                onChange={(v) => setPicking(v as string)}
                placeholder="Pick a location to add…"
                emptyMessage="No matching location."
              />
            </div>
            <Button onClick={add} disabled={!picking || saving}>
              <PlusIcon className="size-4" />
              Add
            </Button>
          </div>
        )}
    </DetailPageSection>
  )
}
