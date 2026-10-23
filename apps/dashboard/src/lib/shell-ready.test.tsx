import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useShellReady } from './shell-ready'

const { state } = vi.hoisted(() => ({
  state: { permsLoading: true, featuresLoading: true, prefsSettled: false },
}))

vi.mock('@/shared/auth/usePermissions', () => ({
  usePermissions: () => ({ has: () => true, loading: state.permsLoading }),
}))
vi.mock('@/shared/features/useFeatures', () => ({
  useFeatures: () => ({ enabled: () => true, loading: state.featuresLoading }),
}))
vi.mock('@/lib/admin-prefs', () => ({ usePrefsSettled: () => state.prefsSettled }))

function Probe() {
  return <span data-testid="ready">{String(useShellReady())}</span>
}

const readyText = (c: HTMLElement) => c.querySelector('[data-testid="ready"]')?.textContent

describe('useShellReady', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.assign(state, { permsLoading: true, featuresLoading: true, prefsSettled: false })
  })
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('waits for grants, server switches and the saved arrangement together', () => {
    const { container, rerender } = render(<Probe />)
    expect(readyText(container)).toBe('false')
    Object.assign(state, { permsLoading: false, featuresLoading: false })
    rerender(<Probe />)
    expect(readyText(container)).toBe('false')
    state.prefsSettled = true
    rerender(<Probe />)
    expect(readyText(container)).toBe('true')
  })

  it('gives up waiting after the timeout, so a failed prefs fetch cannot hold the skeletons', () => {
    const { container } = render(<Probe />)
    act(() => vi.advanceTimersByTime(3000))
    expect(readyText(container)).toBe('true')
  })

  it('stays ready once ready, so a later refetch never brings the skeletons back', () => {
    Object.assign(state, { permsLoading: false, featuresLoading: false, prefsSettled: true })
    const { container, rerender } = render(<Probe />)
    state.permsLoading = true
    rerender(<Probe />)
    expect(readyText(container)).toBe('true')
  })
})
