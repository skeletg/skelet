import { useState } from 'react'
import { act, fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderDialog } from '@/test/page-harness'
import { PreviewScheduleDocument } from './Scheduler.gql'
import { SchedulePicker } from './SchedulerWidgets'

// What this pins: the next-runs preview queries the schedule only once it has been still for 350ms,
// not on every keystroke

afterEach(() => vi.useRealTimers())

describe('SchedulePicker', () => {
  it('previews the schedule 350ms after it stops changing', async () => {
    vi.useFakeTimers()
    const asked: string[] = []
    const mocks = [
      {
        request: {
          query: PreviewScheduleDocument,
          variables: (vars: { schedule: string }) => {
            asked.push(vars.schedule)
            return true
          },
        },
        result: { data: { previewSchedule: { __typename: 'SchedulePreview', valid: true, error: null, nextRuns: [] } } },
        maxUsageCount: 5,
      },
    ]
    function Host() {
      const [schedule, setSchedule] = useState('@daily')
      return <SchedulePicker schedule={schedule} onScheduleChange={setSchedule} timezone="" onTimezoneChange={() => {}} />
    }
    renderDialog(<Host />, mocks)
    await act(async () => {})
    expect(asked).toEqual(['@daily'])

    fireEvent.change(screen.getByPlaceholderText(/^Cron/), { target: { value: '@hourly' } })
    await act(async () => vi.advanceTimersByTimeAsync(349))
    expect(asked).toEqual(['@daily'])
    await act(async () => vi.advanceTimersByTimeAsync(1))
    expect(asked).toEqual(['@daily', '@hourly'])
  })
})
