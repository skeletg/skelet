import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
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
  useProductSaveSection,
  type SectionDiffEntry,
} from '@/features/catalog/productSaveRegistry'
import {
  AdjustInventoryLevelDocument,
  InventoryLocationsDocument,
  InventoryModeDocument,
  ProductInventoryDocument,
  SetInventoryItemBackorderDocument,
  SetInventoryItemTrackingDocument,
  SyncProductInventoryDocument,
} from './Inventory.gql'

// Per-product inventory state that queues every edit into the ProductDetail page-header
// Save - no inline mutations, so operators edit freely and commit in one click
//
// The table lists every configured location, not just those with levels - a blank quantity
// means no level row yet, and typing stages an adjustInventoryLevel that creates it on Save
export function ProductInventorySection({ productId }: { productId: string }) {
  const { has } = usePermissions()
  const canManage = has('inventory.manage')

  const inventoryQuery = useQuery(ProductInventoryDocument, {
    variables: { productId },
  })
  const locationsQuery = useQuery(InventoryLocationsDocument)
  const modeQuery = useQuery(InventoryModeDocument, { fetchPolicy: 'cache-first' })

  const item = inventoryQuery.data?.productInventory ?? null
  const allLocations = useMemo(() => locationsQuery.data?.inventoryLocations ?? [], [locationsQuery.data])
  const isFlatMode = modeQuery.data?.inventoryMode === 'FLAT'

  const refetchQueries = [{ query: ProductInventoryDocument, variables: { productId } }]
  const [setTrackingMut] = useMutation(SetInventoryItemTrackingDocument, { refetchQueries })
  const [setBackorderMut] = useMutation(SetInventoryItemBackorderDocument, { refetchQueries })
  const [adjustLevelMut] = useMutation(AdjustInventoryLevelDocument, { refetchQueries })
  const [syncProductInventoryMut] = useMutation(SyncProductInventoryDocument, { refetchQueries })

  // null/0 for the "no item row yet" case, so the dirty math works either way
  const serverTracking = item?.tracked ?? false
  const serverBackorder = item?.backorderable ?? false
  const serverThreshold = item?.lowStockThreshold ?? 0
  const serverFlatQty = item?.quantity ?? 0

  // Draft state - what's currently displayed in the controls
  const [tracking, setTrackingLocal] = useState(serverTracking)
  const [backorderable, setBackorderable] = useState(serverBackorder)
  const [lowStockThreshold, setLowStockThreshold] = useState(serverThreshold)
  // Keyed by location id, values kept as strings so the input can hold partial typing -
  // parsed once at commit - An entry exists only for locations touched in this draft
  const [pendingLevels, setPendingLevels] = useState<Record<string, string>>({})
  // Flat-mode single-quantity draft - String for the same partial-typing
  // reason as pendingLevels. null = "untouched, mirror server"
  const [pendingFlatQty, setPendingFlatQty] = useState<string | null>(null)

  // Adjust-state-during-render rather than an effect - keyed on item identity so a server
  // hydrate can't blow away in-flight edits, and save() resets pendingLevels via the refetch
  const [prevItem, setPrevItem] = useState(item)
  if (prevItem !== item) {
    setPrevItem(item)
    setTrackingLocal(serverTracking)
    setBackorderable(serverBackorder)
    setLowStockThreshold(serverThreshold)
  }

  // Lets the table render existing levels alongside not-yet-stocked locations, which show
  // as blank rows the operator can fill in
  const levelByLocation = useMemo(() => {
    const out: Record<string, { quantity: number; reserved: number; available: number }> = {}
    if (!item) return out
    for (const lvl of item.levels) {
      out[lvl.location.id] = {
        quantity: lvl.quantity,
        reserved: lvl.reservedQuantity,
        available: lvl.available,
      }
    }
    return out
  }, [item])

  // Drives every quantity input's `step` and display format - whole-unit fallback until an
  // inventory_items row exists, since the product join isn't available before then
  const decimals = item?.product.quantityDecimals ?? 0
  const step = item?.product.quantityStep || (decimals > 0 ? Math.pow(10, -decimals) : 1)
  const formatQty = useCallback(
    (n: number) => (decimals > 0 ? n.toFixed(decimals) : String(Math.trunc(n))),
    [decimals],
  )

  // Diff: one SectionDiffEntry per field that's actually changed - Drives
  // the header's Save-changes counter and the confirm-save dialog
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    if (tracking !== serverTracking) {
      entries.push({
        label: 'Track inventory',
        before: serverTracking ? 'on' : 'off',
        after: tracking ? 'on' : 'off',
      })
    }
    if (item && backorderable !== serverBackorder) {
      entries.push({
        label: 'Allow backorder',
        before: serverBackorder ? 'on' : 'off',
        after: backorderable ? 'on' : 'off',
      })
    }
    if (item && lowStockThreshold !== serverThreshold) {
      entries.push({
        label: 'Low-stock threshold',
        before: formatQty(serverThreshold),
        after: formatQty(lowStockThreshold),
      })
    }
    if (isFlatMode) {
      // Flat mode leaves per-location levels dormant and hides the table, so pendingLevels stays empty
      if (pendingFlatQty !== null && pendingFlatQty !== '') {
        const next = Number(pendingFlatQty)
        if (!Number.isNaN(next) && next !== serverFlatQty) {
          entries.push({
            label: 'Quantity',
            before: formatQty(serverFlatQty),
            after: formatQty(next),
          })
        }
      }
    } else {
      for (const [locationId, pending] of Object.entries(pendingLevels)) {
        const loc = allLocations.find((l) => l.id === locationId)
        if (!loc) continue
        const cur = levelByLocation[locationId]
        const qtyNum = pending === '' ? null : Number(pending)
        const curQty = cur?.quantity ?? null
        if (qtyNum !== null && qtyNum !== curQty) {
          entries.push({
            label: `Stock at ${loc.name}`,
            before: curQty == null ? '-' : formatQty(curQty),
            after: formatQty(qtyNum),
          })
        }
      }
    }
    return entries
  }, [
    formatQty,
    tracking, serverTracking,
    backorderable, serverBackorder,
    lowStockThreshold, serverThreshold,
    pendingLevels, levelByLocation, allLocations, item, isFlatMode, pendingFlatQty, serverFlatQty,
  ])

  async function save() {
    // Tracking flip first, so a never-tracked product gets its inventory_items row
    // provisioned before the backorder and level writes that require it
    if (tracking !== serverTracking) {
      await setTrackingMut({ variables: { productId, tracked: tracking } })
    }
  // One call updates both backorderable and lowStockThreshold - skipped when neither
  // changed, since it needs an inventory_items row that may not exist yet
    const wantBackorderWrite =
      item != null && (backorderable !== serverBackorder || lowStockThreshold !== serverThreshold)
    if (wantBackorderWrite) {
      await setBackorderMut({
        variables: {
          productId,
          backorderable,
          lowStockThreshold,
        },
      })
    }
    if (isFlatMode) {
      // Flat mode: one syncProductInventory call carries the new qty
      // and stamps last_synced_at / last_synced_source on the server
      if (pendingFlatQty !== null && pendingFlatQty !== '') {
        const next = Number(pendingFlatQty)
        if (!Number.isNaN(next) && next !== serverFlatQty) {
          await syncProductInventoryMut({
            variables: {
              input: { productId, quantity: next },
            },
          })
        }
      }
      setPendingFlatQty(null)
      return
    }
    // Then per-location quantity changes - Sequential so a later
    // failure doesn't hide an earlier one in the header's error
    for (const [locationId, pending] of Object.entries(pendingLevels)) {
      const cur = levelByLocation[locationId]
      const qtyNum = pending === '' ? null : Number(pending)
      if (qtyNum === null || qtyNum === (cur?.quantity ?? null)) continue
      await adjustLevelMut({
        variables: {
          input: {
            productId,
            locationId,
            newQuantity: qtyNum,
          },
        },
      })
    }
    // Refetch happens via the mutation's refetchQueries; clear pending
    // edits so the next render shows the now-server values
    setPendingLevels({})
  }

  useProductSaveSection({ id: 'inventory', title: 'Inventory', diff, save })

  // Stocked rows first, then unstocked, both alphabetical - must run before any early
  // return so the hook order stays stable between the loading and loaded renders
  const sortedLocations = useMemo(() => {
    const withLevel = allLocations.filter((l) => levelByLocation[l.id])
    const without = allLocations.filter((l) => !levelByLocation[l.id])
    const byName = (a: typeof allLocations[number], b: typeof allLocations[number]) => a.name.localeCompare(b.name)
    return [...withLevel.sort(byName), ...without.sort(byName)]
  }, [allLocations, levelByLocation])

  // A big fleet renders as: stocked + draft-edited rows always, unstocked capped behind
  // "show all" - the filter searches everything so any location is one keystroke away
  const [locationSearch, setLocationSearch] = useState('')
  const [showAllLocations, setShowAllLocations] = useState(false)
  const UNSTOCKED_CAP = 25
  const visibleLocations = useMemo(() => {
    const q = locationSearch.trim().toLowerCase()
    if (q !== '') {
      return sortedLocations.filter(
        (l) =>
          l.name.toLowerCase().includes(q) ||
          l.slug.toLowerCase().includes(q) ||
          (l.city ?? '').toLowerCase().includes(q),
      )
    }
    if (showAllLocations) return sortedLocations
    let unstocked = 0
    return sortedLocations.filter((l) => {
      if (levelByLocation[l.id] || pendingLevels[l.id] != null) return true
      unstocked++
      return unstocked <= UNSTOCKED_CAP
    })
  }, [sortedLocations, locationSearch, showAllLocations, levelByLocation, pendingLevels])
  const hiddenCount = locationSearch.trim() === '' && !showAllLocations
    ? sortedLocations.length - visibleLocations.length
    : 0

  if (inventoryQuery.loading && !item) {
    return null
  }

  // Helpers for the input bindings
  function pendingQty(locationId: string): string {
    if (pendingLevels[locationId] != null) return pendingLevels[locationId]
    const cur = levelByLocation[locationId]
    return cur ? formatQty(cur.quantity) : ''
  }
  function setPendingQty(locationId: string, value: string) {
    setPendingLevels((prev) => ({ ...prev, [locationId]: value }))
  }

  // An untracked product can't have inventory_levels rows - the FK needs inventory_items -
  // so accepting input would silently no-op on save
  const levelsEditable = canManage && tracking

  return (
    <section className="grid gap-4 px-6 py-6">
      <div className="grid gap-1">
        <h3 className="text-lg font-semibold tracking-tight"><span className="mr-1.5 text-primary">#</span>Inventory</h3>
        <p className="text-sm text-muted-foreground">
          Physical stock per location. Reservations are subtracted on order
          placement and compensated on cancel/fulfill. Edits queue into the
          page Save button.
        </p>
      </div>

      <div className="grid gap-3 rounded-md border p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-0.5">
            <Label htmlFor="inv-tracking" className="text-sm">Track inventory</Label>
            <p className="text-xs text-muted-foreground">
              {item
                ? 'When off, reservations short-circuit and the raw quantity is returned as-is. Use this when an external ERP owns truth.'
                : "Off: this product behaves as 'always in stock' - orders never write a reservation. Flip on (and Save) to provision the inventory record."}
            </p>
          </div>
          <Switch
            id="inv-tracking"
            checked={tracking}
            disabled={!canManage}
            onCheckedChange={setTrackingLocal}
          />
        </div>

        {item && (
          <>
            <div className="flex items-start justify-between gap-4 border-t pt-3">
              <div className="grid gap-0.5">
                <Label htmlFor="inv-backorder" className="text-sm">Allow backorder</Label>
                <p className="text-xs text-muted-foreground">
                  Permit orders even when stock is depleted. Salable goes negative;
                  operations need to fulfill from incoming stock.
                </p>
              </div>
              <Switch
                id="inv-backorder"
                checked={backorderable}
                disabled={!canManage}
                onCheckedChange={setBackorderable}
              />
            </div>

            <div className="grid grid-cols-2 items-end gap-3 border-t pt-3">
              <div className="grid gap-2">
                <Label htmlFor="inv-low-stock" className="text-sm">Low-stock threshold</Label>
                <p className="text-xs text-muted-foreground">
                  Customer-facing badge fires when net salable falls to or below this.
                </p>
              </div>
              <Input
                id="inv-low-stock"
                type="number"
                inputMode={decimals > 0 ? 'decimal' : 'numeric'}
                step={step}
                value={lowStockThreshold}
                disabled={!canManage}
                onChange={(e) => setLowStockThreshold(Number(e.target.value) || 0)}
              />
            </div>
          </>
        )}
      </div>

      {item && !tracking && !isFlatMode && (
        <Alert>
          <AlertTitle>Tracking is disabled</AlertTitle>
          <AlertDescription>
            Quantities below are recorded but reservations are skipped. Salable
            quantity returns the raw number on order checks.
          </AlertDescription>
        </Alert>
      )}

      {isFlatMode ? (
        <FlatQuantityCard
          serverQty={serverFlatQty}
          pendingQty={pendingFlatQty}
          onChange={setPendingFlatQty}
          lastSyncedAt={item?.lastSyncedAt ?? null}
          lastSyncedSource={item?.lastSyncedSource ?? null}
          decimals={decimals}
          step={step}
          formatQty={formatQty}
          disabled={!canManage || !tracking}
        />
      ) : (
        <div className="rounded-md border">
          {allLocations.length > UNSTOCKED_CAP && (
            <div className="border-b p-2">
              <Input
                value={locationSearch}
                onChange={(e) => setLocationSearch(e.target.value)}
                placeholder={`Filter ${allLocations.length} locations…`}
                className="h-8 max-w-xs"
              />
            </div>
          )}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Location</TableHead>
                <TableHead className="w-28">Quantity</TableHead>
                <TableHead className="w-24">Reserved</TableHead>
                <TableHead className="w-24">Available</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedLocations.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-sm text-muted-foreground">
                    No inventory locations configured. Add one under Inventory → Locations.
                  </TableCell>
                </TableRow>
              ) : (
                visibleLocations.map((loc) => {
                  const cur = levelByLocation[loc.id]
                  const reserved = cur?.reserved ?? 0
                  const available = cur?.available ?? 0
                  return (
                    <TableRow key={loc.id}>
                      <TableCell className="font-medium">
                        {loc.name}
                        <code className="ml-2 text-xs text-muted-foreground">{loc.slug}</code>
                        {!loc.isActive && (
                          <Badge variant="outline" className="ml-2 text-muted-foreground">Inactive</Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        <Input
                          type="number"
                          inputMode={decimals > 0 ? 'decimal' : 'numeric'}
                          step={step}
                          value={pendingQty(loc.id)}
                          placeholder={cur ? undefined : '-'}
                          disabled={!levelsEditable}
                          onChange={(e) => setPendingQty(loc.id, e.target.value)}
                          className="h-8 w-24"
                        />
                      </TableCell>
                      <TableCell>
                        <span className={`font-mono text-sm ${reserved < 0 ? 'text-warning' : 'text-muted-foreground'}`}>
                          {cur ? formatQty(reserved) : '-'}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="font-mono text-sm font-semibold">
                          {cur ? formatQty(available) : '-'}
                        </span>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
          {hiddenCount > 0 && (
            <button
              type="button"
              onClick={() => setShowAllLocations(true)}
              className="w-full border-t p-2 text-center text-xs text-muted-foreground hover:bg-muted/50"
            >
              Show {hiddenCount} more unstocked location{hiddenCount === 1 ? '' : 's'}
            </button>
          )}
        </div>
      )}
    </section>
  )
}

// Flat-mode replacement for the locations table - one quantity input plus a muted
// "Last synced" line, so operators see whether the ERP is up to date
function FlatQuantityCard({
  serverQty,
  pendingQty,
  onChange,
  lastSyncedAt,
  lastSyncedSource,
  decimals,
  step,
  formatQty,
  disabled,
}: {
  serverQty: number
  pendingQty: string | null
  onChange: (v: string) => void
  lastSyncedAt: string | null
  lastSyncedSource: string | null
  decimals: number
  step: number
  formatQty: (n: number) => string
  disabled: boolean
}) {
  const displayValue = pendingQty ?? formatQty(serverQty)
  const syncedLabel = lastSyncedAt
    ? `Last synced ${new Date(lastSyncedAt).toLocaleString()}${lastSyncedSource ? ` by ${lastSyncedSource}` : ''}`
    : 'Never synced - enter a quantity and save to record the first value.'
  return (
    <div className="grid gap-3 rounded-md border p-4">
      <div className="grid gap-2">
        <Label htmlFor="inv-flat-qty" className="text-sm">Quantity</Label>
        <p className="text-xs text-muted-foreground">
          Flat inventory mode is on - a single quantity per product, no warehouses. ERP integrations
          can also push this value via the <code className="font-mono">syncProductInventory</code> mutation.
        </p>
        <Input
          id="inv-flat-qty"
          type="number"
          inputMode={decimals > 0 ? 'decimal' : 'numeric'}
          step={step}
          value={displayValue}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className="h-9 w-40"
        />
      </div>
      <p className="text-xs text-muted-foreground">{syncedLabel}</p>
    </div>
  )
}
