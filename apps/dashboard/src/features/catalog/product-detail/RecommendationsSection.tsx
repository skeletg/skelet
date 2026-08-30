import { useCallback, useEffect, useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  Link2Icon,
  PlusIcon,
  SearchIcon,
  ShoppingCartIcon,
  TrendingUpIcon,
  XIcon,
  type LucideIcon,
} from 'lucide-react'
import { Input } from '@/components/ui/input'
import type { ProductLinkKind } from '@/graphql/schema.types'
import {
  ProductLinksDocument,
  ProductLabelsDocument,
  ProductPickerDocument,
  SetProductLinksDocument,
} from '../Catalog.gql'
import { useProductSaveSection, type SectionDiffEntry } from '../productSaveRegistry'
import { Section } from './shared'

// Curated product links: related (PDP "you may also like"), upsell (better alternatives), cross-sell (cart companions) - Order = curated
// display order. "Also bought" recommendations are automatic and appear alongside these on the storefront

const KINDS: { kind: ProductLinkKind; label: string; hint: string; Icon: LucideIcon }[] = [
  { kind: 'RELATED', label: 'Related', hint: 'Shown on the product page rail.', Icon: Link2Icon },
  { kind: 'UPSELL', label: 'Upsells', hint: 'Higher-value alternatives.', Icon: TrendingUpIcon },
  { kind: 'CROSSSELL', label: 'Cross-sells', hint: 'Companions surfaced in the cart.', Icon: ShoppingCartIcon },
]

type SlimProduct = { id: string; name?: string | null; sku?: string | null }

export function RecommendationsSection({
  productId,
  canManage,
}: {
  productId: string
  canManage: boolean
}) {
  // Labels for every id any kind currently links - fetched by id, so the picker never
  // needs the whole catalog in memory just to name a chip
  const [linkedIds, setLinkedIds] = useState<string[]>([])
  const labelsQuery = useQuery(ProductLabelsDocument, {
    variables: { ids: linkedIds },
    skip: linkedIds.length === 0,
  })
  const byId = useMemo(
    () => new Map(((labelsQuery.data?.productsByIds ?? []) as SlimProduct[]).map((p) => [p.id, p])),
    [labelsQuery.data],
  )
  const reportIds = useCallback((ids: string[]) => {
    setLinkedIds((prev) => {
      const merged = new Set(prev)
      let grew = false
      for (const id of ids) {
        if (!merged.has(id)) {
          merged.add(id)
          grew = true
        }
      }
      return grew ? [...merged] : prev
    })
  }, [])

  return (
    <Section
      title="Recommendations"
      description="Curated product links. Automatic 'also bought' suggestions fill in alongside these."
    >
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {KINDS.map(({ kind, label, hint, Icon }) => (
          <KindEditor
            key={kind}
            productId={productId}
            kind={kind}
            label={label}
            hint={hint}
            Icon={Icon}
            byId={byId}
            reportIds={reportIds}
            canManage={canManage}
          />
        ))}
      </div>
    </Section>
  )
}

function KindEditor({
  productId,
  kind,
  label,
  hint,
  Icon,
  byId,
  reportIds,
  canManage,
}: {
  productId: string
  kind: ProductLinkKind
  label: string
  hint: string
  Icon: LucideIcon
  byId: Map<string, SlimProduct>
  reportIds: (ids: string[]) => void
  canManage: boolean
}) {
  const { data } = useQuery(ProductLinksDocument, { variables: { productId, kind } })
  const original = useMemo(() => data?.productLinks ?? [], [data])
  const [ids, setIds] = useState<string[] | null>(null)
  const current = ids ?? original
  const [search, setSearch] = useState('')
  // Every id this kind touches flows up, so the shared label fetch covers all four panels in one query
  useEffect(() => {
    reportIds(current)
  }, [current, reportIds])
  const [setLinks] = useMutation(SetProductLinksDocument, {
    refetchQueries: [{ query: ProductLinksDocument, variables: { productId, kind } }],
  })

  const dirty = ids !== null && JSON.stringify(ids) !== JSON.stringify(original)
  const diff = useMemo<SectionDiffEntry[]>(() => {
    if (!dirty) return []
    const names = (list: string[]) =>
      list.map((id) => byId.get(id)?.name ?? byId.get(id)?.sku ?? id).join(', ') || '-'
    return [{ label: `${label} links`, before: names(original), after: names(current) }]
  }, [dirty, original, current, byId, label])

  const save = useCallback(async () => {
    if (!dirty) return
    await setLinks({ variables: { productId, kind, linkedProductIds: current } })
  }, [dirty, setLinks, productId, kind, current])

  useProductSaveSection({ id: `links-${kind.toLowerCase()}`, title: `${label} links`, diff, save })

  const debounced = useDebounced(search.trim(), 250)
  const pickerQuery = useQuery(ProductPickerDocument, {
    variables: { search: debounced, limit: 10 },
    skip: debounced === '', // the dropdown only opens once the operator types
  })
  const candidates = useMemo(() => {
    if (debounced === '') return []
    return ((pickerQuery.data?.productsPaged.items ?? []) as SlimProduct[])
      .filter((p) => p.id !== productId && !current.includes(p.id))
      .slice(0, 6)
  }, [debounced, pickerQuery.data, productId, current])

  return (
    <div className="flex flex-col gap-3 rounded-lg border p-4">
      <div className="flex items-start gap-2.5">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{label}</span>
            {current.length > 0 && (
              <span className="rounded-full bg-primary/10 px-1.5 py-0.5 text-[11px] font-medium tabular-nums text-primary">
                {current.length}
              </span>
            )}
          </div>
          <p className="text-xs text-muted-foreground">{hint}</p>
        </div>
      </div>

      {current.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {current.map((id) => (
            <span
              key={id}
              className="inline-flex max-w-full items-center gap-1 rounded-full border bg-muted/40 py-1 pl-2.5 pr-1 text-xs"
            >
              <span className="truncate">{byId.get(id)?.name ?? byId.get(id)?.sku ?? id}</span>
              <button
                type="button"
                className="rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
                onClick={() => setIds(current.filter((x) => x !== id))}
                disabled={!canManage}
                aria-label="Remove link"
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
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          disabled={!canManage}
          className="h-8 pl-8 text-sm"
        />
        {candidates.length > 0 && (
          <div className="absolute z-10 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
            {candidates.map((p) => (
              <button
                key={p.id}
                type="button"
                className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
                onClick={() => {
                  setIds([...current, p.id])
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
