import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import {
  ActivityIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  CircleOffIcon,
  LayersIcon,
  PackageIcon,
  StarIcon,
  TrendingUpIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useConfigScope } from '@/lib/config-scope'
import { useFeatures } from '@/shared/features/useFeatures'
import { formatCents } from '@/lib/format'
import { cn } from '@/lib/utils'
import { ProductSidebarStatsDocument } from '../Catalog.gql'
import { ProductTrafficDocument } from '../Catalog.gql'

const COLLAPSE_KEY = 'skelet-dash:product-sidebar-collapsed'
const WINDOW_DAYS = 30

type SidebarProduct = {
  id: string
  sku?: string | null
  kind: string
  url?: string | null
  variantMode?: string | null
  defaultPrice?: number | null
  defaultCurrencyCode?: string | null
  productType?: { name: string } | null
  // the fallback-y one: main image, else thumbnail, else the first gallery image - mainImage is the
  // literal role assignment and belongs to the media-role editor, not to a tile
  displayImage?: { url: string; altText: string; status: string } | null
  // Structure the parent query already loads - counted here rather than refetched
  children?: unknown[] | null
  bundleOptions?: unknown[] | null
  kitComponents?: unknown[] | null
  groupedMembers?: unknown[] | null
  downloads?: unknown[] | null
  subscriptionTerms?: { intervalUnit?: string | null; intervalCount?: number | null } | null
}

// What a kind is actually made of - a bundle's rows mean nothing on a simple product, and an
// empty structure section on every simple product is worse than no section
function structureRows(p: SidebarProduct): { label: string; value: React.ReactNode }[] {
  switch (p.kind) {
    case 'CONFIGURABLE':
      return [
        { label: 'Variants', value: (p.children?.length ?? 0).toLocaleString() },
        { label: 'Variant mode', value: p.variantMode ?? '-' },
      ]
    case 'BUNDLE':
      return [{ label: 'Bundle options', value: (p.bundleOptions?.length ?? 0).toLocaleString() }]
    case 'KIT':
      return [{ label: 'Components', value: (p.kitComponents?.length ?? 0).toLocaleString() }]
    case 'GROUPED':
      return [{ label: 'Members', value: (p.groupedMembers?.length ?? 0).toLocaleString() }]
    case 'SUBSCRIPTION': {
      const t = p.subscriptionTerms
      return [
        {
          label: 'Billing cycle',
          value: t?.intervalUnit ? `every ${t.intervalCount ?? 1} ${t.intervalUnit.toLowerCase()}` : '-',
        },
      ]
    }
    default:
      return []
  }
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-sm font-medium">{value}</span>
    </div>
  )
}

function Section({
  title,
  icon: Icon,
  children,
}: {
  title: string
  icon: React.ComponentType<{ className?: string }>
  children: React.ReactNode
}) {
  return (
    <section className="border-b px-4 py-3 last:border-b-0">
      <h3 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        <Icon className="size-3.5" />
        {title}
      </h3>
      {children}
    </section>
  )
}

