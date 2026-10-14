import { useId, useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Link2Icon, ListPlusIcon, PlusIcon, SearchIcon, ShoppingCartIcon, TrendingUpIcon, type LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { ProductLinksDocument, RecommendationProductsDocument, RecommendationSearchDocument, SetProductLinksDocument } from '../Catalog.gql'
import {
  PRODUCT_LINK_KINDS,
  PRODUCT_LINK_LABELS,
  changedLinkKinds,
  productLinksDiff,
  productLinksSchema,
  type ProductLinksValues,
} from '../schemas/product'
import { Section } from './shared'
import { ProductPickerDialog } from '../ProductPickerDialog'
import { RecommendationRows, RecommendationImage } from './RecommendationRows'
import { recommendationName, recommendationSummary, type RecommendationProduct } from './recommendation-products'

// Curated product links: related (PDP "you may also like"), upsell (better alternatives), cross-sell (cart companions) - Order = curated
// display order. "Also bought" recommendations are automatic and appear alongside these on the storefront

const KINDS: { key: keyof ProductLinksValues; label: string; hint: string; Icon: LucideIcon }[] = [
  { key: 'related', label: 'Related', hint: 'Shown on the product page rail.', Icon: Link2Icon },
  { key: 'upsell', label: 'Upsells', hint: 'Higher-value alternatives.', Icon: TrendingUpIcon },
  { key: 'crossSell', label: 'Cross-sells', hint: 'Companions surfaced in the cart.', Icon: ShoppingCartIcon },
]

type SlimProduct = RecommendationProduct

export function RecommendationsSection({ productId, canManage }: { productId: string; canManage: boolean }) {
  const related = useQuery(ProductLinksDocument, { variables: { productId, kind: PRODUCT_LINK_KINDS.related } })
  const upsell = useQuery(ProductLinksDocument, { variables: { productId, kind: PRODUCT_LINK_KINDS.upsell } })
  const crossSell = useQuery(ProductLinksDocument, { variables: { productId, kind: PRODUCT_LINK_KINDS.crossSell } })
  const loaded = !!related.data && !!upsell.data && !!crossSell.data
  const baseline = useMemo<ProductLinksValues>(
    () => ({
      related: related.data?.productLinks ?? [],
      upsell: upsell.data?.productLinks ?? [],
      crossSell: crossSell.data?.productLinks ?? [],
    }),
    [related.data, upsell.data, crossSell.data],
  )
  if (!loaded) {
    const failed = !!(related.error || upsell.error || crossSell.error)
    return (
      <Section title="Recommendations" description="Curated related products, higher-value alternatives, and cart companions.">
        {failed ? <p role="alert" className="text-sm text-destructive">Could not load recommendations. <Button type="button" variant="link" onClick={() => { void related.refetch().catch(() => {}); void upsell.refetch().catch(() => {}); void crossSell.refetch().catch(() => {}) }}>Retry</Button></p> : <p role="status" className="text-sm text-muted-foreground">Loading recommendations…</p>}
      </Section>
    )
  }
  return <RecommendationsEditor productId={productId} canManage={canManage} baseline={baseline} />
}

// Mount the form only once the saved lists arrive, so an async baseline cannot discard a stored draft.
function RecommendationsEditor({ productId, canManage, baseline }: { productId: string; canManage: boolean; baseline: ProductLinksValues }) {
  const [setLinks] = useMutation(SetProductLinksDocument)

  // Metadata for selected products: every id linked now or before, fetched by id so the picker never needs the
  // catalog in memory - plus what was just picked, already known from the search result
  const [picked, setPicked] = useState<Map<string, SlimProduct>>(new Map())
  const [labelIds, setLabelIds] = useState<string[]>([])
  const labelsQuery = useQuery(RecommendationProductsDocument, { variables: { ids: labelIds }, skip: labelIds.length === 0 })
  const labels = (labelsQuery.data ?? labelsQuery.previousData)?.productsByIds as SlimProduct[] | undefined
  const byId = useMemo(() => new Map([...picked, ...(labels ?? []).map((p) => [p.id, p] as const)]), [labels, picked])
  const diff = useMemo(() => {
    // Even unavailable or identically named products must produce a faithful order diff.
    const counts = new Map<string, number>()
    for (const p of byId.values()) {
      const summary = recommendationSummary(p)
      counts.set(summary, (counts.get(summary) ?? 0) + 1)
    }
    return productLinksDiff((id) => {
      const product = byId.get(id)
      const summary = recommendationSummary(product)
      return !product || (counts.get(summary) ?? 0) > 1 ? `${summary} (reference: ${id})` : summary
    })
  }, [byId])

  const { form, draft } = useSectionForm({
    id: 'links',
    title: 'Recommendations',
    baseline,
    schema: productLinksSchema,
    diff,
    routeKey: `product-links:${productId}`,
    enabled: canManage,
    submit: async (v) => {
      for (const k of changedLinkKinds(baseline, v)) {
        await setLinks({
          variables: { productId, kind: PRODUCT_LINK_KINDS[k], linkedProductIds: v[k] },
          refetchQueries: [{ query: ProductLinksDocument, variables: { productId, kind: PRODUCT_LINK_KINDS[k] } }],
        })
      }
    },
  })

  // The label fetch only grows, so a removed product keeps its name if it comes back
  const values = useStore(form.store, (s) => s.values)
  const unnamed = [...baseline.related, ...baseline.upsell, ...baseline.crossSell, ...values.related, ...values.upsell, ...values.crossSell].filter(
    (id) => !labelIds.includes(id),
  )
  if (unnamed.length > 0) setLabelIds([...labelIds, ...new Set(unnamed)])

  return (
    <Section title="Recommendations" description="Curated product links. Automatic 'also bought' suggestions fill in alongside these.">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_LINK_LABELS} />}
      {labelsQuery.error && <p role="alert" className="text-sm text-destructive">Could not load product details. Your selections are preserved. <Button type="button" variant="link" onClick={() => void labelsQuery.refetch().catch(() => {})}>Retry product details</Button></p>}
      {!labelsQuery.loading && !labelsQuery.error && labelIds.some((id) => !byId.has(id)) && <p role="status" className="text-xs text-muted-foreground">Some product details are unavailable. <Button type="button" variant="link" onClick={() => void labelsQuery.refetch().catch(() => {})}>Retry product details</Button></p>}
      <p className="text-xs text-muted-foreground">First product appears first. Drag or use the arrows to reorder. Changes apply when you save the page.</p>
      <div className="grid items-start gap-3 xl:grid-cols-3">
        {KINDS.map(({ key, label, hint, Icon }) => (
          <form.Field key={key} name={key}>
            {(f) => (
              <KindEditor
                productId={productId}
                label={label}
                hint={hint}
                Icon={Icon}
                value={f.state.value}
                onChange={f.handleChange}
                onPick={(p) => setPicked((m) => new Map(m).set(p.id, p))}
                byId={byId}
                metadataLoading={labelsQuery.loading}
                disabled={!canManage}
              />
            )}
          </form.Field>
        ))}
      </div>
    </Section>
  )
}

