import { useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Link2Icon, PlusIcon, SearchIcon, ShoppingCartIcon, TrendingUpIcon, XIcon, type LucideIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { ProductLinksDocument, ProductLabelsDocument, ProductPickerDocument, SetProductLinksDocument } from '../Catalog.gql'
import {
  PRODUCT_LINK_KINDS,
  PRODUCT_LINK_LABELS,
  changedLinkKinds,
  productLinksDiff,
  productLinksSchema,
  type ProductLinksValues,
} from '../schemas/product'
import { Section } from './shared'

// Curated product links: related (PDP "you may also like"), upsell (better alternatives), cross-sell (cart companions) - Order = curated
// display order. "Also bought" recommendations are automatic and appear alongside these on the storefront

const KINDS: { key: keyof ProductLinksValues; label: string; hint: string; Icon: LucideIcon }[] = [
  { key: 'related', label: 'Related', hint: 'Shown on the product page rail.', Icon: Link2Icon },
  { key: 'upsell', label: 'Upsells', hint: 'Higher-value alternatives.', Icon: TrendingUpIcon },
  { key: 'crossSell', label: 'Cross-sells', hint: 'Companions surfaced in the cart.', Icon: ShoppingCartIcon },
]

type SlimProduct = { id: string; name?: string | null; sku?: string | null }

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
  const [setLinks] = useMutation(SetProductLinksDocument)

  // Names for the chips: every id linked now or before, fetched by id so the picker never needs the
  // catalog in memory - plus what was just picked, already known from the search result
  const [picked, setPicked] = useState<Map<string, SlimProduct>>(new Map())
  const [labelIds, setLabelIds] = useState<string[]>([])
  const labelsQuery = useQuery(ProductLabelsDocument, { variables: { ids: labelIds }, skip: labelIds.length === 0 })
  const labels = (labelsQuery.data ?? labelsQuery.previousData)?.productsByIds as SlimProduct[] | undefined
  const byId = useMemo(() => new Map([...(labels ?? []).map((p) => [p.id, p] as const), ...picked]), [labels, picked])
  const diff = useMemo(() => productLinksDiff((id) => byId.get(id)?.name ?? byId.get(id)?.sku ?? id), [byId])

  const { form, draft } = useSectionForm({
    id: 'links',
    title: 'Recommendations',
    baseline,
    schema: productLinksSchema,
    diff,
    routeKey: `product-links:${productId}`,
    enabled: loaded,
    submit: async (v) => {
      for (const k of changedLinkKinds(baseline, v)) {
        await setLinks({
          variables: { productId, kind: PRODUCT_LINK_KINDS[k], linkedProductIds: v[k] },
          refetchQueries: [{ query: ProductLinksDocument, variables: { productId, kind: PRODUCT_LINK_KINDS[k] } }],
        })
      }
    },
  })

  // The label fetch only grows, so a removed chip keeps its name if it comes back
  const values = useStore(form.store, (s) => s.values)
  const unnamed = [...baseline.related, ...baseline.upsell, ...baseline.crossSell, ...values.related, ...values.upsell, ...values.crossSell].filter(
    (id) => !labelIds.includes(id),
  )
  if (unnamed.length > 0) setLabelIds([...labelIds, ...new Set(unnamed)])

  return (
    <Section title="Recommendations" description="Curated product links. Automatic 'also bought' suggestions fill in alongside these.">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_LINK_LABELS} />}
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
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
                disabled={!canManage || !loaded}
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
}) {
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const pickerQuery = useQuery(ProductPickerDocument, {
    variables: { search: debounced, limit: 10 },
    skip: debounced === '', // the dropdown only opens once the operator types
  })
  const candidates = useMemo(() => {
    if (debounced === '') return []
    return ((pickerQuery.data?.productsPaged.items ?? []) as SlimProduct[]).filter((p) => p.id !== productId && !value.includes(p.id)).slice(0, 6)
  }, [debounced, pickerQuery.data, productId, value])
  const name = (id: string) => byId.get(id)?.name ?? byId.get(id)?.sku ?? id

  return (
    <div className="flex flex-col gap-3 rounded-lg border p-4">
      <div className="flex items-start gap-2.5">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{label}</span>
            {value.length > 0 && (
              <span className="rounded-full bg-muted px-1.5 py-0.5 text-[11px] font-medium tabular-nums text-muted-foreground">{value.length}</span>
            )}
          </div>
          <p className="text-xs text-muted-foreground">{hint}</p>
        </div>
      </div>

      {value.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {value.map((id) => (
            <span key={id} className="inline-flex max-w-full items-center gap-1 rounded-full border bg-muted/40 py-1 pl-2.5 pr-1 text-xs">
              <span className="truncate">{name(id)}</span>
              <button
                type="button"
                className="rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
                onClick={() => onChange(value.filter((x) => x !== id))}
                disabled={disabled}
                aria-label={`Remove ${name(id)} from ${label}`}
              >
                <XIcon className="size-3" />
              </button>
            </span>
          ))}
        </div>
      ) : (
        <p className="text-xs text-muted-foreground/70">Nothing linked yet.</p>
      )}

      <div className="relative mt-auto">
        <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          placeholder="Search products to add…"
          aria-label={`Add to ${label}`}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          disabled={disabled}
          className="h-8 pl-8 text-sm"
        />
        {candidates.length > 0 && (
          <div className="absolute z-10 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
            {candidates.map((p) => (
              <button
                key={p.id}
                type="button"
                className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
                onClick={() => {
                  onPick(p)
                  onChange([...value, p.id])
                  setSearch('')
                }}
              >
                <PlusIcon className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{p.name ?? p.sku ?? ''}</span>
                {p.sku && <code className="shrink-0 font-mono text-[10px] text-muted-foreground">{p.sku}</code>}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
