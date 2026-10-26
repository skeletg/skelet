import { useEffect, useMemo, useRef, useState } from 'react'
import { useApolloClient, useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Link2Icon, PlusIcon, SearchIcon, ShoppingCartIcon, TrendingUpIcon, type LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { ProductLinksDocument, RecommendationProductsDocument, SetProductLinksDocument } from '../Catalog.gql'
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
import { RecommendationRows } from './RecommendationRows'
import { LONG_LIST, matchesRecommendation, recommendationSummary, type RecommendationProduct } from './recommendation-products'

// Curated product links: related (PDP "you may also like"), upsell (better alternatives), cross-sell (cart companions) - Order = curated
// display order. "Also bought" recommendations are automatic and appear alongside these on the storefront

type LinkKind = keyof ProductLinksValues

const KINDS: { key: LinkKind; label: string; noun: string; hint: string; Icon: LucideIcon }[] = [
  { key: 'related', label: 'Related', noun: 'related products', hint: 'Shown in the "you may also like" rail on the product page.', Icon: Link2Icon },
  { key: 'upsell', label: 'Upsells', noun: 'upsells', hint: 'Higher-value alternatives suggested instead of this product.', Icon: TrendingUpIcon },
  { key: 'crossSell', label: 'Cross-sells', noun: 'cross-sells', hint: 'Companions surfaced in the cart alongside this product.', Icon: ShoppingCartIcon },
]

type SlimProduct = RecommendationProduct

// productsByIds answers at most 100 ids, and three lists of up to 200 links each can need far more
const LABEL_BATCH = 100

function useRecommendationProducts(ids: string[]) {
  const client = useApolloClient()
  const key = ids.join(',')
  const [attempt, setAttempt] = useState(0)
  const [result, setResult] = useState<{ key: string; attempt: number; products: SlimProduct[]; error: Error | null }>(
    { key: '', attempt: 0, products: [], error: null },
  )
  const retried = useRef(0)
  useEffect(() => {
    if (key === '') return
    let live = true
    const all = key.split(',')
    const fetchPolicy = attempt !== retried.current ? 'network-only' : 'cache-first'
    retried.current = attempt
    const batches = Array.from({ length: Math.ceil(all.length / LABEL_BATCH) }, (_, i) => all.slice(i * LABEL_BATCH, (i + 1) * LABEL_BATCH))
    Promise.all(batches.map((batch) => client.query({ query: RecommendationProductsDocument, variables: { ids: batch }, fetchPolicy })))
      .then((answers) => {
        if (live) setResult({ key, attempt, products: answers.flatMap((a) => (a.data?.productsByIds ?? []) as SlimProduct[]), error: null })
      })
      .catch((error: Error) => {
        if (live) setResult((prev) => ({ ...prev, key, attempt, error }))
      })
    return () => {
      live = false
    }
  }, [client, key, attempt])
  // Until the new set lands the previous products stay, so rows keep their names while a pick is looked up
  const settled = result.key === key && result.attempt === attempt
  return {
    products: result.products,
    loading: key !== '' && !settled,
    error: settled ? result.error : null,
    retry: () => setAttempt((a) => a + 1),
  }
}

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
  const [tab, setTab] = useState<LinkKind>('related')

  // Metadata for selected products: every id linked now or before, fetched by id so the picker never needs the
  // catalog in memory - plus what was just picked, already known from the picker's page
  const [picked, setPicked] = useState<Map<string, SlimProduct>>(new Map())
  const [labelIds, setLabelIds] = useState<string[]>([])
  const labels = useRecommendationProducts(labelIds)
  const byId = useMemo(() => new Map([...picked, ...labels.products.map((p) => [p.id, p] as const)]), [labels.products, picked])
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

  const changed = new Set(changedLinkKinds(baseline, values))

  return (
    <Section title="Recommendations" description="Curated product links, shown in the order you set. Automatic 'also bought' suggestions fill in alongside these.">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_LINK_LABELS} />}
      {labels.error && <p role="alert" className="text-sm text-destructive">Could not load product details. Your selections are preserved. <Button type="button" variant="link" onClick={labels.retry}>Retry product details</Button></p>}
      {!labels.loading && !labels.error && labelIds.some((id) => !byId.has(id)) && <p role="status" className="text-xs text-muted-foreground">Some product details are unavailable. <Button type="button" variant="link" onClick={labels.retry}>Retry product details</Button></p>}
      <Tabs value={tab} onValueChange={(v) => setTab(v as LinkKind)} className="grid gap-4">
        <TabsList className="h-10 max-w-full justify-start justify-self-start overflow-x-auto">
          {KINDS.map(({ key, label, Icon }) => (
            <TabsTrigger key={key} value={key} className="gap-1.5 px-3">
              <Icon className="size-3.5" />
              {label}
              <span className="rounded-full bg-muted-foreground/10 px-1.5 text-[11px] tabular-nums">{values[key].length}</span>
              {changed.has(key) ? (
                <>
                  <span aria-hidden className="size-1.5 rounded-full bg-warning" />
                  <span className="sr-only">, unsaved changes</span>
                </>
              ) : null}
            </TabsTrigger>
          ))}
        </TabsList>
        {KINDS.map((kind) => (
          <TabsContent key={kind.key} value={kind.key} className="mt-0">
            <form.Field name={kind.key}>
              {(f) => (
                <KindEditor
                  productId={productId}
                  kind={kind}
                  value={f.state.value}
                  saved={baseline[kind.key]}
                  onChange={f.handleChange}
                  onPick={(products) => setPicked((m) => new Map([...m, ...products.map((p) => [p.id, p] as const)]))}
                  byId={byId}
                  metadataLoading={labels.loading}
                  canManage={canManage}
                />
              )}
            </form.Field>
          </TabsContent>
        ))}
      </Tabs>
    </Section>
  )
}