function KindEditor({
  productId,
  label,
  hint,
  Icon,
  value,
  onChange,
  onPick,
  byId,
  disabled,
  metadataLoading,
}: {
  productId: string
  label: string
  hint: string
  Icon: LucideIcon
  value: string[]
  onChange: (ids: string[]) => void
  onPick: (p: SlimProduct) => void
  byId: Map<string, SlimProduct>
  disabled: boolean
  metadataLoading: boolean
}) {
  const [search, setSearch] = useState('')
  const [browsing, setBrowsing] = useState(false)
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const listId = useId()
  const debounced = useDebounced(search.trim(), 250)
  const linked = useMemo(() => new Set(value), [value])
  const selfId = useMemo(() => new Set([productId]), [productId])
  const pickerQuery = useQuery(RecommendationSearchDocument, {
    variables: { search: debounced, limit: 10 },
    skip: debounced === '', // the dropdown only opens once the operator types
  })
  const candidates = useMemo(() => {
    if (debounced === '' || debounced !== search.trim()) return []
    return ((pickerQuery.data?.productsPaged.items ?? []) as SlimProduct[]).filter((p) => p.id !== productId && !value.includes(p.id)).slice(0, 6)
  }, [debounced, search, pickerQuery.data, productId, value])
  const add = (p: SlimProduct) => {
    if (disabled || p.id === productId || linked.has(p.id)) return
    onPick(p)
    onChange([...value, p.id])
    setSearch('')
    setOpen(false)
    setActiveIndex(0)
  }
  const waiting = search.trim() !== debounced || pickerQuery.loading
  const showing = open && search.trim() !== '' && !disabled

  return (
    <div role="group" aria-labelledby={`${listId}-heading`} className="flex min-w-0 flex-col gap-3 rounded-md border p-3">
      <div className="flex items-start gap-2.5">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 id={`${listId}-heading`} className="text-sm font-medium">{label}</h3>
            <span className="rounded-full bg-muted px-1.5 py-0.5 text-[11px] font-medium tabular-nums text-muted-foreground">{value.length}</span>
          </div>
          <p className="text-xs text-muted-foreground">{hint}</p>
        </div>
      </div>

      {value.length > 0 ? (
        <RecommendationRows value={value} byId={byId} label={label} disabled={disabled} loading={metadataLoading} onChange={onChange} />
      ) : (
        <p className="text-xs text-muted-foreground">No products selected. Search or browse to add products.</p>
      )}

      <div className="relative mt-auto flex gap-2">
        <div className="relative min-w-0 flex-1">
        <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          placeholder="Search products to add…"
          aria-label={`Add to ${label}`}
          value={search}
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={showing}
          aria-controls={showing ? listId : undefined}
          aria-activedescendant={showing && !waiting && candidates[activeIndex] ? `${listId}-${candidates[activeIndex].id}` : undefined}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          onChange={(e) => { setSearch(e.target.value); setOpen(true); setActiveIndex(0) }}
          onKeyDown={(e) => {
            if (e.key === 'Escape') { setOpen(false); return }
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
              e.preventDefault()
              setOpen(true)
              setActiveIndex((i) => candidates.length ? (i + (e.key === 'ArrowDown' ? 1 : -1) + candidates.length) % candidates.length : 0)
            }
            if (e.key === 'Enter' && showing && !waiting && candidates[activeIndex]) { e.preventDefault(); add(candidates[activeIndex]) }
          }}
          disabled={disabled}
          className="h-8 pl-8 text-sm"
        />
        {showing && (
          <div className="absolute z-10 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
            {waiting ? <p role="status" className="p-2 text-xs text-muted-foreground">Searching…</p> : pickerQuery.error ? <div role="alert" className="p-2 text-xs text-destructive">Could not search products. <Button type="button" variant="link" onMouseDown={(e) => e.preventDefault()} onClick={() => void pickerQuery.refetch().catch(() => {})}>Retry search</Button></div> : null}
            <div id={listId} role="listbox" aria-label={`${label} search results`}>
              {!waiting && !pickerQuery.error && candidates.length === 0 && <p role="status" className="p-2 text-xs text-muted-foreground">No products available for this search.</p>}
              {!waiting && !pickerQuery.error && candidates.map((p, i) => (
                <button key={p.id} id={`${listId}-${p.id}`} type="button" role="option" tabIndex={-1} aria-selected={i === activeIndex}
                  className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-muted ${i === activeIndex ? 'bg-muted' : ''}`}
                  onMouseDown={(e) => e.preventDefault()} onMouseEnter={() => setActiveIndex(i)} onClick={() => add(p)}>
                  <RecommendationImage product={p} />
                  <span className="min-w-0 flex-1"><span className="block truncate font-medium">{recommendationName(p)}</span><span className="block truncate font-mono text-xs text-muted-foreground">{p.sku || 'No SKU'}</span></span>
                  <PlusIcon className="size-3.5 shrink-0 text-muted-foreground" />
                </button>
              ))}
            </div>
          </div>
        )}
        </div>
        {/* Linking a whole range at once - the quick search above adds one product per search */}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 shrink-0"
          onClick={() => setBrowsing(true)}
          disabled={disabled}
          aria-label={`Browse products to add to ${label}`}
        >
          <ListPlusIcon className="size-3.5" />
          Browse
        </Button>
      </div>

      <ProductPickerDialog
        open={browsing}
        onOpenChange={setBrowsing}
        title={`Add to ${label}`}
        description={`${hint} Picks join the end of the list, and nothing is saved until you save the page.`}
        takenIds={linked}
        takenLabel="Linked"
        excludeIds={selfId}
        onConfirm={(picks) => {
          if (disabled) return
          const additions = picks.filter(({ product }) => product.id !== productId && !linked.has(product.id))
          for (const { product } of additions) onPick(product)
          onChange([...new Set([...value, ...additions.map((p) => p.product.id)])])
        }}
      />
    </div>
  )
}
