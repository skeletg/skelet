import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { ActivityProfile } from './ActivityProfile'
import { CustomerRecordingsSection } from '@/features/lens/SessionList'
import { CustomerAudiencesDocument, CustomerCommerceDocument, CustomerProfileDocument, CustomerSignalsDocument, SynapseCustomerHeaderDocument } from './Synapse.gql'

function fmtMoney(cents: number, currency: string): string {
  return (cents / 100).toLocaleString(undefined, { style: 'currency', currency: currency || 'USD' })
}
function fmtDate(v?: string | null): string {
  if (!v) return '-'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString(undefined, { dateStyle: 'medium' })
}
function humanize(s: string): string {
  return s.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
}
function pct(n: number, d: number): string {
  return d > 0 ? `${Math.round((n / d) * 100)}%` : '-'
}

export function SynapseCustomer() {
  const { id = '' } = useParams()
  const { data: hdr, error: hdrError } = useQuery(SynapseCustomerHeaderDocument, { variables: { id }, skip: !id })
  const c = hdr?.customer
  const name = c ? [c.firstName, c.lastName].filter(Boolean).join(' ').trim() || c.email : ''

  const { data, loading, error: profileError } = useQuery(CustomerProfileDocument, {
    variables: { customerId: id, sinceDays: 90 },
    skip: !id,
    fetchPolicy: 'cache-and-network',
  })
  const p = data?.customerProfile
  const error = profileError ?? hdrError

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={name || 'Customer'}
        meta={
          <span className="text-xs text-muted-foreground">
            {c?.email ? `${c.email} · ` : ''}Behavioural profile - every signal stitched to this person, last 90 days.
            {c ? (
              <>
                {' · '}
                <Link to={`/customers/${id}`} className="underline hover:text-foreground">
                  Open customer record
                </Link>
              </>
            ) : null}
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load customer profile</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        <DetailPageSection title="Commerce">
          <CommerceSection customerId={id} />
        </DetailPageSection>
        <SignalsSections customerId={id} />
        <CustomerRecordingsSection customerId={id} />
        {!p ? (
          <DetailPageSection title="Activity">
            <p className="text-sm text-muted-foreground">{loading ? 'Loading…' : 'No activity yet.'}</p>
          </DetailPageSection>
        ) : (
          <ActivityProfile
            profile={p}
            kind="customer"
            emptyText="No tracked activity yet. Web visits attach once this customer browses the storefront signed in (or signs in from a browser that has been tracking)."
          />
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// CommerceSection: the customer's order/CRM summary (orders, lifetime spend, AOV, subscriptions) + a meta line (member
// since, group, marketing consent, order span) - Customer-only; reads the customerCommerce query
function CommerceSection({ customerId }: { customerId: string }) {
  const { data, loading, error } = useQuery(CustomerCommerceDocument, {
    variables: { customerId },
    skip: !customerId,
    fetchPolicy: 'cache-and-network',
  })
  const c = data?.customerCommerce
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn't load commerce data</AlertTitle>
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }
  if (!c) {
    return <p className="py-4 text-sm text-muted-foreground">{loading ? 'Loading…' : 'No commerce data.'}</p>
  }
  const spend = c.spend.length ? c.spend.map((s) => fmtMoney(s.cents, s.currency)).join(' · ') : '-'
  // AOV is only meaningful within a single currency - cents across currencies don't sum - Mixed-currency customers get
  // '-' (the lifetime-spend line above already shows the per-currency split)
  const currencies = new Set(c.spend.map((s) => s.currency))
  const totalCents = c.spend.reduce((a, s) => a + s.cents, 0)
  const totalOrders = c.spend.reduce((a, s) => a + s.orders, 0)
  const aov = currencies.size === 1 && totalOrders > 0 ? fmtMoney(Math.round(totalCents / totalOrders), c.spend[0].currency) : '-'
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <CommerceTile label="Orders" value={c.orders.toLocaleString()} sub={c.returns > 0 ? `${c.returns} return${c.returns === 1 ? '' : 's'}` : undefined} />
        <CommerceTile label="Lifetime spend" value={spend} />
        <CommerceTile label="Avg order value" value={aov} />
        <CommerceTile label="Active subscriptions" value={c.activeSubscriptions.toLocaleString()} />
      </div>
      <p className="text-xs text-muted-foreground">
        Member since {fmtDate(c.memberSince)}
        {c.group ? ` · ${c.group}` : ''}
        {` · Marketing ${c.marketingOptedIn ? 'opted in' : 'opted out'}`}
        {c.firstOrderAt ? ` · Ordered ${fmtDate(c.firstOrderAt)} – ${fmtDate(c.lastOrderAt)}` : ''}
      </p>
    </div>
  )
}

function CommerceTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

// SignalsSections renders the cross-Signals 360° data for a customer - Audiences (Synapse segments) + Marketing (Pulse consent, email engagement, automation flows), customer-only
function SignalsSections({ customerId }: { customerId: string }) {
  const { data, loading, error } = useQuery(CustomerSignalsDocument, {
    variables: { customerId },
    skip: !customerId,
    fetchPolicy: 'cache-and-network',
  })
  const { data: audData } = useQuery(CustomerAudiencesDocument, {
    variables: { customerId },
    skip: !customerId,
    fetchPolicy: 'cache-and-network',
  })
  const audiences = audData?.customerAudiences ?? []
  const s = data?.customerSignals
  if (error) {
    return (
      <DetailPageSection title="Audiences & marketing">
        <Alert variant="destructive">
          <AlertTitle>Couldn't load audiences & marketing</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      </DetailPageSection>
    )
  }
  if (!s) {
    return (
      <DetailPageSection title="Audiences & marketing">
        <p className="py-4 text-sm text-muted-foreground">{loading ? 'Loading…' : 'No data.'}</p>
      </DetailPageSection>
    )
  }
  const email = s.email
  return (
    <>
      <DetailPageSection
        title="Audiences"
        description="Segments this customer is a member of - the audiences targetable by campaigns, flows, promotions and experiments."
      >
        {audiences.length === 0 ? (
          <p className="text-sm text-muted-foreground">Not a member of any segment yet.</p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {audiences.map((seg) => (
              <Link
                key={seg.id}
                to={`/synapse/segments/${seg.id}`}
                className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium hover:bg-muted"
                title={`Open “${seg.name}” segment`}
              >
                {seg.name}
              </Link>
            ))}
          </div>
        )}
      </DetailPageSection>

      <DetailPageSection
        title="Marketing"
        description="Consent, lifetime email engagement, and automation-flow enrollments - from Pulse."
      >
        <div className="space-y-4">
          {/* Consent */}
          <div className="flex flex-wrap gap-2">
            {s.consents.map((c) => (
              <span
                key={c.kind}
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium ${
                  c.optedIn
                    ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400'
                    : 'border-border bg-muted text-muted-foreground'
                }`}
                title={`${c.name} (${c.channel} · ${c.purpose})`}
              >
                <span aria-hidden>{c.optedIn ? '✓' : '✕'}</span>
                {c.name}
              </span>
            ))}
          </div>

          {/* Email engagement */}
          <div className="grid gap-3 sm:grid-cols-3">
            <CommerceTile label="Emails sent" value={email.sent.toLocaleString()} />
            <CommerceTile label="Opened" value={email.opened.toLocaleString()} sub={`${pct(email.opened, email.sent)} open rate`} />
            <CommerceTile label="Clicked" value={email.clicked.toLocaleString()} sub={`${pct(email.clicked, email.sent)} click rate`} />
          </div>
          {email.sent > 0 ? (
            <p className="text-xs text-muted-foreground">
              Last sent {fmtDate(email.lastSentAt)}
              {email.lastOpenAt ? ` · last opened ${fmtDate(email.lastOpenAt)}` : ''}
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">No marketing emails sent yet.</p>
          )}

          {/* Automation flows */}
          <div>
            <p className="mb-1.5 text-xs font-medium uppercase tracking-wider text-muted-foreground">Automation flows</p>
            {s.flows.length === 0 ? (
              <p className="text-sm text-muted-foreground">Not enrolled in any flow.</p>
            ) : (
              <ul className="space-y-1.5">
                {s.flows.map((f) => (
                  <li key={`${f.flowId}-${f.enrolledAt}`} className="flex items-center justify-between gap-3 rounded-lg border p-2 text-sm">
                    <div className="min-w-0">
                      <Link to={`/flows/${f.flowId}`} className="font-medium hover:underline" title="Open flow">
                        {f.flowName}
                      </Link>
                      <span className="ml-2 text-xs text-muted-foreground">{humanize(f.trigger)}</span>
                    </div>
                    <div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                      <FlowStatusBadge status={f.status} />
                      <span className="tabular-nums">{fmtDate(f.enrolledAt)}</span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </DetailPageSection>
    </>
  )
}

function FlowStatusBadge({ status }: { status: string }) {
  const tone: Record<string, string> = {
    active: 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-400',
    waiting: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400',
    completed: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    cancelled: 'border-border bg-muted text-muted-foreground',
    failed: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-400',
  }
  return (
    <span className={`rounded-full border px-2 py-0.5 font-medium capitalize ${tone[status] ?? 'border-border bg-muted text-muted-foreground'}`}>
      {status}
    </span>
  )
}
