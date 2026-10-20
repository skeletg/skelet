import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { dayEndISO, dayStartISO } from '@/lib/format'
import { PromotionsHome } from './PromotionsHome'
import { PromotionsHubDocument } from './Promotions.gql'

// What this pins: picking the same From and To day asks for that whole local day - new Date('YYYY-MM-DD') was UTC
// midnight, so the range was empty and the end day's redemptions never showed

const emptyHub = {
  promotionRules: { __typename: 'PromotionRulePage', items: [], totalCount: 0 },
  promotionAnalyticsTotals: { __typename: 'PromotionAnalyticsTotals', redemptionCount: 0, discountTotalCents: 0, uniqueCustomers: 0 },
  promotionAnalytics: [],
  promotionAnalyticsDaily: [],
}

describe('PromotionsHome date range', () => {
  it('sends the picked days as whole local days', async () => {
    const asked = vi.fn()
    renderPage({
      path: '/promotions',
      routes: { '/promotions': <PromotionsHome /> },
      adminId: 'admin-1',
      mocks: [
        meMock(),
        {
          request: {
            query: PromotionsHubDocument,
            variables: (vars: Record<string, unknown>) => {
              asked(vars)
              return true
            },
          },
          result: { data: emptyHub },
          maxUsageCount: 10,
        },
      ],
    })
    fireEvent.change(await screen.findByLabelText('From', {}, SLOW), { target: { value: '2026-03-04' } })
    fireEvent.change(screen.getByLabelText('To'), { target: { value: '2026-03-04' } })
    await vi.waitFor(
      () => expect(asked).toHaveBeenLastCalledWith({ start: dayStartISO('2026-03-04'), end: dayEndISO('2026-03-04') }),
      SLOW,
    )
  })
})
