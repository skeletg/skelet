import { createContext, useContext } from 'react'

// Which workspace pane and tab a page is sitting in - its own module so page-title and
// workspace-tabs can both use it without importing each other
//
// `active` is the strict one: the focused pane's current tab, the single page that owns the url and
// the document title - `visible` is merely on screen, which every split pane's current tab also is
export const TabPanelContext = createContext<{
  active: boolean
  visible: boolean
  id: string | null
  paneId: string | null
}>({
  active: true,
  visible: true,
  id: null,
  paneId: null,
})

export function useTabPanelActive(): boolean {
  return useContext(TabPanelContext).active
}
