import { useMatch, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'

import { DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { PromotionRuleCreatePage, PromotionRuleEditPage } from './PromotionRuleForm'
import { PromotionDetailAnalyticsDocument } from './Promotions.gql'
import { formatCents } from './promoFormat'
import { RedemptionsTrend } from './analyticsShared'

// PromotionPage is the standalone page for a single promotion at /promotions/:id (and /promotions/new) - Edit mode
// prepends a per-promotion performance strip above the rule editor; create mode shows the editor alone
export function PromotionPage() {
  const params = useParams<{ id?: string }>()
  const id = params.id ?? ''
  // /promotions/new is a literal route with no :id, so match the path rather than the param
  const isCreate = useMatch('/promotions/new') !== null || id === 'new'

  return (
    <DetailPageLayout>
      {isCreate ? (
        <PromotionRuleCreatePage />
      ) : (
        <PromotionRuleEditPage ruleId={id} analyticsSlot={<PromotionPerformance ruleId={id} />} />
      )}
    </DetailPageLayout>
  )
}

function PromotionPerformance({ ruleId }: { ruleId: string }) {
  const { data } = useQuery(PromotionDetailAnalyticsDocument, {
    variables: { ruleId, start: null, end: null },
    fetchPolicy: 'cache-and-network',
  })
  const stat = (data?.promotionAnalytics ?? []).find((a) => a.ruleId === ruleId)
  const daily = data?.promotionAnalyticsDaily ?? []

  return (
    <DetailPageSection
      title="Performance"
      description="All-time redemptions and discount impact for this promotion."
    >
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Tile label="Redemptions" value={(stat?.redemptionCount ?? 0).toLocaleString()} />
        <Tile label="Discount given" value={formatCents(stat?.discountTotalCents ?? 0)} />
        <Tile label="Order revenue" value={formatCents(stat?.orderRevenueCents ?? 0)} />
        <Tile label="Unique customers" value={(stat?.uniqueCustomers ?? 0).toLocaleString()} />
      </div>
      <div className="rounded border">
        <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Redemptions over time
        </div>
        <div className="p-3">
          <RedemptionsTrend daily={daily} height={180} gradientId="promoDetailTrend" />
        </div>
      </div>
    </DetailPageSection>
  )
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
    </div>
  )
}
