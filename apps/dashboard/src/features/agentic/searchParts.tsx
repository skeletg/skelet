import type { FormEvent, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { SearchIcon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Empty, Panel } from '@/components/layout/panels'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useConfigScope } from '@/lib/config-scope'
import { formatCents } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { EmbeddingCoverageQuery, SearchPreviewProductFieldsFragment } from './HybridSearch.gql'

type Coverage = EmbeddingCoverageQuery['embeddingCoverage']
type Product = SearchPreviewProductFieldsFragment
type Near = { distance: number; counted: boolean; product: Product }

const MAX_QUERY = 500

// What search by meaning has to work with - quiet under the question, with the one thing to fix when there is one
export function CoverageLine({ c }: { c: Coverage }) {
  if (!c.model) {
    return (
      <p className="text-muted-foreground">
        Search by meaning is off, so it searches by words alone. Set an embedding model under{' '}
        <Link to="/settings/ai/embeddings" className="text-foreground underline underline-offset-2">
          Settings, AI, Embeddings
        </Link>{' '}
        to find products and pages by what a shopper means.
      </p>
    )
  }
  return (
    <div className="space-y-1">
      <p className="flex flex-wrap items-center gap-x-4 gap-y-1 text-muted-foreground">
        <span>
          Search by meaning on <code className="text-xs text-foreground">{c.model}</code>
        </span>
        <span>
          <span className="tabular-nums text-foreground">{`${c.embeddedProducts.toLocaleString()} of ${c.products.toLocaleString()}`}</span> products
        </span>
        <span>
          <span className="tabular-nums text-foreground">{`${c.embeddedPages.toLocaleString()} of ${c.pages.toLocaleString()}`}</span> pages
        </span>
        {c.queued > 0 ? <span className="tabular-nums">{c.queued.toLocaleString()} waiting to be embedded</span> : null}
      </p>
      {c.failed ? <p className="text-warning">The last embedding run failed: {c.failed}</p> : null}
    </div>
  )
}

export function ProductLine({ p }: { p: Product }) {
  return (
    <div className="flex min-w-0 flex-1 items-center gap-3">
      <div className="min-w-0 flex-1">
        <a href={p.url} target="_blank" rel="noreferrer" className="block truncate text-sm hover:underline">
          {p.name}
        </a>
        <span className="text-xs text-muted-foreground">{p.sku}</span>
      </div>
      <span className="shrink-0 text-sm tabular-nums">{formatCents(p.price, p.currencyCode)}</span>
      {p.inStock ? null : <Badge variant="warning">Out of stock</Badge>}
    </div>
  )
}

export function Distance({ d, counted }: { d: number; counted: boolean }) {
  return (
    <span className="flex shrink-0 items-center gap-2">
      <span className="text-xs tabular-nums text-muted-foreground" title="Distance - smaller is closer">
        {d.toFixed(3)}
      </span>
      {counted ? <Badge variant="success">Counts</Badge> : <Badge variant="secondary">Too far</Badge>}
    </span>
  )
}

export function QueryForm({
  value,
  onChange,
  locale,
  onLocale,
  onSubmit,
  busy,
  label,
  placeholder,
  large,
  className,
  children,
}: {
  value: string
  onChange: (value: string) => void
  locale: string
  onLocale: (locale: string) => void
  onSubmit: () => void
  busy: boolean
  label: string
  placeholder: string
  large?: boolean
  className?: string
  children?: ReactNode
}) {
  const { availableLocales } = useConfigScope()
  const height = large ? 'h-11' : 'h-10'
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (value.trim()) onSubmit()
  }
  return (
    <form onSubmit={submit} className={cn('flex flex-wrap items-center gap-2', className)}>
      {children}
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        maxLength={MAX_QUERY}
        placeholder={placeholder}
        aria-label={label}
        className={cn(height, 'min-w-64 flex-1', large && 'bg-background/80 text-base md:text-base')}
      />
      {availableLocales.length > 1 ? (
        <Select value={locale} onValueChange={onLocale}>
          <SelectTrigger className={cn(height, 'w-24', large && 'bg-background/80')} aria-label="Language">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {availableLocales.map((l) => (
              <SelectItem key={l} value={l}>
                {l}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}
      <Button type="submit" size={large ? 'lg' : 'default'} className={height} disabled={!value.trim() || busy}>
        <SearchIcon className="size-4" />
        Search
      </Button>
    </form>
  )
}

export function MeaningFailed({ message, answers }: { message: string; answers: string }) {
  if (!message) return null
  return (
    <DetailPageSection>
      <Alert variant="warning">
        <AlertTitle>Search by meaning failed, so this is words alone - as {answers} would answer right now</AlertTitle>
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    </DetailPageSection>
  )
}

export function RankedList({ rows, empty }: { rows: { byWords: boolean; byMeaning: boolean; product: Product }[]; empty: string }) {
  if (rows.length === 0) return <Empty>{empty}</Empty>
  return (
    <ol className="divide-y rounded-md border">
      {rows.map((s, i) => (
        <li key={s.product.id} className="flex flex-wrap items-center gap-3 px-3 py-2">
          <span className="w-5 shrink-0 text-right text-xs tabular-nums text-muted-foreground">{i + 1}</span>
          <ProductLine p={s.product} />
          <span className="flex gap-1">
            {s.byWords ? <Badge variant="outline">Words</Badge> : null}
            {s.byMeaning ? <Badge variant="outline">Meaning</Badge> : null}
          </span>
        </li>
      ))}
    </ol>
  )
}

export function WordsPanel({ total, products }: { total: number; products: Product[] }) {
  return (
    <Panel title={`By words - ${total.toLocaleString()} matched`}>
      {products.length === 0 ? (
        <Empty>No product matches these words.</Empty>
      ) : (
        <ul className="divide-y">
          {products.map((p) => (
            <li key={p.id} className="flex items-center gap-3 py-2">
              <ProductLine p={p} />
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

export function MeaningPanel({ title = 'By meaning', near, unavailable, empty }: { title?: string; near: Near[]; unavailable?: string; empty: string }) {
  return (
    <Panel title={title}>
      {unavailable ? (
        <Empty>{unavailable}</Empty>
      ) : near.length === 0 ? (
        <Empty>{empty}</Empty>
      ) : (
        <ul className="divide-y">
          {near.map((n) => (
            <li key={n.product.id} className="flex items-center gap-3 py-2">
              <ProductLine p={n.product} />
              <Distance d={n.distance} counted={n.counted} />
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}
