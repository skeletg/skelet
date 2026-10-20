import { useContext, useEffect, useRef } from 'react'
import { TabPanelContext } from './tab-panel-context'

// Workspace tabs stay mounted when hidden, so a polling page kept fetching every few seconds in tabs nobody was
// looking at. Polling pages pass their interval through this and stop while hidden
export function useVisiblePollInterval(ms: number): number {
  return useContext(TabPanelContext).visible ? ms : 0
}

// Polling restarts a full interval after the tab is shown again - fetch once on return so the page is current at once
export function useRefetchWhenShown(refetch: () => unknown): void {
  const visible = useContext(TabPanelContext).visible
  const wasVisible = useRef(visible)
  const latest = useRef(refetch)
  useEffect(() => {
    latest.current = refetch
  })
  useEffect(() => {
    if (visible && !wasVisible.current) void latest.current()
    wasVisible.current = visible
  }, [visible])
}
