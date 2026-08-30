import { useQuery } from '@apollo/client/react'
import { useConfigScope } from '@/lib/config-scope'
import { SalesReportDocument } from '@/features/dashboard/Sales.gql'

// The dashboard's money figures, server-aggregated
//
// Several sections read from the same report, so they all call this with IDENTICAL variables and
// Apollo's in-flight deduplication collapses them into one request. Keep the window and the limit
// here rather than per section, or they stop being identical and each section pays its own round trip
export const SALES_DAYS = 30
export const TOP_PRODUCTS_LIMIT = 5

// Returns the same string all day, so the variables stay value-equal across renders and Apollo has
// no reason to refetch - a tab left open past midnight picks up the new window on its next fetch
function windowStart(): string {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() - (SALES_DAYS - 1))
  return d.toISOString()
}

export function useSalesReport() {
  // follows the header scope: a channel reports on that channel, Global spans every permitted one
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(SalesReportDocument, {
    variables: {
      channelId: effectiveChannelId ?? null,
      startDate: windowStart(),
      endDate: null,
      bucket: 'day',
      topProductsLimit: TOP_PRODUCTS_LIMIT,
    },
    fetchPolicy: 'cache-and-network',
  })

  const report = data?.salesReport
  const summaries = report?.summary ?? []
  // summary comes back revenue-sorted, so the first entry is the store's primary currency
  const currency = summaries[0]?.currencyCode ?? ''
  const summary = summaries[0]
  const series = (report?.timeseries ?? [])
    .filter((p) => p.currencyCode === currency)
    .map((p) => ({ period: String(p.period), revenue: p.revenue, orders: p.orderCount }))

  return {
    loading,
    error: error ?? null,
    currency,
    summary,
    multiCurrency: summaries.length > 1,
    series,
    topProducts: report?.topProducts ?? [],
    statusBreakdown: report?.statusBreakdown ?? [],
  }
}
