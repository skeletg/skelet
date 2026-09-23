import { useCallback, useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ProductDocument, UpdateProductDocument } from '../Catalog.gql'
import { useProductSaveSection, type SectionDiffEntry } from '../productSaveRegistry'

// Advanced pricing + inventory fields, split out of the General section so the primary form stays focused on name / SKU
// / price - Registers its own save section; the parent groups it (and Tier pricing) under one collapsible

type AdvancedProduct = {
  id: string
  defaultCurrencyCode: string
  compareAtPrice?: number | null
  specialPrice?: number | null
  specialFrom?: string | null
  specialTo?: string | null
  weightGrams?: number | null
  lengthMm?: number | null
  widthMm?: number | null
  heightMm?: number | null
  minOrderQty?: number | null
  maxOrderQty?: number | null
  costCents?: number | null
}

function dollarsToCents(input: string): number {
  const n = Number(input)
  if (!Number.isFinite(n)) return 0
  return Math.round(n * 100)
}

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2)
}

function isoToLocalInput(iso?: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function localInputToISO(v: string): string | null {
  if (!v) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

export function AdvancedInventoryPricingSection({
  product,
  canManage,
  productVariables,
}: {
  product: AdvancedProduct
  canManage: boolean
  productVariables: { id: string }
}) {
  const numStr = (v?: number | null) => (v != null && v > 0 ? String(v) : '')
  const originalCompareAt = product.compareAtPrice != null ? centsToDollars(product.compareAtPrice) : ''
  const originalSpecial = product.specialPrice != null ? centsToDollars(product.specialPrice) : ''
  const originalSpecialFrom = isoToLocalInput(product.specialFrom)
  const originalSpecialTo = isoToLocalInput(product.specialTo)
  const originalWeight = numStr(product.weightGrams)
  const originalLength = numStr(product.lengthMm)
  const originalWidth = numStr(product.widthMm)
  const originalHeight = numStr(product.heightMm)
  const originalMinQty = numStr(product.minOrderQty)
  const originalMaxQty = numStr(product.maxOrderQty)
  const originalCost = product.costCents != null ? centsToDollars(product.costCents) : ''

  const [compareAt, setCompareAt] = useState(originalCompareAt)
  const [special, setSpecial] = useState(originalSpecial)
  const [specialFrom, setSpecialFrom] = useState(originalSpecialFrom)
  const [specialTo, setSpecialTo] = useState(originalSpecialTo)
  const [weight, setWeight] = useState(originalWeight)
  const [length, setLength] = useState(originalLength)
  const [width, setWidth] = useState(originalWidth)
  const [height, setHeight] = useState(originalHeight)
  const [minQty, setMinQty] = useState(originalMinQty)
  const [maxQty, setMaxQty] = useState(originalMaxQty)
  const [cost, setCost] = useState(originalCost)

  const refetchQueries = [
    'ProductsPaged',
    { query: ProductDocument, variables: productVariables },
  ]
  const [updateProduct] = useMutation(UpdateProductDocument, { refetchQueries })

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    const money = (s: string) => (s.trim() === '' ? '' : (Number(s) || 0).toFixed(2))
    if (money(cost) !== money(originalCost)) {
      entries.push({ label: 'Unit cost', before: money(originalCost) || '-', after: money(cost) || '-' })
    }
    if (money(compareAt) !== money(originalCompareAt)) {
      entries.push({ label: 'Compare-at price', before: money(originalCompareAt) || '-', after: money(compareAt) || '-' })
    }
    if (money(special) !== money(originalSpecial) || specialFrom !== originalSpecialFrom || specialTo !== originalSpecialTo) {
      const fmt = (p: string, f: string, t: string) =>
        p ? `${money(p)}${f ? ` from ${f}` : ''}${t ? ` until ${t}` : ''}` : '-'
      entries.push({
        label: 'Special price',
        before: fmt(originalSpecial, originalSpecialFrom, originalSpecialTo),
        after: fmt(special, specialFrom, specialTo),
      })
    }
    if (weight !== originalWeight || length !== originalLength || width !== originalWidth || height !== originalHeight) {
      const dims = (w: string, l: string, wd: string, h: string) =>
        [w && `${w}g`, (l || wd || h) && `${l || '?'}x${wd || '?'}x${h || '?'}mm`].filter(Boolean).join(' ') || '-'
      entries.push({
        label: 'Weight / dimensions',
        before: dims(originalWeight, originalLength, originalWidth, originalHeight),
        after: dims(weight, length, width, height),
      })
    }
    if (minQty !== originalMinQty || maxQty !== originalMaxQty) {
      entries.push({
        label: 'Order quantity limits',
        before: `${originalMinQty || '-'} to ${originalMaxQty || '-'}`,
        after: `${minQty || '-'} to ${maxQty || '-'}`,
      })
    }
    return entries
  }, [
    cost, compareAt, special, specialFrom, specialTo, weight, length, width, height, minQty, maxQty,
    originalCost, originalCompareAt, originalSpecial, originalSpecialFrom, originalSpecialTo,
    originalWeight, originalLength, originalWidth, originalHeight, originalMinQty, originalMaxQty,
  ])

  const save = useCallback(async () => {
    const pricingDirty =
      compareAt !== originalCompareAt ||
      special !== originalSpecial ||
      specialFrom !== originalSpecialFrom ||
      specialTo !== originalSpecialTo
    const physicalDirty =
      weight !== originalWeight || length !== originalLength || width !== originalWidth || height !== originalHeight
    const qtyBoundsDirty = minQty !== originalMinQty || maxQty !== originalMaxQty
    const costDirty = cost !== originalCost
    if (!pricingDirty && !physicalDirty && !qtyBoundsDirty && !costDirty) return
    const dimVal = (v: string) => (v === '' ? undefined : Math.max(0, Math.round(Number(v))) || 0)
    await updateProduct({
      variables: {
        input: {
          id: product.id,
          // Compare-at: send the value, or the clear flag when emptied
          ...(compareAt
            ? { compareAtPrice: dollarsToCents(compareAt) }
            : { clearCompareAtPrice: true }),
          // Special trio travels together: a price replaces the whole
          // schedule; empty price clears it
          ...(special
            ? {
                specialPrice: dollarsToCents(special),
                specialFrom: localInputToISO(specialFrom),
                specialTo: localInputToISO(specialTo),
              }
            : { clearSpecialPrice: true }),
          ...(physicalDirty
            ? {
                weightGrams: dimVal(weight) ?? 0,
                lengthMm: dimVal(length) ?? 0,
                widthMm: dimVal(width) ?? 0,
                heightMm: dimVal(height) ?? 0,
              }
            : {}),
          ...(qtyBoundsDirty
            ? {
                minOrderQty: minQty === '' ? 0 : Math.max(0, Number(minQty)) || 0,
                maxOrderQty: maxQty === '' ? 0 : Math.max(0, Number(maxQty)) || 0,
              }
            : {}),
          ...(costDirty ? { costCents: cost === '' ? 0 : dollarsToCents(cost) } : {}),
        },
      },
    })
  }, [
    compareAt, special, specialFrom, specialTo, weight, length, width, height, minQty, maxQty, cost,
    originalCompareAt, originalSpecial, originalSpecialFrom, originalSpecialTo,
    originalWeight, originalLength, originalWidth, originalHeight, originalMinQty, originalMaxQty, originalCost,
    product.id, updateProduct,
  ])

  useProductSaveSection({ id: 'advanced', title: 'Advanced inventory & pricing', diff, save })

  const currency = product.defaultCurrencyCode

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Cost &amp; sale pricing</div>
          <p className="text-xs text-muted-foreground">
            Unit cost powers margin reporting. Compare-at renders a strikethrough "was" price; a
            special price is what customers pay while its window is active (leave dates empty for an
            open-ended sale).
          </p>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
          <div className="grid gap-2">
            <Label htmlFor="adv-cost">Unit cost (COGS)</Label>
            <Input
              id="adv-cost"
              type="number"
              step="0.01"
              min="0"
              inputMode="decimal"
              value={cost}
              onChange={(e) => setCost(e.target.value)}
              disabled={!canManage}
              placeholder="-"
            />
            <p className="text-xs text-muted-foreground">In {currency}. Snapshotted onto order lines at placement.</p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adv-compare-at">Compare-at price</Label>
            <Input
              id="adv-compare-at"
              type="number"
              step="0.01"
              min="0"
              inputMode="decimal"
              value={compareAt}
              onChange={(e) => setCompareAt(e.target.value)}
              disabled={!canManage}
              placeholder="-"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adv-special">Special price</Label>
            <Input
              id="adv-special"
              type="number"
              step="0.01"
              min="0"
              inputMode="decimal"
              value={special}
              onChange={(e) => setSpecial(e.target.value)}
              disabled={!canManage}
              placeholder="-"
            />
          </div>
          <div className="grid grid-cols-2 gap-4 sm:col-span-1">
            <div className="grid gap-2">
              <Label htmlFor="adv-special-from">Sale starts</Label>
              <Input
                id="adv-special-from"
                type="datetime-local"
                value={specialFrom}
                onChange={(e) => setSpecialFrom(e.target.value)}
                disabled={!canManage || !special}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="adv-special-to">Sale ends</Label>
              <Input
                id="adv-special-to"
                type="datetime-local"
                value={specialTo}
                onChange={(e) => setSpecialTo(e.target.value)}
                disabled={!canManage || !special}
              />
            </div>
          </div>
        </div>
      </div>

      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Shipping &amp; dimensions</div>
          <p className="text-xs text-muted-foreground">
            Weight and box dimensions feed carrier rate quotes and labels. Leave empty when not
            applicable (digital goods).
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div className="grid gap-2">
            <Label htmlFor="adv-weight">Weight (g)</Label>
            <Input id="adv-weight" type="number" min="0" inputMode="numeric" value={weight} onChange={(e) => setWeight(e.target.value)} disabled={!canManage} placeholder="-" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adv-length">Length (mm)</Label>
            <Input id="adv-length" type="number" min="0" inputMode="numeric" value={length} onChange={(e) => setLength(e.target.value)} disabled={!canManage} placeholder="-" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adv-width">Width (mm)</Label>
            <Input id="adv-width" type="number" min="0" inputMode="numeric" value={width} onChange={(e) => setWidth(e.target.value)} disabled={!canManage} placeholder="-" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adv-height">Height (mm)</Label>
            <Input id="adv-height" type="number" min="0" inputMode="numeric" value={height} onChange={(e) => setHeight(e.target.value)} disabled={!canManage} placeholder="-" />
          </div>
        </div>
      </div>

      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Order quantity limits</div>
          <p className="text-xs text-muted-foreground">
            Minimum and maximum units per order line. Leave empty for no limit.
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4 sm:max-w-sm">
          <div className="grid gap-2">
            <Label htmlFor="adv-min-qty">Min order qty</Label>
            <Input id="adv-min-qty" type="number" min="0" step="any" inputMode="decimal" value={minQty} onChange={(e) => setMinQty(e.target.value)} disabled={!canManage} placeholder="-" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adv-max-qty">Max order qty</Label>
            <Input id="adv-max-qty" type="number" min="0" step="any" inputMode="decimal" value={maxQty} onChange={(e) => setMaxQty(e.target.value)} disabled={!canManage} placeholder="-" />
          </div>
        </div>
      </div>
    </div>
  )
}