function KindEditor({
  productId,
  kind: { label, noun, hint },
  value,
  saved,
  onChange,
  onPick,
  byId,
  canManage,
  metadataLoading,
}: {
  productId: string
  kind: (typeof KINDS)[number]
  value: string[]
  saved: string[]
  onChange: (ids: string[]) => void
  onPick: (products: SlimProduct[]) => void
  byId: Map<string, SlimProduct>
  canManage: boolean
  metadataLoading: boolean
}) {
  const [browsing, setBrowsing] = useState(false)
  const [query, setQuery] = useState('')
  if (value.length === 0 && query !== '') setQuery('')
  // Kept while a filter is typed, or a removal that shortens the list would strand it filtered
  const long = value.length > LONG_LIST || query !== ''
  const linked = useMemo(() => new Set(value), [value])
  const added = useMemo(() => new Set(value.filter((id) => !saved.includes(id))), [value, saved])
  const selfId = useMemo(() => new Set([productId]), [productId])

  const addButton = canManage ? (
    <Button type="button" size="sm" onClick={() => setBrowsing(true)} aria-label={`Add products to ${label}`}>
      <PlusIcon className="size-4" />
      Add products
    </Button>
  ) : null

  return (
    <div className="grid gap-3">
      {value.length > 0 ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">{hint} Drag or use the arrows to set the order.</p>
            {long ? null : addButton}
          </div>
          {long ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex min-w-0 flex-1 flex-wrap items-center gap-3">
                <div className="relative w-full max-w-xs">
                  <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    type="search" value={query} onChange={(e) => setQuery(e.target.value)}
                    placeholder={`Filter ${value.length} products…`} aria-label={`Filter ${label}`} className="h-8 pl-8 text-sm"
                  />
                </div>
                {query.trim() !== '' ? (
                  <p role="status" className="text-xs text-muted-foreground">
                    {value.filter((id) => matchesRecommendation(byId.get(id), query)).length} of {value.length} shown. Clear the filter to reorder.
                  </p>
                ) : null}
              </div>
              {addButton}
            </div>
          ) : null}
          <RecommendationRows value={value} byId={byId} label={label} disabled={!canManage} loading={metadataLoading} added={added} query={query} onChange={onChange} />
        </>
      ) : (
        <div className="flex flex-col items-center gap-3 rounded-md border border-dashed bg-muted/20 px-6 py-10 text-center">
          <div className="grid gap-1">
            <p className="text-sm font-medium">No {noun} yet</p>
            <p className="text-sm text-muted-foreground">{hint}</p>
          </div>
          {addButton}
        </div>
      )}

      {canManage ? (
        <ProductPickerDialog
          open={browsing}
          onOpenChange={setBrowsing}
          title={`Add to ${label}`}
          description={`${hint} Picks join the end of the list, and nothing is saved until you save the page.`}
          takenIds={linked}
          takenLabel="Linked"
          excludeIds={selfId}
          onConfirm={(picks) => {
            const additions = picks.map((p) => p.product).filter((p) => p.id !== productId && !linked.has(p.id))
            onPick(additions)
            onChange([...value, ...additions.map((p) => p.id)])
          }}
        />
      ) : null}
    </div>
  )
}
