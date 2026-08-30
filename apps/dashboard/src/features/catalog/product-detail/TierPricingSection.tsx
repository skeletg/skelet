import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CustomerGroupsDocument } from '../../customers/CustomerGroups.gql'
import { ProductDocument, SetProductGroupPricesDocument } from '../Catalog.gql'
import { useProductSaveSection, type SectionDiffEntry } from '../productSaveRegistry'
import { added, removed } from '@/shared/saving/diff'
import { Section } from './shared'

// Tier (customer-group / quantity-break) pricing editor - Edits the full row set and saves via the global save flow
// with a replace-all mutation. "All groups" rows apply to every customer; group rows win only for members

type GroupPriceRow = {
  customerGroupId: string | null
  minQty: string
  price: string // dollars in the UI, cents on the wire
}

type ExistingGroupPrice = {
  id: string
  channelId?: string | null
  customerGroupId?: string | null
  minQty: number
  price: number
}

const ALL_GROUPS = '__all__'

function toRows(prices: ExistingGroupPrice[]): GroupPriceRow[] {
  return prices.map((p) => ({
    customerGroupId: p.customerGroupId ?? null,
    minQty: String(p.minQty),
    price: (p.price / 100).toFixed(2),
  }))
}

// Compare NUMERICALLY - the draft holds raw input strings ("12.5") while originals arrive toFixed(2) ("12.50"); a
// string compare left the section permanently dirty after a save
function sameRow(a: GroupPriceRow, b: GroupPriceRow): boolean {
  return (
    a.customerGroupId === b.customerGroupId &&
    Number(a.minQty) === Number(b.minQty) &&
    Number(a.price || 0) === Number(b.price || 0)
  )
}

function rowsEqual(a: GroupPriceRow[], b: GroupPriceRow[]): boolean {
  return a.length === b.length && a.every((r, i) => sameRow(r, b[i]))
}

