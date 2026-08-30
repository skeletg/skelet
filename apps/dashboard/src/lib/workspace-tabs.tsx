import { useEffect, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useResolvedPageTitle } from '@/lib/page-title'
import { useLandingPref } from '@/lib/landing'
import { randomId } from '@/lib/random-id'
import { confirmCloseTab, confirmSwitchWorkspace, mirrorTabTitles } from '@/lib/unsaved-changes'
import { allowNextNavigation } from '@/lib/nav-guard'
import { usePrefsLoaded, usePreference } from '@/lib/admin-prefs'
import {
  layoutPaneIds,
  paneNode,
  parseLayout,
  removePane,
  setSplitSizes,
  splitPane,
  type Edge,
  type LayoutNode,
} from '@/lib/workspace-layout'
import { TabsContext } from './workspace-tabs-context'

// Chrome-style workspace tabs: every page lives in a tab, navigating stays IN the current tab (path
// and title follow the router), and new tabs only open through a strip's + button
//
// Tabs are grouped into PANES arranged by a split tree - one pane is the whole content area until a
// tab gets dragged onto an edge. Only the focused pane's current tab is live: it owns the browser
// url, so every other pane holds its own page frozen at the path it was last showing

export type WorkspaceTab = {
  id: string
  path: string // pathname + search, so filters and pagination survive a tab switch
  title: string
}

export type WorkspacePane = {
  id: string
  tabs: WorkspaceTab[]
  activeId: string
}

type TabsState = { panes: WorkspacePane[]; layout: LayoutNode; focusedPaneId: string }

// A workspace is one whole arrangement - its tabs, its panes, which pane had focus. Several can be
// kept side by side, but only the active one is MOUNTED: keeping every workspace's pages warm would
// multiply the live Apollo subscriptions, and workspaces exist precisely to be different contexts
export type Workspace = { id: string; name: string } & TabsState

type Store = { workspaces: Workspace[]; activeId: string }

// Workspaces roam per admin through the preferences store, the same way saved views and table
// layouts do - so the tabs and splits you left on one machine are the ones waiting on the next
const PREF_KEY = 'workspaces'

// A device-local copy, read once as a seed and then cleared
const DEVICE_KEY = 'skelet-dash:workspace-tabs'

type StoredWorkspaces = { workspaces: unknown[]; activeWorkspaceId: string }

function freshTab(path: string, title: string): WorkspaceTab {
  return { id: randomId(), path, title }
}

function isTab(t: unknown): t is WorkspaceTab {
  const v = t as WorkspaceTab | null
  return !!v && typeof v.id === 'string' && typeof v.path === 'string'
}

function normalizePane(raw: unknown): WorkspacePane | null {
  const v = raw as Partial<WorkspacePane> | null
  if (!v || typeof v.id !== 'string' || !Array.isArray(v.tabs)) return null
  const tabs = v.tabs.filter(isTab).map((t) => ({ ...t, title: typeof t.title === 'string' ? t.title : '' }))
  if (tabs.length === 0) return null
  const active = tabs.find((t) => t.id === v.activeId) ?? tabs[0]
  return { id: v.id, tabs, activeId: active.id }
}

function flatLayout(paneIds: string[]): LayoutNode {
  if (paneIds.length === 1) return paneNode(paneIds[0])
  return {
    kind: 'split',
    id: randomId(),
    dir: 'row',
    sizes: paneIds.map(() => 100 / paneIds.length),
    children: paneIds.map(paneNode),
  }
}

function sameMembers(a: string[], b: string[]): boolean {
  return a.length === b.length && new Set([...a, ...b]).size === a.length
}

// A deep link or reload should land in the tab you were on, never spawn a duplicate - so the
// focused pane's current tab takes over the incoming url, dropping a label that no longer fits it
function adoptUrl(panes: WorkspacePane[], focusedPaneId: string, path: string): WorkspacePane[] {
  return panes.map((p) => {
    if (p.id !== focusedPaneId) return p
    return {
      ...p,
      tabs: p.tabs.map((t) =>
        t.id === p.activeId ? { ...t, path, title: t.path === path ? t.title : '' } : t,
      ),
    }
  })
}

function restorePanes(raw: Record<string, unknown>): WorkspacePane[] {
  if (Array.isArray(raw.panes)) {
    return raw.panes.map(normalizePane).filter((p): p is WorkspacePane => p !== null)
  }
  // a bare {tabs, activeId} strip carries no panes - it restores as the single pane it describes
  const pane = normalizePane({ id: randomId(), tabs: raw.tabs, activeId: raw.activeId })
  return pane ? [pane] : []
}

