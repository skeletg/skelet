import { useState } from 'react'
import { Link } from 'react-router-dom'
import { type LucideIcon, ActivityIcon, GlobeIcon, MailIcon, PackageIcon, ShoppingBagIcon, UserIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { DetailPageSection } from '@/components/layout/detail-page'
import { eventLabel } from './eventLabel'
import { Tile } from '@/components/layout/panels'
import { formatCents, formatDate, formatDateTime } from '@/lib/format'

// Shared behavioural-profile renderer for Synapse: KPI tiles + a by-event
// breakdown + the activity timeline (List ↔ visual day-grouped rail toggle)
// Used by both the per-customer (SynapseCustomer) and per-anonymous-visitor
// (SynapseVisitor) pages - they fetch the same CustomerProfile shape and hand
// it here

export type ProfileData = {
  firstSeen?: string | null
  lastSeen?: string | null
  totalEvents: number
  pageViews: number
  conversions: number
  revenue: { currency: string; cents: number; orders: number }[]
  counts: { name: string; count: number }[]
  timeline: { eventName: string; occurredAt: string; source: string; summary?: string | null; detailsJson: string }[]
  devices: { value: string; count: number }[]
  browsers: { value: string; count: number }[]
  referrers: { value: string; count: number }[]
  ipAddresses: { value: string; count: number }[]
  operatingSystems: { value: string; count: number }[]
  screens: { value: string; count: number }[]
  countries: { value: string; count: number }[]
  sources: { value: string; count: number }[]
  campaigns: { value: string; count: number }[]
  topPages: { title: string; pageKey: string; views: number; dwellSeconds: number }[]
  pagesViewedTotal: number
  topProducts: { name: string; image: string; price: string; sku: string; views: number }[]
  searches: { value: string; count: number }[]
  sessions: number
  // Persisted Synapse traits (customers only; null for anonymous visitors and
  // customers not yet covered by a trait-compute run)
  traits?: {
    lifecycle: string
    rfmRecency: number
    rfmFrequency: number
    rfmMonetary: number
    recencyDays: number
    frequency: number
    monetaryCents: number
    engagementScore: number
    churnRisk: number
    predictedClvCents: number
    nextOrderDays: number
  } | null
  experiments?: { experiment: string; variation: string; lastExposedAt: string }[]
  // Attribute affinities (Synapse-tracked attributes the profile filtered/viewed)
  affinities?: { slug: string; name: string; values: { value: string; count: number }[] }[]
}

const LIFECYCLE_STYLE: Record<string, string> = {
  new: 'bg-info-soft text-info border-info/30',
  active: 'bg-success-soft text-success border-success/30',
  at_risk: 'bg-warning-soft text-warning border-warning/30',
  lapsed: 'bg-destructive-soft text-destructive border-destructive/30',
  prospect: 'bg-muted text-muted-foreground border-border',
}

const SOURCE_LABELS: Record<string, string> = {
  web: 'Web',
  email: 'Email',
  campaign: 'Email',
  flow: 'Email',
  transactional: 'Email',
  commerce: 'Commerce',
  account: 'Account',
}
const sourceLabel = (s: string): string => SOURCE_LABELS[s] ?? s

// sourceStyle gives each channel a stable color + icon for the visual rail
// Class strings are full literals (never interpolated) so they survive purging
function sourceStyle(s: string): { dot: string; fg: string; Icon: LucideIcon } {
  switch (s) {
    case 'web':
      return { dot: 'bg-info', fg: 'text-info-foreground', Icon: GlobeIcon }
    case 'commerce':
      return { dot: 'bg-success', fg: 'text-success-foreground', Icon: ShoppingBagIcon }
    case 'account':
      return { dot: 'bg-muted-foreground', fg: 'text-background', Icon: UserIcon }
    case 'email':
    case 'campaign':
    case 'flow':
    case 'transactional':
      return { dot: 'bg-muted-foreground', fg: 'text-background', Icon: MailIcon }
    default:
      return { dot: 'bg-primary', fg: 'text-primary-foreground', Icon: ActivityIcon }
  }
}

function fmtTime(v?: string | null): string {
  if (!v) return '-'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}
type TimelineItem = ProfileData['timeline'][number]
type View = 'sessions' | 'channels' | 'story' | 'rhythm' | 'list'

// centsToMoney formats a cents value (number or numeric string) as currency
function centsToMoney(v: unknown, currency?: string): string | undefined {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number.parseInt(v, 10) : NaN
  if (!Number.isFinite(n)) return undefined
  return (n / 100).toLocaleString(undefined, { style: 'currency', currency: currency || 'USD' })
}

type Detail = { primary: string; sub?: string; image?: string }

// detailFor turns an event's raw properties into the rich row content: a primary line (the thing the event is about), a
// secondary detail line, and an optional thumbnail (product image for product views)
function detailFor(item: TimelineItem): Detail {
  let d: Record<string, unknown> = {}
  try {
    if (item.detailsJson) d = JSON.parse(item.detailsJson) as Record<string, unknown>
  } catch {
    // malformed props - fall back to the summary below
  }
  const str = (k: string): string | undefined => (typeof d[k] === 'string' ? (d[k] as string) : undefined)
  const join = (...xs: (string | undefined)[]): string | undefined => xs.filter(Boolean).join(' · ') || undefined
  const n = item.eventName

  if (n === 'page_view' && d.entity === 'product') {
    return {
      primary: str('entity_name') || item.summary || 'Product',
      sub: join('Viewed product', centsToMoney(str('entity_price')), str('sku') ? `SKU ${str('sku')}` : undefined),
      image: str('entity_image') || undefined,
    }
  }
  if (n === 'page_view') {
    return { primary: str('title') || item.summary || 'Page view', sub: join('Page view', str('page_key')) }
  }
  if (n === 'cart_add') {
    return { primary: 'Added to cart', sub: join(str('sku'), d.qty ? `× ${d.qty}` : undefined, centsToMoney(d.unit_price)) }
  }
  if (n === 'product_filter') return { primary: 'Filtered', sub: str('filters') }
  if (n === 'order_placed') {
    return { primary: join('Order placed', str('order_number')) ?? 'Order placed', sub: centsToMoney(d.total_cents, str('currency')) }
  }
  if (n === 'giftcard_redeemed') {
    return { primary: 'Gift card redeemed', sub: join(str('order_number'), centsToMoney(d.amount_cents, str('currency'))) }
  }
  if (n === 'review_submitted') return { primary: 'Review submitted', sub: str('status') }
  if (n.startsWith('order_')) return { primary: eventLabel(n), sub: str('order_number') }
  // Everything else: the human label + the best-effort summary
  return { primary: eventLabel(n), sub: item.summary || undefined }
}

export function ActivityProfile({
  profile: p,
  emptyText,
  kind = 'visitor',
}: {
  profile: ProfileData
  emptyText?: string
  // 'customer' shows a "traits not computed yet" hint when traits are absent
  // (RFM is purchase-based + materialized); 'visitor' (anonymous) hides it
  kind?: 'customer' | 'visitor'
}) {
  const [view, setView] = useState<View>('sessions')
  if (p.totalEvents === 0) {
    return (
      <DetailPageSection title="Activity">
        <p className="text-sm text-muted-foreground">{emptyText ?? 'No tracked activity yet in this window.'}</p>
      </DetailPageSection>
    )
  }
  const maxCount = Math.max(1, ...p.counts.map((c) => c.count))
  const timeline = p.timeline
  const context = contextRows(p)
  return (
    <>
      <DetailPageSection title="Overview">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <Tile label="First seen" value={formatDate(p.firstSeen)} />
          <Tile label="Last seen" value={formatDate(p.lastSeen)} />
          <Tile label="Visits" value={p.sessions.toLocaleString()} sub="distinct sessions" />
          <Tile label="Total events" value={p.totalEvents.toLocaleString()} />
          <Tile
            label="Page views"
            value={p.pageViews.toLocaleString()}
            sub={
              p.conversions > 0
                ? `${p.conversions} conversion${p.conversions === 1 ? '' : 's'}${p.revenue.length ? ' · ' + p.revenue.map((r) => formatCents(r.cents, r.currency)).join(', ') : ''}`
                : 'no conversions'
            }
          />
        </div>
      </DetailPageSection>

      {p.traits ? (
        <DetailPageSection title="Traits" trailing={<LifecycleBadge lifecycle={p.traits.lifecycle} />}>
          <TraitsBody t={p.traits} currency={p.revenue[0]?.currency ?? 'USD'} />
        </DetailPageSection>
      ) : kind === 'customer' ? (
        <DetailPageSection title="Traits">
          <p className="text-sm text-muted-foreground">
            No traits computed yet. RFM (recency / frequency / monetary), lifecycle, and engagement are calculated for
            customers <span className="font-medium">after their first order</span> and refreshed every 12h
            (the <code className="text-xs">trait-compute</code> job - run it on demand from Scheduled jobs).
          </p>
        </DetailPageSection>
      ) : null}

      {p.affinities && p.affinities.length > 0 ? (
        <DetailPageSection
          title="Interests"
          description="What they filter by and view - the strongest signal of what they're shopping for."
        >
          <AffinitiesBody affinities={p.affinities} />
        </DetailPageSection>
      ) : null}

      {p.topProducts.length > 0 ? (
        <DetailPageSection title="Products viewed">
          <ProductsBody products={p.topProducts} />
        </DetailPageSection>
      ) : null}

      {p.topPages.length > 0 ? (
        <DetailPageSection
          title="Pages viewed"
          description={
            p.pagesViewedTotal > p.topPages.length
              ? `Most-viewed pages - top ${p.topPages.length} of ${p.pagesViewedTotal.toLocaleString()} distinct pages. View count + avg time; open any page's Lens heatmap.`
              : "View count + average time on page. Open any page's Lens heatmap."
          }
        >
          <PagesBody pages={p.topPages} total={p.pagesViewedTotal} />
        </DetailPageSection>
      ) : null}

      {p.searches.length > 0 ? (
        <DetailPageSection title="Searches">
          <SearchesBody searches={p.searches} />
        </DetailPageSection>
      ) : null}

      {context.length > 0 ? (
        <DetailPageSection title="Context" description="Device, location & acquisition - the top value seen for each.">
          <ContextBody rows={context} />
        </DetailPageSection>
      ) : null}

      {p.experiments && p.experiments.length > 0 ? (
        <DetailPageSection title="Experiments">
          <ExperimentsBody arms={p.experiments} />
        </DetailPageSection>
      ) : null}

      <DetailPageSection title="Activity" trailing={<Segmented view={view} onChange={setView} />}>
        {/* Event-frequency summary - a compact full-width strip, not a column */}
        <div className="flex flex-wrap gap-1.5">
          {p.counts.map((c) => (
            <span key={c.name} className="relative inline-flex items-center gap-1.5 overflow-hidden rounded-full border px-2.5 py-1 text-xs">
              <span className="absolute inset-y-0 left-0 bg-primary/10" style={{ width: `${Math.round((c.count / maxCount) * 100)}%` }} aria-hidden />
              <span className="relative text-muted-foreground">{eventLabel(c.name)}</span>
              <span className="relative font-semibold tabular-nums">{c.count}</span>
            </span>
          ))}
        </div>

        <div className="min-w-0">
          {timeline.length === 0 ? (
            <p className="text-sm text-muted-foreground">No events.</p>
          ) : view === 'sessions' ? (
            <SessionTimeline items={timeline} />
          ) : view === 'channels' ? (
            <ChannelsView items={timeline} />
          ) : view === 'story' ? (
            <StoryView items={timeline} />
          ) : view === 'rhythm' ? (
            <RhythmView items={timeline} />
          ) : (
            <CompactList items={timeline} />
          )}
          {timeline.length > 0 && (view === 'sessions' || view === 'channels' || view === 'story') ? <Legend className="mt-4" /> : null}
        </div>
      </DetailPageSection>
    </>
  )
}

// LifecycleBadge - the coloured lifecycle pill shown in the Traits section header
function LifecycleBadge({ lifecycle }: { lifecycle: string }) {
  return (
    <span className={cn('rounded-full border px-2 py-0.5 text-xs font-medium capitalize', LIFECYCLE_STYLE[lifecycle] ?? LIFECYCLE_STYLE.prospect)}>
      {lifecycle.replace('_', ' ')}
    </span>
  )
}

// TraitsBody - the persisted Synapse RFM / engagement summary that also drives
// shared trait segments (Pulse / promotions / Prism targeting)
function TraitsBody({ t, currency }: { t: NonNullable<ProfileData['traits']>; currency: string }) {
  const recency = t.recencyDays < 0 ? 'never' : `${t.recencyDays}d ago`
  const money = (t.monetaryCents / 100).toLocaleString(undefined, { style: 'currency', currency: currency || 'USD' })
  const clv = (t.predictedClvCents / 100).toLocaleString(undefined, { style: 'currency', currency: currency || 'USD' })
  // Churn styling flips at the same thresholds the lifecycle stages use
  const churnColor = t.churnRisk >= 70 ? 'bg-destructive' : t.churnRisk >= 40 ? 'bg-warning' : 'bg-success'
  return (
    <div className="grid gap-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <RfmCell label="Recency" score={t.rfmRecency} sub={recency} />
        <RfmCell label="Frequency" score={t.rfmFrequency} sub={`${t.frequency} order${t.frequency === 1 ? '' : 's'}`} />
        <RfmCell label="Monetary" score={t.rfmMonetary} sub={money} />
        <div className="grid content-start gap-1">
          <span className="text-xs text-muted-foreground">Engagement</span>
          <span className="h-1.5 overflow-hidden rounded-full bg-muted">
            <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.min(100, Math.max(0, t.engagementScore))}%` }} />
          </span>
          <span className="text-xs tabular-nums text-muted-foreground">{t.engagementScore}/100 · last 30d</span>
        </div>
      </div>
      {/* Predictive layer - churn vs their own cadence, expected 12-month value */}
      <div className="grid gap-3 border-t pt-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="grid content-start gap-1">
          <span className="text-xs text-muted-foreground">Churn risk</span>
          <span className="h-1.5 overflow-hidden rounded-full bg-muted">
            <span className={`block h-full rounded-full ${churnColor}`} style={{ width: `${Math.min(100, Math.max(0, t.churnRisk))}%` }} />
          </span>
          <span className="text-xs tabular-nums text-muted-foreground">
            {t.churnRisk}/100 · vs their own cadence
          </span>
        </div>
        <div className="grid content-start gap-1">
          <span className="text-xs text-muted-foreground">Predicted value (12 mo)</span>
          <span className="text-sm font-medium tabular-nums">{clv}</span>
        </div>
        <div className="grid content-start gap-1">
          <span className="text-xs text-muted-foreground">Next order expected</span>
          <span className="text-sm font-medium tabular-nums">
            {t.nextOrderDays < 0 ? '-' : t.nextOrderDays === 0 ? 'overdue' : `in ~${t.nextOrderDays}d`}
          </span>
          {t.nextOrderDays < 0 && (
            <span className="text-xs text-muted-foreground">needs 2+ orders</span>
          )}
        </div>
      </div>
    </div>
  )
}

// RfmCell shows one RFM dimension: its 1–5 quintile score (dots) + a raw value
function RfmCell({ label, score, sub }: { label: string; score: number; sub: string }) {
  return (
    <div className="grid gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="flex items-center gap-1">
        {[1, 2, 3, 4, 5].map((n) => (
          <span key={n} className={cn('size-2.5 rounded-full', n <= score ? 'bg-primary' : 'bg-muted')} />
        ))}
        <span className="ml-1 text-xs font-medium tabular-nums">{score > 0 ? score : '-'}</span>
      </span>
      <span className="text-xs tabular-nums text-muted-foreground">{sub}</span>
    </div>
  )
}

// AffinitiesBody - what this profile is interested in (Synapse-tracked attributes they filtered by / viewed), one card
// per attribute - The top value is the strongest signal (e.g. tire size 205/55R16 → "they likely need this size")
function AffinitiesBody({ affinities }: { affinities: NonNullable<ProfileData['affinities']> }) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {affinities.map((a) => {
        const total = a.values.reduce((s, v) => s + v.count, 0) || 1
        const top = a.values[0]
        return (
          <div key={a.slug} className="rounded-lg border p-3">
            <div className="mb-3 flex items-center justify-between gap-2">
              <span className="text-sm font-medium">{a.name}</span>
              {top ? (
                <span className="inline-flex items-center gap-1 rounded-full bg-success-soft px-2 py-0.5 text-xs">
                  <span className="text-muted-foreground">likely</span>
                  <span className="font-medium">{top.value}</span>
                  <span className="font-semibold tabular-nums text-success">{Math.round((top.count / total) * 100)}%</span>
                </span>
              ) : null}
            </div>
            <ul className="space-y-2">
              {a.values.map((v, i) => {
                const pct = Math.round((v.count / total) * 100)
                return (
                  <li key={v.value} className="grid gap-1 text-sm">
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate font-medium" title={v.value}>{v.value}</span>
                      <span className="shrink-0 tabular-nums text-muted-foreground">{pct}% · {v.count}</span>
                    </div>
                    <span className="block h-1.5 overflow-hidden rounded-full bg-muted">
                      <span
                        className={cn('block h-full rounded-full', i === 0 ? 'bg-success' : 'bg-primary')}
                        style={{ width: `${Math.max(3, pct)}%` }}
                      />
                    </span>
                  </li>
                )
              })}
            </ul>
          </div>
        )
      })}
    </div>
  )
}

// ExperimentsBody - the Prism A/B arms this customer is in (Prism → Synapse)
function ExperimentsBody({ arms }: { arms: NonNullable<ProfileData['experiments']> }) {
  return (
    <ul className="divide-y rounded-lg border">
      {arms.map((a) => (
        <li key={`${a.experiment}:${a.variation}`} className="flex items-center justify-between gap-2 px-3 py-2 text-sm">
          <span className="font-mono text-xs">{a.experiment}</span>
          <span className="flex items-center gap-2">
            <span className="rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">{a.variation}</span>
            <span className="text-xs text-muted-foreground">{formatDate(a.lastExposedAt)}</span>
          </span>
        </li>
      ))}
    </ul>
  )
}

function fmtDuration(s: number): string {
  if (s <= 0) return '-'
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return rem ? `${m}m ${rem}s` : `${m}m`
}

// ProductsBody - products the profile viewed, as visual cards (image, price,
// view count) - The strongest signal of what they're shopping for
function ProductsBody({ products }: { products: ProfileData['topProducts'] }) {
  return (
    <ul className="grid gap-2 sm:grid-cols-2">
      {products.map((pr) => (
        <li key={pr.sku || pr.name} className="flex items-center gap-3 rounded-lg border p-2">
          {pr.image ? (
            <img src={pr.image} alt="" className="size-12 shrink-0 rounded border object-cover" loading="lazy" />
          ) : (
            <span className="grid size-12 shrink-0 place-items-center rounded border bg-muted text-muted-foreground">
              <PackageIcon className="size-5" />
            </span>
          )}
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium" title={pr.name}>{pr.name}</div>
            <div className="truncate text-xs text-muted-foreground">
              {pr.price ? centsToMoney(pr.price) : null}
              {pr.sku ? <span className="ml-1.5 font-mono">{pr.sku}</span> : null}
            </div>
          </div>
          <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-xs tabular-nums text-muted-foreground">{pr.views}×</span>
        </li>
      ))}
    </ul>
  )
}

// PagesBody - pages the profile viewed, with view count + avg dwell, each
// linking to its Lens heatmap (their on-page behaviour)
function PagesBody({ pages, total }: { pages: ProfileData['topPages']; total: number }) {
  const [expanded, setExpanded] = useState(false)
  const COLLAPSED = 6
  const shown = expanded ? pages : pages.slice(0, COLLAPSED)
  const moreNotShown = Math.max(0, total - pages.length) // distinct pages beyond the fetched top slice
  return (
    <div className="space-y-2">
      <ul className="grid gap-2 sm:grid-cols-2">
        {shown.map((pg) => (
          <li key={pg.pageKey || pg.title} className="flex items-center justify-between gap-3 rounded-lg border p-2 text-sm">
            <div className="min-w-0">
              <Link
                to={`/lens/heatmap?page=${encodeURIComponent(pg.pageKey || '/')}&viewport=desktop`}
                className="block truncate font-medium hover:underline"
                title={`${pg.title} - open Lens heatmap`}
              >
                {pg.title}
              </Link>
              <span className="block truncate font-mono text-[11px] text-muted-foreground">{pg.pageKey || '/'}</span>
            </div>
            <div className="shrink-0 text-right text-xs text-muted-foreground">
              <div className="tabular-nums">{pg.views}×</div>
              {pg.dwellSeconds > 0 ? <div className="tabular-nums">{fmtDuration(pg.dwellSeconds)} avg</div> : null}
            </div>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap items-center gap-x-3 text-xs">
        {pages.length > COLLAPSED ? (
          <button type="button" onClick={() => setExpanded((v) => !v)} className="font-medium text-primary hover:underline">
            {expanded ? 'Show less' : `Show all ${pages.length}`}
          </button>
        ) : null}
        {moreNotShown > 0 ? (
          <span className="text-muted-foreground">+{moreNotShown.toLocaleString()} more pages with fewer views</span>
        ) : null}
      </div>
    </div>
  )
}

// SearchesBody - what the profile searched for, as chips with hit counts
function SearchesBody({ searches }: { searches: ProfileData['searches'] }) {
  return (
    <ul className="flex flex-wrap gap-2">
      {searches.map((s) => (
        <li key={s.value} className="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-sm">
          <span className="truncate">{s.value}</span>
          <span className="text-xs tabular-nums text-muted-foreground">{s.count}</span>
        </li>
      ))}
    </ul>
  )
}

// contextRows collapses the context dimensions to their single top value (a profile is usually one device, so the
// most-frequent value of each dimension is "the" value) - Returns only dimensions that have a value
function contextRows(p: ProfileData): { label: string; value: string; cap?: boolean }[] {
  const top = (xs: { value: string; count: number }[]) => xs[0]?.value ?? ''
  return [
    { label: 'Device', value: top(p.devices), cap: true },
    { label: 'OS', value: top(p.operatingSystems) },
    { label: 'Browser', value: top(p.browsers), cap: true },
    { label: 'Screen', value: top(p.screens), cap: true },
    { label: 'Location', value: top(p.countries) },
    { label: 'IP', value: top(p.ipAddresses) },
    { label: 'Came from', value: top(p.referrers) },
    { label: 'Source', value: top(p.sources), cap: true },
    { label: 'Campaign', value: top(p.campaigns) },
  ].filter((r) => r.value)
}

// ContextBody - device / OS / browser / screen / location / IP / acquisition as
// a grid of small labelled stat cells
function ContextBody({ rows }: { rows: { label: string; value: string; cap?: boolean }[] }) {
  return (
    <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {rows.map((r) => (
        <div key={r.label} className="rounded-lg border p-3">
          <dt className="text-xs uppercase tracking-wider text-muted-foreground">{r.label}</dt>
          <dd className={cn('mt-0.5 truncate text-sm font-medium', r.cap && 'capitalize')} title={r.value}>{r.value}</dd>
        </div>
      ))}
    </dl>
  )
}

function Segmented({ view, onChange }: { view: View; onChange: (v: View) => void }) {
  const opts: Array<{ v: View; label: string }> = [
    { v: 'sessions', label: 'Sessions' },
    { v: 'channels', label: 'Channels' },
    { v: 'story', label: 'Story' },
    { v: 'rhythm', label: 'Rhythm' },
    { v: 'list', label: 'List' },
  ]
  return (
    <div className="inline-flex rounded-md border p-0.5">
      {opts.map((o) => (
        <button
          key={o.v}
          type="button"
          onClick={() => onChange(o.v)}
          className={cn(
            'rounded px-2 py-0.5 text-xs',
            view === o.v ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

// buildSessions groups the (newest-first) timeline into visits: a gap of more than 30 minutes between consecutive events starts a new
// session - Each returned group keeps the source order (newest-first); the card renders it chronologically
function buildSessions(items: TimelineItem[]): TimelineItem[][] {
  const GAP = 30 * 60 * 1000
  const sessions: TimelineItem[][] = []
  let cur: TimelineItem[] = []
  for (const e of items) {
    if (cur.length === 0) {
      cur = [e]
      continue
    }
    const prev = new Date(cur[cur.length - 1].occurredAt).getTime()
    const t = new Date(e.occurredAt).getTime()
    if (Math.abs(prev - t) <= GAP) cur.push(e)
    else {
      sessions.push(cur)
      cur = [e]
    }
  }
  if (cur.length) sessions.push(cur)
  return sessions
}

// SessionTimeline groups the visitor's events into visits (sessions) and renders each as a card - the visit's date/duration/size header
// over its chronological event sequence - so the stream reads as discrete trips, not a flat log
function SessionTimeline({ items }: { items: TimelineItem[] }) {
  const sessions = buildSessions(items)
  return (
    <ol className="space-y-3">
      {sessions.map((evts, i) => (
        <SessionCard key={i} events={evts} />
      ))}
    </ol>
  )
}

function SessionCard({ events }: { events: TimelineItem[] }) {
  const asc = [...events].reverse() // chronological within the visit
  const startT = new Date(asc[0].occurredAt).getTime()
  const endT = new Date(asc[asc.length - 1].occurredAt).getTime()
  const durSec = Math.max(0, Math.round((endT - startT) / 1000))
  return (
    <li className="overflow-hidden rounded-lg border">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 border-b bg-muted/30 px-3 py-2 text-xs">
        <span className="font-medium text-foreground">{formatDate(asc[0].occurredAt)}</span>
        <span className="text-muted-foreground">{fmtTime(asc[0].occurredAt)}</span>
        {durSec >= 60 ? <span className="text-muted-foreground">· {fmtDuration(durSec)}</span> : null}
        <span className="ml-auto text-muted-foreground tabular-nums">
          {asc.length} event{asc.length === 1 ? '' : 's'}
        </span>
      </div>
      <ol className="divide-y">
        {asc.map((e, i) => (
          <SessionRow key={i} e={e} />
        ))}
      </ol>
    </li>
  )
}

function SessionRow({ e }: { e: TimelineItem }) {
  const s = sourceStyle(e.source)
  const Icon = s.Icon
  const det = detailFor(e)
  return (
    <li className="flex items-start gap-3 px-3 py-2">
      <span className={cn('mt-0.5 grid size-6 shrink-0 place-items-center rounded-full', s.dot, s.fg)}>
        <Icon className="size-3" />
      </span>
      {det.image ? <img src={det.image} alt="" className="size-9 shrink-0 rounded border object-cover" loading="lazy" /> : null}
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{det.primary}</div>
        {det.sub ? <div className="truncate text-xs text-muted-foreground">{det.sub}</div> : null}
      </div>
      <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{fmtTime(e.occurredAt)}</span>
    </li>
  )
}

function CompactList({ items }: { items: TimelineItem[] }) {
  return (
    <ol className="space-y-3">
      {items.map((e, i) => {
        const det = detailFor(e)
        return (
          <li key={i} className="grid grid-cols-[auto_1fr] gap-3 text-sm">
            <div className="mt-1 size-2 rounded-full bg-primary/60" />
            <div className="min-w-0">
              <div className="truncate font-medium">
                {det.primary}
                {det.sub ? <span className="font-normal text-muted-foreground"> · {det.sub}</span> : null}
              </div>
              <div className="text-xs text-muted-foreground">
                {sourceLabel(e.source)} · {formatDateTime(e.occurredAt)}
              </div>
            </div>
          </li>
        )
      })}
    </ol>
  )
}

// sourceGroup collapses the spine sources into the four channel lanes (every
// email-ish source - campaign/flow/transactional - folds into "email")
function sourceGroup(s: string): 'web' | 'email' | 'commerce' | 'account' {
  if (s === 'web') return 'web'
  if (s === 'commerce') return 'commerce'
  if (s === 'account') return 'account'
  return 'email'
}
const LANES: Array<{ key: 'web' | 'email' | 'commerce' | 'account'; label: string }> = [
  { key: 'web', label: 'Web' },
  { key: 'email', label: 'Email' },
  { key: 'commerce', label: 'Commerce' },
  { key: 'account', label: 'Account' },
]

// ChannelsView - cross-channel swimlanes: one lane per source, events plotted by
// time across the visitor's whole history - Shows the interplay + bursts/gaps
function ChannelsView({ items }: { items: TimelineItem[] }) {
  const times = items.map((e) => new Date(e.occurredAt).getTime()).filter((t) => !Number.isNaN(t))
  const min = Math.min(...times)
  const max = Math.max(...times)
  const span = Math.max(1, max - min)
  const lanes = LANES.filter((l) => items.some((e) => sourceGroup(e.source) === l.key))
  return (
    <div>
      <div className="space-y-2">
        {lanes.map((l) => (
          <div key={l.key} className="flex items-center gap-2">
            <span className="w-20 shrink-0 text-xs text-muted-foreground">{l.label}</span>
            <div className="relative h-7 flex-1 rounded bg-muted/30">
              {items
                .filter((e) => sourceGroup(e.source) === l.key)
                .map((e, i) => {
                  const left = ((new Date(e.occurredAt).getTime() - min) / span) * 100
                  const det = detailFor(e)
                  return (
                    <span
                      key={i}
                      title={`${det.primary} · ${formatDateTime(e.occurredAt)}`}
                      className={cn('absolute top-1/2 size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full ring-2 ring-background', sourceStyle(e.source).dot)}
                      style={{ left: `${left}%` }}
                    />
                  )
                })}
            </div>
          </div>
        ))}
      </div>
      <div className="mt-1.5 flex justify-between pl-[88px] text-[11px] text-muted-foreground">
        <span>{formatDate(new Date(min).toISOString())}</span>
        <span>{formatDate(new Date(max).toISOString())}</span>
      </div>
    </div>
  )
}

// StoryView - a richer day-grouped feed: bigger thumbnails + the channel label on
// each row, for skimming the narrative
function StoryView({ items }: { items: TimelineItem[] }) {
  const groups: { day: string; events: TimelineItem[] }[] = []
  for (const e of items) {
    const day = formatDate(e.occurredAt)
    const last = groups[groups.length - 1]
    if (last && last.day === day) last.events.push(e)
    else groups.push({ day, events: [e] })
  }
  return (
    <div className="space-y-4">
      {groups.map((g, gi) => (
        <div key={gi}>
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{g.day}</div>
          <ol className="space-y-2">
            {g.events.map((e, i) => {
              const s = sourceStyle(e.source)
              const Icon = s.Icon
              const det = detailFor(e)
              return (
                <li key={i} className="flex items-start gap-3 rounded-lg border p-2">
                  <span className={cn('mt-0.5 grid size-7 shrink-0 place-items-center rounded-full', s.dot, s.fg)}>
                    <Icon className="size-3.5" />
                  </span>
                  {det.image ? <img src={det.image} alt="" className="size-12 shrink-0 rounded border object-cover" loading="lazy" /> : null}
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{det.primary}</div>
                    {det.sub ? <div className="truncate text-xs text-muted-foreground">{det.sub}</div> : null}
                    <div className="mt-0.5 text-[11px] text-muted-foreground/70">
                      {sourceLabel(e.source)} · {fmtTime(e.occurredAt)}
                    </div>
                  </div>
                </li>
              )
            })}
          </ol>
        </div>
      ))}
    </div>
  )
}

const DOW = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const RHYTHM_PARTS = ['Morning', 'Afternoon', 'Evening', 'Night']
function partOfDay(h: number): number {
  if (h >= 5 && h < 12) return 0
  if (h >= 12 && h < 17) return 1
  if (h >= 17 && h < 22) return 2
  return 3
}

// RhythmView - when is this visitor active: a day-of-week × time-of-day heatmap,
// with the compact event list below for detail
function RhythmView({ items }: { items: TimelineItem[] }) {
  const grid = RHYTHM_PARTS.map(() => Array(7).fill(0) as number[])
  let maxCell = 0
  for (const e of items) {
    const d = new Date(e.occurredAt)
    if (Number.isNaN(d.getTime())) continue
    const p = partOfDay(d.getHours())
    grid[p][d.getDay()]++
    if (grid[p][d.getDay()] > maxCell) maxCell = grid[p][d.getDay()]
  }
  return (
    <div className="space-y-4">
      <div className="overflow-x-auto">
        <table className="border-separate border-spacing-1 text-[11px]">
          <thead>
            <tr>
              <th />
              {DOW.map((d) => (
                <th key={d} className="px-1 font-medium text-muted-foreground">
                  {d}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {RHYTHM_PARTS.map((part, pi) => (
              <tr key={part}>
                <td className="pr-2 text-right text-muted-foreground">{part}</td>
                {grid[pi].map((c, wi) => (
                  <td key={wi}>
                    <div className="relative grid size-7 place-items-center overflow-hidden rounded" title={`${DOW[wi]} ${part}: ${c}`}>
                      <div className="absolute inset-0 bg-primary" style={{ opacity: c > 0 ? Math.max(0.1, c / maxCell) : 0 }} aria-hidden />
                      {c > 0 ? (
                        <span className={cn('relative tabular-nums', c / maxCell > 0.55 ? 'text-primary-foreground' : 'text-foreground')}>{c}</span>
                      ) : (
                        <span className="relative text-muted-foreground/30">·</span>
                      )}
                    </div>
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <CompactList items={items} />
    </div>
  )
}

function Legend({ className }: { className?: string }) {
  const items: Array<{ s: string; label: string }> = [
    { s: 'web', label: 'Web' },
    { s: 'email', label: 'Email' },
    { s: 'commerce', label: 'Commerce' },
    { s: 'account', label: 'Account' },
  ]
  return (
    <div className={cn('flex flex-wrap items-center gap-x-4 gap-y-1 px-1 text-xs text-muted-foreground', className)}>
      {items.map((it) => {
        const s = sourceStyle(it.s)
        const Icon = s.Icon
        return (
          <span key={it.s} className="inline-flex items-center gap-1.5">
            <span className={cn('grid size-3.5 place-items-center rounded-full', s.dot, s.fg)}>
              <Icon className="size-2" />
            </span>
            {it.label}
          </span>
        )
      })}
    </div>
  )
}
