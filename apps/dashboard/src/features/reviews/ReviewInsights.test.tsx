import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { dayEndISO, dayStartISO } from '@/lib/format'
import { ReviewInsightsContent } from './ReviewInsights'
import { ReviewInsightsDocument } from './Reviews.gql'

// What this pins: the default range ends on the admin's own today and is sent as whole local days - near midnight
// toISOString() named tomorrow (or yesterday), and the bounds were UTC days. Meaningful under a non-UTC TZ

afterEach(() => vi.useRealTimers())

describe('ReviewInsightsContent date range', () => {
  it('defaults to the local today and sends local day bounds', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 2, 4, 23, 30)) // late evening local
    const asked = vi.fn()
    renderPage({
      path: '/reviews/insights',
      routes: { '/reviews/insights': <ReviewInsightsContent /> },
      mocks: [
        meMock(),
        {
          request: {
            query: ReviewInsightsDocument,
            variables: (vars: Record<string, unknown>) => {
              asked(vars)
              return true
            },
          },
          result: { data: { reviewInsights: null } },
          maxUsageCount: 5,
        },
      ],
    })
    expect(((await screen.findByLabelText('To', {}, SLOW)) as HTMLInputElement).value).toBe('2026-03-04')
    await vi.waitFor(() => expect(asked).toHaveBeenCalled(), SLOW)
    expect(asked.mock.calls[0][0]).toMatchObject({ startDate: dayStartISO('2026-02-02'), endDate: dayEndISO('2026-03-04') })
  })
})
