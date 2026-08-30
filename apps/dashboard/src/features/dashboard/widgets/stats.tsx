import { useQuery } from '@apollo/client/react'
import { formatCents } from '@/lib/format'
import { usePreference } from '@/lib/admin-prefs'
import { EMPTY_LAYOUT, applyLayout, isHiddenInLayout, type OrderedLayout } from '@/lib/layout-pref'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { TILES_PREF_KEY, TILE_DEFS } from '@/features/dashboard/home-items'
import { ProductCountDocument } from '@/features/catalog/Catalog.gql'
import { CollectionsDocument } from '@/features/catalog/Collections.gql'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { OrderQueueCountsDocument } from '@/features/orders/Orders.gql'
import { SALES_DAYS, useSalesReport } from './sales'
import { Tile, WidgetSection } from './primitives'

const nfmt = (n: number) => n.toLocaleString()

// The at-a-glance row. Money comes from the server-side sales report rather than a client-side
// reduce over every order, so the figures come from one aggregate and the page doesn't pull the order table
export function StatsWidget() {
  const { has } = usePermissions()
  const { effectiveChannelId } = useConfigScope()
  const canViewOrders = has('orders.view')
  const canViewCustomers = has('customers.view')
  const canViewCatalog = has('catalog.view')

  const sales = useSalesReport()
  // the queue-counts document, so this shares one request with the Order queues section rather than
  // asking for the same integer again
  const queuesQuery = useQuery(OrderQueueCountsDocument, {
    variables: { channelId: effectiveChannelId ?? null },
    skip: !canViewOrders,
  })
  const customerCountQuery = useQuery(CustomersPagedDocument, {
    variables: { limit: 1, channelId: effectiveChannelId ?? null },
    skip: !canViewCustomers,
  })
  const productCountQuery = useQuery(ProductCountDocument, { skip: !canViewCatalog })
  const collectionsQuery = useQuery(CollectionsDocument, { skip: !canViewCatalog })

  const summary = sales.summary
  const window = sales.multiCurrency
    ? `last ${SALES_DAYS} days · ${sales.currency}`
    : `last ${SALES_DAYS} days`
  const money = (cents: number | undefined) =>
    sales.currency && cents && cents > 0 ? formatCents(cents, sales.currency) : '-'

  const values: Record<string, { value: string; sub: string }> = {
    revenue: { value: money(summary?.grossRevenue), sub: window },
    orders: { value: nfmt(summary?.orderCount ?? 0), sub: `placed in the ${window}` },
    aov: { value: money(summary?.averageOrderValue), sub: `${nfmt(summary?.orderCount ?? 0)} orders` },
    margin: { value: money(summary?.marginTotal), sub: `cost ${money(summary?.costTotal)}` },
    refunds: { value: money(summary?.refundedAmount), sub: `${nfmt(summary?.refundCount ?? 0)} refunded` },
    'ordering-customers': { value: nfmt(summary?.uniqueCustomers ?? 0), sub: `ordered in the ${window}` },
    pending: {
      value: nfmt(queuesQuery.data?.pending.totalCount ?? 0),
      sub: 'awaiting confirmation',
    },
    customers: {
      value: nfmt(customerCountQuery.data?.customersPaged.totalCount ?? 0),
      sub: 'registered accounts',
    },
    products: { value: nfmt(productCountQuery.data?.productsPaged.totalCount ?? 0), sub: 'in the catalog' },
    collections: { value: nfmt(collectionsQuery.data?.collections.length ?? 0), sub: 'curated groupings' },
  }

  const [layout] = usePreference<OrderedLayout>(TILES_PREF_KEY, EMPTY_LAYOUT)
  const tiles = applyLayout(TILE_DEFS.filter((t) => has(t.permission)), layout)
    .filter((t) => !isHiddenInLayout(layout, t.id, t.defaultHidden))
    .map((t) => ({ ...t, ...values[t.id] }))

  return (
    <WidgetSection
      title="At a glance"
      description={`Money figures cover the last ${SALES_DAYS} days.`}
      error={sales.error}
      empty={tiles.length === 0}
    >
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {tiles.map((t) => (
          <Tile key={t.id} label={t.label} value={t.value ?? '-'} sub={t.sub} />
        ))}
      </div>
    </WidgetSection>
  )
}