export function ProductSidebar({ product }: { product: SidebarProduct }) {
  // Persisted, so the choice survives moving between products rather than resetting each time
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem(COLLAPSE_KEY) === '1',
  )
  const { effectiveChannelId } = useConfigScope()
  const { enabled: featureEnabled } = useFeatures()
  // Lens keys pages by path and needs one channel - a global scope has no single page to ask about
  const signalsOn = featureEnabled('signals') && !!product.url
  const trafficReady = signalsOn && !!effectiveChannelId

  const toggle = () => {
    setCollapsed((c) => {
      localStorage.setItem(COLLAPSE_KEY, c ? '0' : '1')
      return !c
    })
  }

  // errorPolicy 'all' so an admin without reports.view still gets stock and reviews rather than
  // an empty sidebar - the missing block simply doesn't render
  const { data, loading } = useQuery(ProductSidebarStatsDocument, {
    variables: {
      id: product.id,
      sku: product.sku ?? '',
      days: WINDOW_DAYS,
      channelId: effectiveChannelId ?? null,
    },
    skip: collapsed || !product.sku,
    errorPolicy: 'all',
  })

  const { data: traffic } = useQuery(ProductTrafficDocument, {
    variables: { channelId: effectiveChannelId ?? '', pageKey: `/${product.url ?? ''}` },
    skip: collapsed || !trafficReady,
    errorPolicy: 'all', // lens.view is a separate grant from catalog.view
  })

  if (collapsed) {
    return (
      <aside className="hidden h-full w-10 shrink-0 flex-col items-center border-l bg-muted/10 py-3 xl:flex">
        <Button variant="ghost" size="icon" className="size-8" onClick={toggle} aria-label="Expand product panel" aria-expanded={false}>
          <ChevronLeftIcon className="size-4" />
        </Button>
        <span className="mt-3 text-xs font-medium tracking-wider text-muted-foreground [writing-mode:vertical-rl]">
          Overview
        </span>
      </aside>
    )
  }

  const sales = data?.productSales
  const reviews = data?.reviewCounts
  const stock = data?.product
  const lens = traffic?.lensPageStats
  const rows = structureRows(product)

  return (
    <aside className="scrollbar-accent hidden h-full w-[20rem] shrink-0 flex-col overflow-y-auto border-l bg-muted/10 xl:flex 2xl:w-[22rem]">
      <div className="flex items-center justify-between border-b bg-background px-4 py-3">
        <div className="text-sm font-semibold">Overview</div>
        <Button variant="ghost" size="icon" className="size-7" onClick={toggle} aria-label="Collapse product panel" aria-expanded>
          <ChevronRightIcon className="size-4" />
        </Button>
      </div>

      <Section title="Product" icon={PackageIcon}>
        <div className="mb-3 flex items-center gap-3">
          {product.displayImage?.status === 'READY' ? (
            <img
              src={product.displayImage.url}
              alt={product.displayImage.altText || ''}
              className="size-14 shrink-0 rounded border object-cover"
            />
          ) : (
            <div className="size-14 shrink-0 rounded border bg-muted" aria-hidden />
          )}
          <div className="min-w-0">
            <div className="truncate font-mono text-xs text-muted-foreground">
              {product.sku || 'No SKU'}
            </div>
            <Badge variant="secondary" className="mt-1">
              {product.kind}
            </Badge>
          </div>
        </div>
        <Row label="Type" value={product.productType?.name ?? '-'} />
        <Row
          label="Price"
          value={
            product.defaultPrice != null
              ? formatCents(product.defaultPrice, product.defaultCurrencyCode ?? 'USD')
              : '-'
          }
        />
      </Section>

      <Section title="Stock" icon={PackageIcon}>
        {loading && !stock ? (
          <Skeleton className="h-5 w-24" />
        ) : (
          <>
            <Row
              label="Salable"
              value={stock?.salableQuantity != null ? stock.salableQuantity : 'Not tracked'}
            />
            <Row
              label="Availability"
              value={
                stock?.inStock ? (
                  <span className="text-success">In stock</span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-muted-foreground">
                    <CircleOffIcon className="size-3" /> Out of stock
                  </span>
                )
              }
            />
          </>
        )}
      </Section>

      {sales && (
        <Section title={`Sales · last ${sales.days} days`} icon={TrendingUpIcon}>
          {loading ? (
            <Skeleton className="h-5 w-24" />
          ) : sales.orderCount === 0 ? (
            <p className="text-sm text-muted-foreground">No orders in this window</p>
          ) : (
            <>
              <Row label="Units sold" value={sales.unitsSold.toLocaleString()} />
              <Row
                label="Revenue"
                value={formatCents(sales.revenue, product.defaultCurrencyCode ?? 'USD')}
              />
              <Row
                label="Margin"
                value={
                  <span className={cn(sales.margin < 0 && 'text-destructive')}>
                    {formatCents(sales.margin, product.defaultCurrencyCode ?? 'USD')}
                  </span>
                }
              />
              <Row label="Orders" value={sales.orderCount.toLocaleString()} />
            </>
          )}
        </Section>
      )}

      {rows.length > 0 && (
        <Section title="Structure" icon={LayersIcon}>
          {rows.map((r) => (
            <Row key={r.label} label={r.label} value={r.value} />
          ))}
        </Section>
      )}

      {(product.downloads?.length ?? 0) > 0 && (
        <Section title="Digital" icon={LayersIcon}>
          <Row label="Downloads" value={(product.downloads?.length ?? 0).toLocaleString()} />
        </Section>
      )}

      {signalsOn && (
        <Section title="Traffic · all time" icon={ActivityIcon}>
          {!trafficReady ? (
            // Hiding it entirely made the panel look like Signals wasn't wired up at all
            <p className="text-sm text-muted-foreground">Pick a channel to see page traffic</p>
          ) : !lens ? (
            <Skeleton className="h-5 w-24" />
          ) : lens.views === 0 ? (
            <p className="text-sm text-muted-foreground">No page views recorded</p>
          ) : (
            <>
              <Row label="Views" value={lens.views.toLocaleString()} />
              <Row label="Visitors" value={lens.visitors.toLocaleString()} />
              <Row label="Clicks" value={lens.clicks.toLocaleString()} />
              <Row label="Avg scroll" value={`${lens.avgScroll}%`} />
            </>
          )}
        </Section>
      )}

      {reviews && (
        <Section title="Reviews" icon={StarIcon}>
          {reviews.total === 0 ? (
            <p className="text-sm text-muted-foreground">No reviews yet</p>
          ) : (
            <>
              <Row label="Approved" value={reviews.approved.toLocaleString()} />
              <Row label="Pending" value={reviews.pending.toLocaleString()} />
              <Row label="Total" value={reviews.total.toLocaleString()} />
            </>
          )}
        </Section>
      )}
    </aside>
  )
}
