import { renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { TabPanelContext } from './tab-panel-context'
import { useRefetchWhenShown, useVisiblePollInterval } from './visible-poll'

// What these pin: a hidden workspace tab stops polling, and coming back fetches once instead of waiting a full interval

function inPanel(visible: boolean) {
  return ({ children }: { children: ReactNode }) => (
    <TabPanelContext.Provider value={{ active: visible, visible, id: 't1', paneId: 'p1' }}>{children}</TabPanelContext.Provider>
  )
}

describe('useVisiblePollInterval', () => {
  it('polls only while the tab is on screen', () => {
    expect(renderHook(() => useVisiblePollInterval(5000), { wrapper: inPanel(true) }).result.current).toBe(5000)
    expect(renderHook(() => useVisiblePollInterval(5000), { wrapper: inPanel(false) }).result.current).toBe(0)
  })

  it('polls outside any workspace panel, e.g. the shell', () => {
    expect(renderHook(() => useVisiblePollInterval(5000)).result.current).toBe(5000)
  })
})

describe('useRefetchWhenShown', () => {
  it('refetches once when a hidden tab is shown again, never on mount', () => {
    const refetch = vi.fn()
    let visible = true
    const wrapper = ({ children }: { children: ReactNode }) => inPanel(visible)({ children })
    const { rerender } = renderHook(() => useRefetchWhenShown(refetch), { wrapper })
    expect(refetch).not.toHaveBeenCalled()
    visible = false
    rerender()
    expect(refetch).not.toHaveBeenCalled()
    visible = true
    rerender()
    expect(refetch).toHaveBeenCalledTimes(1)
  })
})
