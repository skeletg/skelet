import { useMemo, useState } from 'react'
import { arrayMove } from '@dnd-kit/sortable'
import { ArrowDownIcon, ArrowUpIcon, ChevronDownIcon, LinkIcon, PackageIcon, PlusIcon, Trash2Icon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { ProductOptionPriceType } from '@/graphql/schema.types'
import { formatCents } from '@/lib/format'
import { cn } from '@/lib/utils'
import { OptionPicture } from './OptionPicture'
import { ChannelSurchargeInputs } from '../ChannelSurchargeInputs'
import { ProductPickerDialog } from '../ProductPickerDialog'
import type { SourceProduct } from '../productOptionsStaging'
import { choiceFromProduct, isBlankChoice, newChoice, parsePrice, type ChoiceDraft, type ChoiceErrors, type DraftContext } from './option-draft'

export function PriceInput({
  id,
  label,
  value,
  type,
  currency,
  allowPercent = true,
  disabled,
  invalid,
  onValue,
  onType,
}: {
  id?: string
  label: string
  value: string
  type: ProductOptionPriceType
  currency: string
  allowPercent?: boolean
  disabled?: boolean
  invalid?: boolean
  onValue: (v: string) => void
  onType: (t: ProductOptionPriceType) => void
}) {
  return (
    <div className="flex min-w-0">
      <Input
        id={id} inputMode="decimal" placeholder="0" value={value} disabled={disabled} aria-label={label} aria-invalid={invalid || undefined}
        className="h-8 min-w-0 rounded-r-none text-right tabular-nums" onChange={(e) => onValue(e.target.value)}
      />
      <Select value={type} onValueChange={(t) => onType(t as ProductOptionPriceType)} disabled={disabled || !allowPercent}>
        <SelectTrigger size="sm" className="w-[4.75rem] shrink-0 rounded-l-none border-l-0" aria-label={`${label} unit`}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="FIXED">{currency}</SelectItem>
          <SelectItem value="PERCENT">%</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}

// The choices of a dropdown, radio or checkbox option - one row each, the rarely needed settings folded under it
export function ChoicesEditor({
  choices,
  single,
  productId,
  ctx,
  translating,
  lockedTitles,
  usedBy,
  errors,
  listError,
  onChange,
}: {
  choices: ChoiceDraft[]
  single: boolean
  productId: string
  ctx: DraftContext
  // Translating only renames - prices, defaults and the list itself stay as they are
  translating: boolean
  // Choices not saved yet can't take a translation
  lockedTitles: ReadonlySet<string>
  // What uses a choice - a condition or a rule - so it can't be removed while they do
  usedBy: (valueId: string) => string[]
  errors: Record<string, ChoiceErrors>
  listError?: string
  onChange: (next: ChoiceDraft[]) => void
}) {
  const [open, setOpen] = useState<string | null>(null)
  const [picking, setPicking] = useState<{ mode: 'add' } | { mode: 'link'; key: string } | null>(null)
  const cur = ctx.baseCurrency
  const excluded = useMemo(() => new Set([productId]), [productId])
  const linked = useMemo(() => new Set(choices.flatMap((c) => (c.sourceProduct ? [c.sourceProduct.id] : []))), [choices])

  const patch = (key: string, p: Partial<ChoiceDraft>) =>
    onChange(choices.map((c) => {
      if (c.key === key) return { ...c, ...p }
      // One pre-selected choice at most when the customer can only pick one
      return single && p.isDefault ? { ...c, isDefault: false } : c
    }))

  return (
    <div className="grid gap-3">
      <div className="overflow-hidden rounded-md border">
        <div className="hidden grid-cols-[3.5rem_minmax(0,1fr)_10.5rem_5.5rem_4rem] gap-2 border-b bg-muted/40 px-2 py-1.5 text-xs font-medium text-muted-foreground sm:grid">
          <span />
          <span>Choice</span>
          <span>Extra charge</span>
          <span className="text-center">Pre-selected</span>
          <span />
        </div>
        <ul className="divide-y">
          {choices.map((c, n) => {
            const e = errors[c.key] ?? {}
            const expanded = open === c.key
            const users = c.id ? usedBy(c.id) : []
            const name = c.title.trim() || `Choice ${n + 1}`
            const hasExtras = !!c.sourceProduct || c.lockQty.trim() !== '' || Number(c.quantity) !== 1 || Object.values(c.channelPrices).some((v) => v.trim() !== '')
            return (
              <li key={c.key} className={cn(isBlankChoice(c) && 'bg-muted/20')}>
                <div className="grid grid-cols-[3.5rem_minmax(0,1fr)_4rem] items-start gap-2 px-2 py-2 sm:grid-cols-[3.5rem_minmax(0,1fr)_10.5rem_5.5rem_4rem]">
                  <div className="flex">
                    <Button
                      type="button" variant="ghost" size="icon" className="size-7" disabled={translating || n === 0}
                      aria-label={`Move ${name} up`} onClick={() => onChange(arrayMove(choices, n, n - 1))}
                    ><ArrowUpIcon className="size-3.5" /></Button>
                    <Button
                      type="button" variant="ghost" size="icon" className="size-7" disabled={translating || n === choices.length - 1}
                      aria-label={`Move ${name} down`} onClick={() => onChange(arrayMove(choices, n, n + 1))}
                    ><ArrowDownIcon className="size-3.5" /></Button>
                  </div>
                  <div className="grid min-w-0 gap-1">
                    <div className="flex items-start gap-2">
                      <OptionPicture size="sm" label={name} value={c.image} disabled={translating} onChange={(image) => patch(c.key, { image })} />
                      <Input
                        value={c.title} placeholder={n === 0 ? 'e.g. Gift wrap' : 'Choice name'} aria-label={`Choice ${n + 1} name`}
                        disabled={lockedTitles.has(c.key)} aria-invalid={!!e.title || undefined} className="h-8"
                        onChange={(ev) => patch(c.key, { title: ev.target.value })}
                      />
                    </div>
                    {e.title ? <p className="text-xs text-destructive">{e.title}</p> : null}
                    {c.sourceProduct ? (
                      <p className="flex items-center gap-1 text-xs text-muted-foreground">
                        <LinkIcon className="size-3 shrink-0" />
                        <span className="truncate">
                          {c.sourceProduct.name || c.sourceProduct.sku} ({formatCents(c.sourceProduct.defaultPrice, c.sourceProduct.defaultCurrencyCode)}) plus the extra charge
                        </span>
                      </p>
                    ) : null}
                  </div>
                  <div className="col-span-3 row-start-2 grid gap-1 sm:col-span-1 sm:row-start-auto">
                    <PriceInput
                      label={`${name} extra charge`} value={c.price} type={c.sourceProduct ? 'FIXED' : c.priceType} currency={cur}
                      allowPercent={!c.sourceProduct} disabled={translating} invalid={!!e.price}
                      onValue={(v) => patch(c.key, { price: v })} onType={(t) => patch(c.key, { priceType: t })}
                    />
                    {e.price ? <p className="text-xs text-destructive">{e.price}</p> : null}
                  </div>
                  <div className="col-start-3 row-start-1 flex h-8 items-center justify-center sm:col-start-auto sm:row-start-auto">
                    <Checkbox
                      checked={c.isDefault} disabled={translating} aria-label={`Pre-select ${name}`}
                      onCheckedChange={(on) => patch(c.key, { isDefault: on === true })}
                    />
                  </div>
                  <div className="col-start-3 row-start-1 hidden justify-end sm:col-start-auto sm:row-start-auto sm:flex">
                    <Button
                      type="button" variant="ghost" size="icon" className={cn('size-7', hasExtras && 'text-foreground')} aria-expanded={expanded}
                      aria-label={`More settings for ${name}`} onClick={() => setOpen(expanded ? null : c.key)}
                    ><ChevronDownIcon className={cn('size-4 transition-transform', expanded && 'rotate-180')} /></Button>
                    <span title={users.length > 0 ? `Used by ${users.join(', ')}` : undefined}>
                      <Button
                        type="button" variant="ghost" size="icon" className="size-7 text-muted-foreground hover:text-destructive"
                        disabled={translating || users.length > 0} aria-label={`Remove ${name}`}
                        onClick={() => onChange(choices.filter((x) => x.key !== c.key))}
                      ><Trash2Icon className="size-3.5" /></Button>
                    </span>
                  </div>
                </div>
                {/* Phones get the row actions on their own line, under the price */}
                <div className="flex justify-end gap-1 px-2 pb-2 sm:hidden">
                  <Button type="button" variant="ghost" size="sm" className="h-7" aria-expanded={expanded} onClick={() => setOpen(expanded ? null : c.key)}>More</Button>
                  <Button type="button" variant="ghost" size="sm" className="h-7" disabled={translating || users.length > 0} onClick={() => onChange(choices.filter((x) => x.key !== c.key))}>Remove</Button>
                </div>
                {users.length > 0 ? <p className="px-2 pb-2 pl-[4.5rem] text-xs text-muted-foreground">Used by {users.join(', ')}, so it can't be removed.</p> : null}
                {expanded ? (
                  <div className="grid gap-4 border-t bg-muted/20 px-4 py-3 sm:pl-[4.5rem]">
                    <div className="grid gap-1.5">
                      <Label className="text-xs">Linked product</Label>
                      {c.sourceProduct ? (
                        <div className="flex items-center justify-between gap-2 rounded-md border bg-background px-3 py-2">
                          <div className="min-w-0 text-sm">
                            <p className="truncate font-medium">{c.sourceProduct.name || c.sourceProduct.sku}</p>
                            <p className="font-mono text-xs text-muted-foreground">{c.sourceProduct.sku}</p>
                          </div>
                          <Button type="button" variant="ghost" size="sm" className="h-7" disabled={translating} onClick={() => patch(c.key, { sourceProduct: null })}>
                            <XIcon className="size-3.5" />
                            Unlink
                          </Button>
                        </div>
                      ) : (
                        <div>
                          <Button type="button" variant="outline" size="sm" disabled={translating} onClick={() => setPicking({ mode: 'link', key: c.key })}>
                            <LinkIcon className="size-3.5" />
                            Link a product…
                          </Button>
                        </div>
                      )}
                      <p className="text-xs text-muted-foreground">
                        Picking this choice then charges that product's price plus the extra charge, and takes from its stock.
                      </p>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="grid gap-1.5">
                        <Label htmlFor={`${c.key}-qty`} className="text-xs">Units per pick</Label>
                        <Input id={`${c.key}-qty`} inputMode="decimal" className="h-8" value={c.quantity} disabled={translating} aria-invalid={!!e.quantity || undefined} onChange={(ev) => patch(c.key, { quantity: ev.target.value })} />
                        <p className="text-xs text-muted-foreground">{e.quantity ?? 'The charge is multiplied by this. Usually 1.'}</p>
                      </div>
                      <div className="grid gap-1.5">
                        <Label htmlFor={`${c.key}-lock`} className="text-xs">Set cart quantity to</Label>
                        <Input id={`${c.key}-lock`} inputMode="numeric" placeholder="Don't change" className="h-8" value={c.lockQty} disabled={translating} aria-invalid={!!e.lockQty || undefined} onChange={(ev) => patch(c.key, { lockQty: ev.target.value })} />
                        <p className="text-xs text-muted-foreground">{e.lockQty ?? 'For choices like "Set of 4" - the cart quantity is fixed at this.'}</p>
                      </div>
                    </div>
                    {(c.sourceProduct || c.priceType === 'FIXED') && ctx.channels.length > 0 ? (
                      <div className="grid gap-1.5">
                        <Label className="text-xs">Extra charge per store</Label>
                        <ChannelSurchargeInputs
                          idPrefix={`${c.key}-store`} channels={ctx.channels} baseCurrency={cur} baseAmount={parsePrice(c.price, 'FIXED', cur) ?? 0}
                          values={c.channelPrices} disabled={translating}
                          onChange={(id, raw) => patch(c.key, { channelPrices: { ...c.channelPrices, [id]: raw } })}
                        />
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </li>
            )
          })}
        </ul>
      </div>
      {listError ? <p className="text-xs text-destructive">{listError}</p> : null}
      {translating ? null : (
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => onChange([...choices, newChoice()])}>
            <PlusIcon className="size-4" />
            Add a choice
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={() => setPicking({ mode: 'add' })}>
            <PackageIcon className="size-4" />
            Add products as choices…
          </Button>
        </div>
      )}

      <ProductPickerDialog
        open={picking !== null}
        onOpenChange={(o) => !o && setPicking(null)}
        title={picking?.mode === 'link' ? 'Link a product' : 'Add products as choices'}
        description={
          picking?.mode === 'link'
            ? 'The choice charges this product’s price and takes from its stock.'
            : 'Each product becomes a choice that charges its price and takes from its stock.'
        }
        kinds={['SIMPLE']}
        excludeIds={excluded}
        takenIds={picking?.mode === 'add' ? linked : undefined}
        takenLabel="Already a choice"
        showPrice
        confirmLabel={(count) => (picking?.mode === 'link' ? 'Link product' : count === 0 ? 'Add choices' : `Add ${count} choice${count === 1 ? '' : 's'}`)}
        validate={(picks) => (picking?.mode === 'link' && picks.length > 1 ? 'Pick just one product to link.' : null)}
        onConfirm={(picks) => {
          const products: SourceProduct[] = picks.map((p) => p.product)
          if (picking?.mode === 'link') {
            const p = products[0]
            if (p) patch(picking.key, { sourceProduct: p, priceType: 'FIXED', ...(choices.find((c) => c.key === picking.key)?.title.trim() ? {} : { title: p.name || p.sku || '' }) })
            return
          }
          onChange([...choices.filter((c) => !isBlankChoice(c)), ...products.map(choiceFromProduct)])
        }}
      />
    </div>
  )
}