export function TierPricingSection({
  productId,
  groupPrices,
  currencyCode,
  canManage,
  productVariables,
  embedded = false,
}: {
  productId: string
  groupPrices: ExistingGroupPrice[]
  currencyCode: string
  canManage: boolean
  productVariables: { id: string }
  // When true, render as a nested dashed block (no top-level Section chrome) so
  // it can sit inside another collapsible section
  embedded?: boolean
}) {
  const original = useMemo(() => toRows(groupPrices), [groupPrices])
  const [rows, setRows] = useState<GroupPriceRow[]>(original)
  // Re-seed the draft when the SERVER rows change (post-save refetch) - without this the just-saved draft kept
  // comparing against stale originals and the section stayed dirty forever
  const originalKey = JSON.stringify(original)
  const [prevOriginalKey, setPrevOriginalKey] = useState(originalKey)
  if (prevOriginalKey !== originalKey) {
    setPrevOriginalKey(originalKey)
    setRows(original)
  }
  const groupsQuery = useQuery(CustomerGroupsDocument)
  const groups = useMemo(() => groupsQuery.data?.customerGroups ?? [], [groupsQuery.data])
  const [setPrices] = useMutation(SetProductGroupPricesDocument, {
    refetchQueries: [{ query: ProductDocument, variables: productVariables }],
  })

  const groupName = useCallback(
    (id: string | null) => (id ? groups.find((g) => g.id === id)?.name ?? 'Group' : 'All groups'),
    [groups],
  )

  const diff = useMemo<SectionDiffEntry[]>(() => {
    // Rows the save will actually send (incomplete drafts are filtered
    // there too - an empty price must not count as a pending change)
    const effective = rows.filter((r) => Number(r.minQty) > 0 && r.price !== '')
    if (rowsEqual(effective, original)) return []
    const rowLabel = (r: GroupPriceRow) => `${groupName(r.customerGroupId)} · ${Number(r.minQty)}+`
    const out: SectionDiffEntry[] = []
    for (const r of effective) {
      const prev = original.find((o) => o.customerGroupId === r.customerGroupId && Number(o.minQty) === Number(r.minQty))
      if (!prev) out.push(added(`Tier · ${rowLabel(r)}`, `@ ${Number(r.price).toFixed(2)}`))
      else if (!sameRow(prev, r)) {
        out.push({ kind: 'change', label: `Tier · ${rowLabel(r)}`, before: `@ ${prev.price}`, after: `@ ${Number(r.price).toFixed(2)}` })
      }
    }
    for (const o of original) {
      const still = effective.some((r) => r.customerGroupId === o.customerGroupId && Number(r.minQty) === Number(o.minQty))
      if (!still) out.push(removed(`Tier · ${rowLabel(o)}`, `@ ${o.price}`))
    }
    return out
  }, [rows, original, groupName])

  const save = useCallback(async () => {
    await setPrices({
      variables: {
        productId,
        prices: rows
          .filter((r) => Number(r.minQty) > 0 && r.price !== '')
          .map((r) => ({
            customerGroupId: r.customerGroupId,
            minQty: Number(r.minQty),
            price: Math.round(Number(r.price) * 100),
          })),
      },
    })
  }, [rows, productId, setPrices])

  useProductSaveSection({ id: 'tier-pricing', title: 'Tier pricing', diff, save })

  const cols =
    'grid grid-cols-[minmax(9rem,13rem)_5.5rem_8.5rem_minmax(0,1fr)_2.25rem] items-center gap-3'

  const body = (
    <div className="grid gap-2">
      {rows.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center">
          <p className="text-sm text-muted-foreground">No tier prices yet.</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Offer a lower per-unit price once the cart reaches a quantity - optionally scoped to a
            customer group.
          </p>
        </div>
      ) : (
        <>
          <div className={`${cols} px-3 text-xs font-medium text-muted-foreground`}>
            <span>Customer group</span>
            <span>Min qty</span>
            <span>Price ({currencyCode})</span>
            <span aria-hidden />
            <span aria-hidden />
          </div>
          {rows.map((row, i) => {
            const q = Number(row.minQty)
            const pr = Number(row.price)
            const preview =
              q > 0 && row.price !== '' && Number.isFinite(pr)
                ? `${q}+ → ${pr.toFixed(2)} each`
                : ''
            return (
              <div key={i} className={`${cols} rounded-lg border bg-muted/20 px-3 py-2`}>
                <Select
                  value={row.customerGroupId ?? ALL_GROUPS}
                  onValueChange={(v) =>
                    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, customerGroupId: v === ALL_GROUPS ? null : v } : r)))
                  }
                  disabled={!canManage}
                >
                  <SelectTrigger className="h-9"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value={ALL_GROUPS}>All groups</SelectItem>
                    {groups.map((g) => (
                      <SelectItem key={g.id} value={g.id}>{g.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Input
                  type="number"
                  min="0"
                  step="any"
                  inputMode="decimal"
                  className="h-9"
                  value={row.minQty}
                  onChange={(e) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, minQty: e.target.value } : r)))}
                  disabled={!canManage}
                />
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  inputMode="decimal"
                  className="h-9"
                  value={row.price}
                  onChange={(e) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, price: e.target.value } : r)))}
                  disabled={!canManage}
                />
                <span className="truncate text-xs tabular-nums text-muted-foreground">
                  {preview}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-9 text-muted-foreground hover:text-destructive"
                  onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}
                  disabled={!canManage}
                  aria-label="Remove tier"
                >
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            )
          })}
        </>
      )}
      <Button
        variant="outline"
        size="sm"
        className="w-fit"
        onClick={() => setRows((rs) => [...rs, { customerGroupId: null, minQty: rs.length === 0 ? '1' : '', price: '' }])}
        disabled={!canManage}
      >
        <PlusIcon className="size-4" /> Add tier
      </Button>
    </div>
  )

  if (embedded) {
    return (
      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Tier pricing</div>
          <p className="text-xs text-muted-foreground">
            Quantity-break prices per customer group. The lowest applicable tier beats the regular
            price in the cart.
          </p>
        </div>
        {body}
      </div>
    )
  }

  return (
    <Section
      title="Tier pricing"
      description="Quantity-break prices per customer group. The lowest applicable tier beats the regular price in the cart."
    >
      {body}
    </Section>
  )
}