function normalizeWorkspace(raw: unknown, fallbackName: string): Workspace | null {
  if (!raw || typeof raw !== 'object') return null
  const w = raw as Record<string, unknown>
  const panes = restorePanes(w)
  if (panes.length === 0) return null
  const ids = panes.map((p) => p.id)
  let layout = parseLayout(w.layout)
  // the tree and the pane list have to name exactly the same panes, or the workspace renders holes
  if (!layout || !sameMembers(layoutPaneIds(layout), ids)) layout = flatLayout(ids)
  const focused = panes.find((p) => p.id === w.focusedPaneId) ?? panes[0]
  return {
    id: typeof w.id === 'string' ? w.id : randomId(),
    name: typeof w.name === 'string' && w.name.trim() ? w.name : fallbackName,
    panes,
    layout,
    focusedPaneId: focused.id,
  }
}

function parseStore(raw: unknown): Store | null {
  if (!raw || typeof raw !== 'object') return null
  const stored = raw as Record<string, unknown>
  // three shapes have lived here - a bare strip, one unnamed workspace, then many
  const list = Array.isArray(stored.workspaces) ? stored.workspaces : [stored]
  const workspaces = list
    .map((w, i) => normalizeWorkspace(w, defaultName(i)))
    .filter((w): w is Workspace => w !== null)
  if (workspaces.length === 0) return null
  const active = workspaces.find((w) => w.id === stored.activeWorkspaceId) ?? workspaces[0]
  return { workspaces, activeId: active.id }
}

// A reload should land in the tab you were on, so the incoming url claims the active tab - kept out
// of parseStore because a copy arriving from ANOTHER machine has its own page to go back to
function withUrl(store: Store, path: string): Store {
  return {
    ...store,
    workspaces: store.workspaces.map((w) =>
      w.id === store.activeId ? { ...w, panes: adoptUrl(w.panes, w.focusedPaneId, path) } : w,
    ),
  }
}

function deviceStore(): unknown {
  try {
    const raw = localStorage.getItem(DEVICE_KEY)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null // corrupt or unavailable - there is simply nothing to carry over
  }
}

function activeTabOf(store: Store): WorkspaceTab {
  const w = store.workspaces.find((x) => x.id === store.activeId) ?? store.workspaces[0]
  const pane = w.panes.find((p) => p.id === w.focusedPaneId) ?? w.panes[0]
  return pane.tabs.find((t) => t.id === pane.activeId) ?? pane.tabs[0]
}

function defaultName(index: number): string {
  return `Workspace ${index + 1}`
}

function freshWorkspace(name: string, path: string, title: string): Workspace {
  const tab = freshTab(path, title)
  const pane: WorkspacePane = { id: randomId(), tabs: [tab], activeId: tab.id }
  return { id: randomId(), name, panes: [pane], layout: paneNode(pane.id), focusedPaneId: pane.id }
}

function buildStore(raw: unknown, path: string): Store {
  try {
    const parsed = parseStore(raw)
    if (parsed) return withUrl(parsed, path)
  } catch {
    /* corrupt stored value - start with a single workspace */
  }
  const w = freshWorkspace(defaultName(0), path, '')
  return { workspaces: [w], activeId: w.id }
}

// A copy has to mint fresh ids all the way down - tab ids key the unsaved-changes registry, so two
// workspaces sharing one would have the copy's edits counted against the original
function cloneWorkspace(source: Workspace, name: string): Workspace {
  const paneIds = new Map<string, string>()
  const panes = source.panes.map((p) => {
    const id = randomId()
    paneIds.set(p.id, id)
    const tabs = p.tabs.map((t) => ({ ...t, id: randomId() }))
    const at = p.tabs.findIndex((t) => t.id === p.activeId)
    return { id, tabs, activeId: tabs[at < 0 ? 0 : at].id }
  })
  const remap = (n: LayoutNode): LayoutNode =>
    n.kind === 'pane'
      ? paneNode(paneIds.get(n.id) ?? n.id)
      : { ...n, id: randomId(), children: n.children.map(remap) }
  return {
    id: randomId(),
    name,
    panes,
    layout: remap(source.layout),
    focusedPaneId: paneIds.get(source.focusedPaneId) ?? panes[0].id,
  }
}

