import { useQuery } from '@apollo/client/react'
import { formatCents } from '@/lib/format'
import { PromotionTotalsDocument } from '@/features/promotions/Promotions.gql'
import { SALES_DAYS, windowStart } from './sales'
import { Tile, WidgetSection } from './primitives'

export function PromotionsWidget() {
  const { data, loading, error } = useQuery(PromotionTotalsDocument, {
    variables: { start: windowStart(), end: null },
    fetchPolicy: 'cache-and-network',
  })
  const totals = data?.promotionAnalyticsTotals

  return (
    <WidgetSection
      title="Promotions"
      description={`What your discounts did over the last ${SALES_DAYS} days.`}
      to="/promotions"
      loading={loading && !totals}
      error={error ?? null}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <Tile label="Redemptions" value={(totals?.redemptionCount ?? 0).toLocaleString()} />
        {/* the totals are summed across currencies server-side, so there is no single code to
            attach - formatCents renders a plain number for an empty one */}
        <Tile label="Discount given" value={formatCents(totals?.discountTotalCents ?? 0, '')} sub="store currency" />
        <Tile label="Customers reached" value={(totals?.uniqueCustomers ?? 0).toLocaleString()} />
      </div>
    </WidgetSection>
  )
}
