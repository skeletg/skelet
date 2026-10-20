import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderDialog } from '@/test/page-harness'
import { CampaignStats } from './CampaignComposer'
import { EmailCampaignStatsDocument } from './Campaigns.gql'

// What these pin: the delivery tiles poll the four counters (not the whole campaign and its email design) and only
// while a send is in flight - a draft or a finished campaign left open no longer re-downloads every 5 seconds

afterEach(() => vi.useRealTimers())

function renderStats(status: string) {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  const asked = vi.fn()
  const stats = { __typename: 'EmailCampaign', id: 'cp1', status, totalRecipients: 10, sentCount: 4, failedCount: 0, skippedCount: 1 }
  renderDialog(<CampaignStats initial={{ id: 'cp1', status } as never} />, [
    {
      request: {
        query: EmailCampaignStatsDocument,
        variables: (v: Record<string, unknown>) => {
          asked(v)
          return true
        },
      },
      result: { data: { emailCampaign: stats } },
      maxUsageCount: 10,
    },
  ])
  return asked
}

describe('CampaignStats', () => {
  it('polls the counters while sending', async () => {
    const asked = renderStats('sending')
    expect(await screen.findByText('4')).toBeTruthy()
    await vi.advanceTimersByTimeAsync(11_000)
    expect(asked.mock.calls.length).toBeGreaterThanOrEqual(3)
  })

  it('fetches once and stops for a finished campaign', async () => {
    const asked = renderStats('sent')
    expect(await screen.findByText('4')).toBeTruthy()
    await vi.advanceTimersByTimeAsync(11_000)
    expect(asked).toHaveBeenCalledTimes(1)
  })
})
