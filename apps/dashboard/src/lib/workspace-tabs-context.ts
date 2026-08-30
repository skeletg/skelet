import { createContext, useContext } from 'react'
import type { Edge, LayoutNode } from './workspace-layout'
import type { WorkspacePane, WorkspaceTab } from './workspace-tabs'

// The context, its shape and its hook live apart from the provider - fast refresh wants contexts
// out of any file that exports a component

export type TabsApi = {
  panes: WorkspacePane[]
  layout: LayoutNode
  focusedPaneId: string
  tabs: WorkspaceTab[] // the focused pane's strip, which is the whole workspace until it's split
  activeId: string
  routerLagging: boolean // true between asking for a tab-switch navigation and the router landing on it
  activate: (paneId: string, tabId: string) => void
  focusPane: (paneId: string) => void
  close: (paneId: string, tabId: string) => void
  open: (paneId?: string) => void
  moveTab: (tabId: string, toPaneId: string, index: number) => void
  splitWithTab: (tabId: string, targetPaneId: string, edge: Edge) => void
  mergePanes: () => void
  resizeSplit: (splitId: string, sizes: number[]) => void
  workspaces: ReadonlyArray<{ id: string; name: string }>
  workspaceId: string
  workspaceName: string
  switchWorkspace: (id: string) => void
  createWorkspace: (name: string) => void
  duplicateWorkspace: (name: string) => void
  renameWorkspace: (id: string, name: string) => void
  deleteWorkspace: (id: string) => void
  nextWorkspaceName: () => string
}

export const TabsContext = createContext<TabsApi | null>(null)

export function useWorkspaceTabs() {
  const ctx = useContext(TabsContext)
  if (!ctx) throw new Error('useWorkspaceTabs must be used within WorkspaceTabsProvider')
  return ctx
}
