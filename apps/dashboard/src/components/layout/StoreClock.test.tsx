import { Profiler } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StoreClock } from './StoreClock'

// What this pins: the sidebar clock shows minutes, so it re-renders once a minute, on the boundary - it used to tick
// every second

vi.mock('@apollo/client/react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@apollo/client/react')>()),
  useQuery: () => ({
    data: { configSections: [{ groups: [{ fields: [{ path: 'general/store_information/timezone', value: 'UTC' }] }] }] },
  }),
}))
vi.mock('@/lib/config-scope', () => ({ useConfigScope: () => ({ scopeInput: undefined }) }))

afterEach(() => vi.useRealTimers())

describe('StoreClock', () => {
  it('renders once per minute, on the minute', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-04T10:15:30Z'))
    const commits = vi.fn()
    render(
      <Profiler id="clock" onRender={commits}>
        <StoreClock />
      </Profiler>,
    )
    expect(screen.getByText(/10:15/)).toBeTruthy()
    const afterMount = commits.mock.calls.length

    await act(() => vi.advanceTimersByTimeAsync(29_000))
    expect(commits.mock.calls.length).toBe(afterMount)
    await act(() => vi.advanceTimersByTimeAsync(1_000))
    expect(screen.getByText(/10:16/)).toBeTruthy()
    await act(() => vi.advanceTimersByTimeAsync(60_000))
    expect(screen.getByText(/10:17/)).toBeTruthy()
    expect(commits.mock.calls.length).toBe(afterMount + 2)
  })
})