function insertAt(list: WorkspaceTab[], tab: WorkspaceTab, index: number): WorkspaceTab[] {
  const next = [...list]
  next.splice(Math.max(0, Math.min(index, next.length)), 0, tab)
  return next
}

// Which tab a pane falls back to when the one leaving was its current - the neighbour on the right,
// then the left, same as closing
function activeAfterLeaving(pane: WorkspacePane, tabId: string): string {
  if (pane.activeId !== tabId) return pane.activeId
  const i = pane.tabs.findIndex((t) => t.id === tabId)
  const next = pane.tabs[i + 1] ?? pane.tabs[i - 1]
  return next ? next.id : pane.activeId
}

export function WorkspaceTabsProvider({ children }: { children: ReactNode }) {
  const location = useLocation()
  const navigate = useNavigate()
  const { title, isOverride } = useResolvedPageTitle()
  // a new tab opens wherever the admin starts their day, same as login does
  const { landing, title: landingTitle } = useLandingPref()
  const home = { path: landing, title: landingTitle }
  const path = location.pathname + location.search
  // The stored entry for the live tab is allowed to go stale while you navigate - the live view below patches it from
  // the router every render, and event handlers snapshot it on tab switches, so no effect ever has to write state back
  const [roaming, setRoaming] = usePreference<StoredWorkspaces | null>(PREF_KEY, null)
  const prefsLoaded = usePrefsLoaded()
  // The preferences store mirrors itself to this device, so on a machine this admin has used the
  // seed is already the real thing - on a fresh one it is a throwaway the roaming copy replaces
  const [store, setStore] = useState(() => buildStore(roaming ?? deviceStore(), path))
  // Every action below reads and writes the ACTIVE workspace, and a Workspace is a TabsState with a
  // name bolted on - so aliasing it here keeps all of them working on plain panes/layout/focus
  const state: Workspace = store.workspaces.find((w) => w.id === store.activeId) ?? store.workspaces[0]

  // React Router runs navigations as transitions, so for a beat after a tab switch the router
  // still reports the OLD page - patching the new live tab from it would stamp the wrong
  // path/title on it (the "name flashes" bug, and a fast second switch would even snapshot the
  // wrong page into a tab) - the latch holds patching until the navigation we asked for lands,
  // cleared with the render-time adjust pattern so no effect has to write state
  const [pendingNav, setPendingNav] = useState<string | null>(null)
  if (pendingNav !== null && pendingNav === path) setPendingNav(null)
  const routerLagging = pendingNav !== null && pendingNav !== path

  // The roaming copy arrives a render after mount at the earliest, so what seeded this session has
  // to be reconciled against it - comparing the RAW seed, not the state built from it, since that
  // already has the current url stamped into its active tab
  const [seed] = useState(() => JSON.stringify(roaming ?? deviceStore() ?? null))
  const [adoptNav, setAdoptNav] = useState<string | null>(null)
  if (adoptNav !== null && adoptNav === path) setAdoptNav(null)
  const [adopted, setAdopted] = useState(false)
  if (!adopted && prefsLoaded) {
    setAdopted(true)
    const incoming = JSON.stringify(roaming ?? null) === seed ? null : parseStore(roaming ?? null)
    if (incoming) {
      const seeded = seed === 'null' ? null : parseStore(JSON.parse(seed) as unknown)
      // A url outranks the roaming copy whenever it was asked for on purpose - on a returning device
      // that means anywhere it did not leave off, and on a fresh one anything but the way in
      const enteredNormally = path === '/' || path === home.path
      const deepLink = seeded ? activeTabOf(seeded).path !== path : !enteredNormally
      if (deepLink) {
        setStore(withUrl(incoming, path))
      } else {
        const tab = activeTabOf(incoming)
        setStore(incoming)
        setPendingNav(tab.path)
        setAdoptNav(tab.path)
      }
    }
  }

  const panes = state.panes.map((p) => {
    if (p.id !== state.focusedPaneId || routerLagging) return p
    return {
      ...p,
      tabs: p.tabs.map((t) => {
        if (t.id !== p.activeId) return t
        // A record override always wins, but a bare route default must not stomp the richer stored
        // label while a revisited tab's page is still resolving - that read as a title flash
        const nextTitle =
          isOverride && title ? title : t.path === path && t.title ? t.title : (title ?? t.title)
        return { ...t, path, title: nextTitle }
      }),
    }
  })

  const focusedPane = panes.find((p) => p.id === state.focusedPaneId) ?? panes[0]
  const allTabs = panes.flatMap((p) => p.tabs)

  // the live view only exists for the active workspace, so it is patched back in on the way out
  const serialized = JSON.stringify({
    workspaces: store.workspaces.map((w) => (w.id === state.id ? { ...w, panes } : w)),
    activeWorkspaceId: store.activeId,
  })
  // Keeps the discard prompts able to name a page by its tab label - cheap enough to redo whenever the strip changes
  mirrorTabTitles(allTabs, focusedPane?.activeId ?? null)
  useEffect(() => {
    // Writing before the roaming copy lands would mark the key dirty, and the prefs store treats a
    // dirty key as newer than the server - this session's seed would win over the real workspaces
    if (!prefsLoaded) return
    setRoaming(JSON.parse(serialized) as StoredWorkspaces)
  }, [serialized, prefsLoaded, setRoaming])

  // Nothing reads the device-local copy once the roaming one is in play
  useEffect(() => {
    if (!prefsLoaded) return
    try {
      localStorage.removeItem(DEVICE_KEY)
    } catch {
      /* nothing to clean up */
    }
  }, [prefsLoaded])

  // The adopted workspace has its own page to return to, and navigate cannot be called mid-render -
  // the latch clears itself once the router lands, same as pendingNav
  useEffect(() => {
    if (adoptNav === null || adoptNav === path) return
    allowNextNavigation()
    navigate(adoptNav, { replace: true })
  }, [adoptNav, path, navigate])

  // Panes and layout are always written together - a navigation only happens when the live tab
  // changes, and the latch has to be armed before the router is asked to move
  const commit = (next: TabsState, navTo: string | null, replace = false) => {
    if (navTo !== null) setPendingNav(navTo)
    setStore((s) => ({
      ...s,
      workspaces: s.workspaces.map((w) => (w.id === s.activeId ? { ...w, ...next } : w)),
    }))
    if (navTo === null) return
    // Every pane and tab move lands here, and none of them unmount a page - the panels stay put and
    // only the url moves, so the leave-page guard has no business seeing these
    allowNextNavigation()
    navigate(navTo, { replace })
  }

  const paneOf = (tabId: string) => panes.find((p) => p.tabs.some((t) => t.id === tabId))

  const activate = (paneId: string, tabId: string, replace = false) => {
    const pane = panes.find((p) => p.id === paneId)
    const target = pane?.tabs.find((t) => t.id === tabId)
    if (!pane || !target) return
    if (paneId === state.focusedPaneId && tabId === pane.activeId) return
    commit(
      {
        ...state,
        panes: panes.map((p) => (p.id === paneId ? { ...p, activeId: tabId } : p)),
        focusedPaneId: paneId,
      },
      target.path,
      replace,
    )
  }

  // Clicking between panes is constant, and each one would otherwise stack a history entry - the
  // page being focused is already on screen, so replacing keeps Back meaning the previous PAGE
  const focusPane = (paneId: string) => {
    const pane = panes.find((p) => p.id === paneId)
    if (pane) activate(paneId, pane.activeId, true)
  }

  const close = (paneId: string, tabId: string) => {
    const pane = panes.find((p) => p.id === paneId)
    const i = pane?.tabs.findIndex((t) => t.id === tabId) ?? -1
    if (!pane || i < 0) return
    // Closing is the one tab action that unmounts a page and drops its staged edits - switching,
    // moving and splitting all leave every panel mounted, so they stay unprompted
    if (!confirmCloseTab(tabId)) return

    const rest = pane.tabs.filter((t) => t.id !== tabId)
    const focused = state.focusedPaneId === paneId

    if (rest.length > 0) {
      const nextActive = activeAfterLeaving(pane, tabId)
      const next = panes.map((p) => (p.id === paneId ? { ...p, tabs: rest, activeId: nextActive } : p))
      const moved = nextActive !== pane.activeId
      const path = rest.find((t) => t.id === nextActive)?.path ?? null
      commit({ ...state, panes: next }, focused && moved ? path : null)
      return
    }

    if (panes.length === 1) {
      // the workspace is never empty - closing the last tab resets it to home, like a fresh window
      const t = freshTab(home.path, home.title)
      commit(
        { panes: [{ ...pane, tabs: [t], activeId: t.id }], layout: paneNode(pane.id), focusedPaneId: pane.id },
        home.path,
      )
      return
    }

    // the pane went with its last tab, so the split closes up around it
    const remaining = panes.filter((p) => p.id !== paneId)
    const idx = panes.findIndex((p) => p.id === paneId)
    const heir = remaining[idx] ?? remaining[idx - 1] ?? remaining[0]
    const layout = removePane(state.layout, paneId) ?? paneNode(heir.id)
    const nextFocus = focused ? heir.id : state.focusedPaneId
    commit(
      { panes: remaining, layout, focusedPaneId: nextFocus },
      focused ? (heir.tabs.find((t) => t.id === heir.activeId)?.path ?? home.path) : null,
    )
  }

  const open = (paneId?: string) => {
    const target = paneId ?? state.focusedPaneId
    if (!panes.some((p) => p.id === target)) return
    const t = freshTab(home.path, home.title)
    commit(
      {
        ...state,
        panes: panes.map((p) => (p.id === target ? { ...p, tabs: [...p.tabs, t], activeId: t.id } : p)),
        focusedPaneId: target,
      },
      home.path,
    )
  }

  // index is the insertion slot in the target's CURRENT list, before the tab is taken out of it
  const moveTab = (tabId: string, toPaneId: string, index: number) => {
    const from = paneOf(tabId)
    const to = panes.find((p) => p.id === toPaneId)
    const tab = from?.tabs.find((t) => t.id === tabId)
    if (!from || !to || !tab) return

    if (from.id === to.id) {
      const at = from.tabs.findIndex((t) => t.id === tabId)
      const without = from.tabs.filter((t) => t.id !== tabId)
      const tabs = insertAt(without, tab, at < index ? index - 1 : index)
      // reordering never changes which page is on screen, so a same-pane drop stays silent unless
      // it also pulls focus over to that pane
      const refocus = state.focusedPaneId !== from.id
      commit(
        { ...state, panes: panes.map((p) => (p.id === from.id ? { ...p, tabs } : p)), focusedPaneId: from.id },
        refocus ? (from.tabs.find((t) => t.id === from.activeId)?.path ?? null) : null,
      )
      return
    }

    const rest = from.tabs.filter((t) => t.id !== tabId)
    let next = panes.map((p) => {
      if (p.id === to.id) return { ...p, tabs: insertAt(p.tabs, tab, index), activeId: tab.id }
      if (p.id === from.id) return { ...p, tabs: rest, activeId: activeAfterLeaving(from, tabId) }
      return p
    })
    let layout = state.layout
    if (rest.length === 0) {
      next = next.filter((p) => p.id !== from.id)
      layout = removePane(layout, from.id) ?? paneNode(to.id)
    }
    commit({ panes: next, layout, focusedPaneId: to.id }, tab.path)
  }

  const splitWithTab = (tabId: string, targetPaneId: string, edge: Edge) => {
    const from = paneOf(tabId)
    const tab = from?.tabs.find((t) => t.id === tabId)
    if (!from || !tab || !panes.some((p) => p.id === targetPaneId)) return
    // splitting a pane off its own only tab would just move the pane sideways and empty the source
    if (from.id === targetPaneId && from.tabs.length === 1) return

    const paneId = randomId()
    let layout = splitPane(state.layout, targetPaneId, paneId, edge)
    const rest = from.tabs.filter((t) => t.id !== tabId)
    let next: WorkspacePane[] = panes.map((p) =>
      p.id === from.id ? { ...p, tabs: rest, activeId: activeAfterLeaving(from, tabId) } : p,
    )
    next = [...next, { id: paneId, tabs: [tab], activeId: tab.id }]
    if (rest.length === 0) {
      next = next.filter((p) => p.id !== from.id)
      layout = removePane(layout, from.id) ?? paneNode(paneId)
    }
    commit({ panes: next, layout, focusedPaneId: paneId }, tab.path)
  }

  // Collapses the split back to one pane, in the order the panes were laid out - the live tab is
  // already on screen, so nothing has to navigate
  const mergePanes = () => {
    if (panes.length === 1) return
    const order = layoutPaneIds(state.layout)
    const tabs = order.flatMap((id) => panes.find((p) => p.id === id)?.tabs ?? [])
    const kept = { id: focusedPane.id, tabs, activeId: focusedPane.activeId }
    commit({ panes: [kept], layout: paneNode(kept.id), focusedPaneId: kept.id }, null)
  }

  // Dividers commit once on release, so this never touches panes - the live view keeps patching
  // whatever the router is doing underneath
  const resizeSplit = (splitId: string, sizes: number[]) => {
    setStore((s) => ({
      ...s,
      workspaces: s.workspaces.map((w) =>
        w.id === s.activeId ? { ...w, layout: setSplitSizes(w.layout, splitId, sizes) } : w,
      ),
    }))
  }

  // Only the active workspace is mounted, so leaving one tears down every page in it - unlike a tab
  // switch, which leaves its neighbours warm, this really can drop staged edits
  const leaveActive = (): boolean => confirmSwitchWorkspace(allTabs.map((t) => t.id))

  const enter = (next: Store, target: Workspace) => {
    const pane = target.panes.find((p) => p.id === target.focusedPaneId) ?? target.panes[0]
    const tab = pane.tabs.find((t) => t.id === pane.activeId) ?? pane.tabs[0]
    setPendingNav(tab.path)
    // the outgoing workspace keeps whatever the router had patched into its live tab
    setStore({ ...next, workspaces: next.workspaces.map((w) => (w.id === state.id ? { ...w, panes } : w)) })
    allowNextNavigation()
    navigate(tab.path)
  }

  // First gap in the default sequence, so deleting "Workspace 2" then adding one reuses the name
  const nextWorkspaceName = (): string => {
    const taken = new Set(store.workspaces.map((w) => w.name))
    for (let i = 0; ; i++) if (!taken.has(defaultName(i))) return defaultName(i)
  }

  const switchWorkspace = (id: string) => {
    if (id === store.activeId) return
    const target = store.workspaces.find((w) => w.id === id)
    if (!target || !leaveActive()) return
    enter({ ...store, activeId: id }, target)
  }

  const createWorkspace = (name: string) => {
    if (!leaveActive()) return
    const w = freshWorkspace(name.trim() || nextWorkspaceName(), home.path, home.title)
    enter({ workspaces: [...store.workspaces, w], activeId: w.id }, w)
  }

  // Forks the arrangement you are looking at, so a layout worth keeping can be kept before you
  // rearrange it - the copy is what you land in, leaving the original untouched
  const duplicateWorkspace = (name: string) => {
    if (!leaveActive()) return
    const w = cloneWorkspace({ ...state, panes }, name.trim() || `${state.name} copy`)
    enter({ workspaces: [...store.workspaces, w], activeId: w.id }, w)
  }

  const renameWorkspace = (id: string, name: string) => {
    const trimmed = name.trim()
    if (!trimmed) return
    setStore((s) => ({ ...s, workspaces: s.workspaces.map((w) => (w.id === id ? { ...w, name: trimmed } : w)) }))
  }

  const deleteWorkspace = (id: string) => {
    // the last one never goes - an empty switcher has no way back
    if (store.workspaces.length < 2) return
    const at = store.workspaces.findIndex((w) => w.id === id)
    if (at < 0) return
    const rest = store.workspaces.filter((w) => w.id !== id)
    if (id !== store.activeId) {
      setStore({ workspaces: rest, activeId: store.activeId })
      return
    }
    if (!leaveActive()) return
    const heir = rest[at] ?? rest[at - 1] ?? rest[0]
    const pane = heir.panes.find((p) => p.id === heir.focusedPaneId) ?? heir.panes[0]
    const tab = pane.tabs.find((t) => t.id === pane.activeId) ?? pane.tabs[0]
    setPendingNav(tab.path)
    setStore({ workspaces: rest, activeId: heir.id })
    allowNextNavigation()
    navigate(tab.path)
  }

  return (
    <TabsContext.Provider
      value={{
        panes,
        layout: state.layout,
        focusedPaneId: state.focusedPaneId,
        tabs: focusedPane.tabs,
        activeId: focusedPane.activeId,
        routerLagging,
        activate,
        focusPane,
        close,
        open,
        moveTab,
        splitWithTab,
        mergePanes,
        resizeSplit,
        workspaces: store.workspaces.map((w) => ({ id: w.id, name: w.name })),
        workspaceId: state.id,
        workspaceName: state.name,
        switchWorkspace,
        createWorkspace,
        duplicateWorkspace,
        renameWorkspace,
        deleteWorkspace,
        nextWorkspaceName,
      }}
    >
      {children}
    </TabsContext.Provider>
  )
}

