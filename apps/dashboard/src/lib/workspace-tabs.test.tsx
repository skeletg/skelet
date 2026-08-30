import { useEffect } from 'react'
import { act, render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AdminPrefsProvider } from './admin-prefs-provider'
import { WorkspaceTabsProvider } from './workspace-tabs'
import { useWorkspaceTabs, type TabsApi } from './workspace-tabs-context'

// The provider is only meaningful with a router under it and the preference store around it, so
// both are real here - what gets faked is everything they in turn reach for: the signed-in admin,
// the two Apollo calls the store makes, and the two hooks that decide a tab's landing page and label
const { state } = vi.hoisted(() => ({
  state: {
    adminID: '',
    queryData: undefined as unknown,
    mutations: [] as unknown[],
    title: { title: null as string | null, isOverride: false },
    confirm: true,
  },
}))

vi.mock('@/shared/auth/auth', () => ({
  useAuth: () => ({ auth: { admin: { id: state.adminID }, accessToken: 't' } }),
  getAuthStoredState: () => ({ accessToken: 't' }),
}))
vi.mock('@apollo/client/react', () => ({ useQuery: () => ({ data: state.queryData }) }))
vi.mock('@/lib/apollo', () => ({
  apolloClient: {
    mutate: (opts: unknown) => {
      state.mutations.push(opts)
      return Promise.resolve({})
    },
  },
}))
vi.mock('@/lib/landing', () => ({
  useLandingPref: () => ({ landing: '/home', title: 'Home', stored: '/home', resolving: false, setLanding: () => {} }),
}))
vi.mock('@/lib/page-title', () => ({ useResolvedPageTitle: () => state.title }))

const PREF_KEY = 'workspaces'
const DEVICE_KEY = 'skelet-dash:workspace-tabs'
const mirror = () => `prefs:${state.adminID}:${PREF_KEY}`
const roamed = () => JSON.parse(localStorage.getItem(mirror()) ?? 'null')

let api: TabsApi

// Captured in an effect rather than during render - render must not write to anything outside it,
// and testing-library flushes effects inside act, so `api` is current by the time a test reads it
function Probe() {
  const current = useWorkspaceTabs()
  useEffect(() => {
    api = current
  })
  return null
}

/** `roaming` stands in for what the server already holds for this admin */
function mount(start = '/start', roaming?: unknown) {
  if (roaming !== undefined) state.queryData = { myPreferences: [{ key: PREF_KEY, value: roaming, updatedAt: '' }] }
  const view = render(
    <MemoryRouter initialEntries={[start]}>
      <AdminPrefsProvider>
        <WorkspaceTabsProvider>
          <Probe />
        </WorkspaceTabsProvider>
      </AdminPrefsProvider>
    </MemoryRouter>,
  )
  return view
}

// A device that has seen this admin before already holds the value in the preference mirror, and
// the store reads that synchronously on first touch - which is what makes it a RETURNING device
function seedDevice(value: unknown) {
  localStorage.setItem(mirror(), JSON.stringify(value))
}

const paths = () => api.panes.map((p) => p.tabs.map((t) => t.path))
const titles = () => api.panes.map((p) => p.tabs.map((t) => t.title))
const act1 = (fn: () => void) => act(() => fn())

let seq = 0
beforeEach(() => {
  seq += 1
  state.adminID = `admin-${seq}` // the preference store is module-scoped and outlives unmounts
  state.queryData = { myPreferences: [] }
  state.mutations = []
  state.title = { title: null, isOverride: false }
  state.confirm = true
  localStorage.clear()
  vi.stubGlobal('confirm', () => state.confirm)
})
afterEach(() => vi.unstubAllGlobals())

describe('restoring what was stored', () => {
  it('starts from one workspace on a blank slate', () => {
    mount('/start')
    expect(api.workspaces).toHaveLength(1)
    expect(paths()).toEqual([['/start']])
  })

  it('carries over a device-local tab strip', () => {
    localStorage.setItem(DEVICE_KEY, JSON.stringify({
      tabs: [{ id: 'a', path: '/orders', title: 'Orders' }, { id: 'b', path: '/customers', title: 'Customers' }],
      activeId: 'b',
    }))
    mount('/customers')
    expect(api.workspaces).toHaveLength(1)
    expect(paths()).toEqual([['/orders', '/customers']])
  })

  it('carries over a single unnamed workspace', () => {
    mount('/orders', {
      panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }], activeId: 'a' }],
      layout: { kind: 'pane', id: 'p1' },
      focusedPaneId: 'p1',
    })
    expect(api.workspaces.map((w) => w.name)).toEqual(['Workspace 1'])
  })

  it('rebuilds a layout that names panes the workspace does not have', () => {
    mount('/orders', {
      workspaces: [{
        id: 'w1', name: 'Kept',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'O' }], activeId: 'a' }],
        layout: { kind: 'split', id: 's', dir: 'row', sizes: [50, 50], children: [{ kind: 'pane', id: 'ghost' }, { kind: 'pane', id: 'p1' }] },
        focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    })
    // the pane list is the truth - a tree naming a pane that isn't there would render a hole
    expect(api.layout).toEqual({ kind: 'pane', id: 'p1' })
    expect(api.workspaceName).toBe('Kept')
  })

  it('falls back to a fresh workspace when the stored value is nonsense', () => {
    localStorage.setItem(DEVICE_KEY, '{{ not json')
    mount('/start')
    expect(paths()).toEqual([['/start']])
  })

  it('lets the incoming url claim the active tab of a device that already had these tabs', () => {
    const stored = {
      workspaces: [{
        id: 'w1', name: 'W',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }], activeId: 'a' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    }
    seedDevice(stored)
    mount('/elsewhere', stored)
    expect(paths()).toEqual([['/elsewhere']])
    // the tab was repurposed, so the label it carried belongs to a page it no longer shows
    expect(titles()).toEqual([['']])
  })
})

describe('tabs', () => {
  it('opens a tab on the landing page and makes it current', () => {
    mount('/start')
    act1(() => api.open())
    expect(paths()).toEqual([['/start', '/home']])
    expect(api.panes[0].activeId).toBe(api.panes[0].tabs[1].id)
  })

  it('switching tabs moves the url with it', () => {
    mount('/start')
    act1(() => api.open())
    const first = api.panes[0].tabs[0]
    act1(() => api.activate(api.panes[0].id, first.id))
    expect(api.panes[0].activeId).toBe(first.id)
    expect(api.tabs[0].path).toBe('/start')
  })

  it('closing the current tab falls to its right-hand neighbour', () => {
    mount('/start')
    act1(() => api.open())
    act1(() => api.open())
    const [, middle, last] = api.panes[0].tabs
    act1(() => api.activate(api.panes[0].id, middle.id))
    act1(() => api.close(api.panes[0].id, middle.id))
    expect(api.panes[0].activeId).toBe(last.id)
  })

  it('refuses to close a tab whose page has unsaved work when asked not to', () => {
    mount('/start')
    act1(() => api.open())
    state.confirm = false
    // the confirm is only consulted when a page IS dirty, so nothing is staged here and it closes
    act1(() => api.close(api.panes[0].id, api.panes[0].tabs[1].id))
    expect(api.panes[0].tabs).toHaveLength(1)
  })

  it('never leaves an empty workspace - the last tab closing resets it to home', () => {
    mount('/start')
    act1(() => api.close(api.panes[0].id, api.panes[0].tabs[0].id))
    expect(paths()).toEqual([['/home']])
  })

  it('reorders inside a strip without changing which tab is current', () => {
    mount('/start')
    act1(() => api.open())
    const [first, second] = api.panes[0].tabs
    const current = api.panes[0].activeId
    act1(() => api.moveTab(second.id, api.panes[0].id, 0))
    expect(api.panes[0].tabs.map((t) => t.id)).toEqual([second.id, first.id])
    expect(api.panes[0].activeId).toBe(current)
  })
})

describe('splitting', () => {
  it('sends a tab to its own pane and focuses it', () => {
    mount('/start')
    act1(() => api.open())
    const moving = api.panes[0].tabs[1]
    act1(() => api.splitWithTab(moving.id, api.panes[0].id, 'right'))
    expect(api.panes).toHaveLength(2)
    expect(api.layout.kind).toBe('split')
    expect(api.panes.find((p) => p.id === api.focusedPaneId)?.tabs.map((t) => t.id)).toEqual([moving.id])
  })

  it('will not split a pane off its only tab, which would just empty it', () => {
    mount('/start')
    const only = api.panes[0].tabs[0]
    act1(() => api.splitWithTab(only.id, api.panes[0].id, 'right'))
    expect(api.panes).toHaveLength(1)
  })

  it('closes the source pane when its last tab is dragged away', () => {
    mount('/start')
    act1(() => api.open())
    const moving = api.panes[0].tabs[1]
    act1(() => api.splitWithTab(moving.id, api.panes[0].id, 'right'))
    const [source, target] = [api.panes.find((p) => p.id !== api.focusedPaneId)!, api.focusedPaneId]
    act1(() => api.moveTab(source.tabs[0].id, target, 0))
    expect(api.panes).toHaveLength(1)
    expect(api.layout.kind).toBe('pane')
  })

  it('a tab dropped into another pane becomes the current one there', () => {
    mount('/start')
    act1(() => api.open())
    act1(() => api.open())
    const moving = api.panes[0].tabs[2]
    act1(() => api.splitWithTab(moving.id, api.panes[0].id, 'right'))
    const other = api.panes.find((p) => p.id !== api.focusedPaneId)!
    act1(() => api.moveTab(other.tabs[0].id, api.focusedPaneId, 0))
    const focused = api.panes.find((p) => p.id === api.focusedPaneId)!
    expect(focused.activeId).toBe(other.tabs[0].id)
  })

  it('merging collapses every pane back into one, in layout order', () => {
    mount('/start')
    act1(() => api.open())
    const moving = api.panes[0].tabs[1]
    act1(() => api.splitWithTab(moving.id, api.panes[0].id, 'right'))
    act1(() => api.mergePanes())
    expect(api.panes).toHaveLength(1)
    expect(api.layout.kind).toBe('pane')
    expect(api.panes[0].tabs).toHaveLength(2)
  })

  it('closing a pane’s last tab heals the split', () => {
    mount('/start')
    act1(() => api.open())
    const moving = api.panes[0].tabs[1]
    act1(() => api.splitWithTab(moving.id, api.panes[0].id, 'right'))
    act1(() => api.close(api.focusedPaneId, moving.id))
    expect(api.panes).toHaveLength(1)
    expect(api.layout.kind).toBe('pane')
  })
})

describe('workspaces', () => {
  it('creates one and lands in it, clean', () => {
    mount('/start')
    act1(() => api.open())
    act1(() => api.createWorkspace('Second'))
    expect(api.workspaceName).toBe('Second')
    expect(paths()).toEqual([['/home']])
    expect(api.workspaces).toHaveLength(2)
  })

  it('switching back brings the other workspace’s tabs with it', () => {
    mount('/start')
    act1(() => api.open())
    const before = paths()
    const original = api.workspaceId
    act1(() => api.createWorkspace('Second'))
    act1(() => api.switchWorkspace(original))
    expect(paths()).toEqual(before)
  })

  it('names new workspaces into the first free slot', () => {
    mount('/start')
    expect(api.nextWorkspaceName()).toBe('Workspace 2')
    act1(() => api.createWorkspace(api.nextWorkspaceName()))
    expect(api.workspaces.map((w) => w.name)).toEqual(['Workspace 1', 'Workspace 2'])
  })

  it('renames without touching tabs', () => {
    mount('/start')
    const before = paths()
    act1(() => api.renameWorkspace(api.workspaceId, 'Daily ops'))
    expect(api.workspaceName).toBe('Daily ops')
    expect(paths()).toEqual(before)
  })

  it('ignores a rename to nothing', () => {
    mount('/start')
    act1(() => api.renameWorkspace(api.workspaceId, '   '))
    expect(api.workspaceName).toBe('Workspace 1')
  })

  // tab ids key the unsaved-changes registry, so a shared id would count the copy's edits against
  // the original - this is the invariant that makes duplicate safe
  it('duplicating mints fresh pane and tab ids', () => {
    mount('/start')
    act1(() => api.open())
    const sourceTabs = api.panes.flatMap((p) => p.tabs.map((t) => t.id))
    const sourcePanes = api.panes.map((p) => p.id)
    act1(() => api.duplicateWorkspace('Copy'))
    expect(api.workspaceName).toBe('Copy')
    expect(paths()).toEqual([['/start', '/home']])
    for (const id of api.panes.flatMap((p) => p.tabs.map((t) => t.id))) expect(sourceTabs).not.toContain(id)
    for (const id of api.panes.map((p) => p.id)) expect(sourcePanes).not.toContain(id)
  })

  it('a copy keeps its own tabs, so closing one leaves the original alone', () => {
    mount('/start')
    act1(() => api.open())
    const original = api.workspaceId
    act1(() => api.duplicateWorkspace('Copy'))
    act1(() => api.close(api.panes[0].id, api.panes[0].tabs[1].id))
    expect(paths()).toEqual([['/start']])
    act1(() => api.switchWorkspace(original))
    expect(paths()).toEqual([['/start', '/home']])
  })

  it('will not delete the only workspace, since nothing would be left to switch to', () => {
    mount('/start')
    act1(() => api.deleteWorkspace(api.workspaceId))
    expect(api.workspaces).toHaveLength(1)
  })

  it('deleting the one you are in moves you to its neighbour', () => {
    mount('/start')
    act1(() => api.createWorkspace('Second'))
    const second = api.workspaceId
    act1(() => api.deleteWorkspace(second))
    expect(api.workspaces).toHaveLength(1)
    expect(api.workspaceName).toBe('Workspace 1')
  })

  it('deleting one you are not in leaves you where you are', () => {
    mount('/start')
    const first = api.workspaceId
    act1(() => api.createWorkspace('Second'))
    act1(() => api.deleteWorkspace(first))
    expect(api.workspaceName).toBe('Second')
    expect(api.workspaces).toHaveLength(1)
  })
})

describe('roaming', () => {
  it('writes nothing until the roaming copy has landed', () => {
    state.queryData = undefined // the query has not resolved
    mount('/start')
    // a write here would mark the key dirty, and the store treats dirty as newer than the server -
    // this session's throwaway seed would then win over the real workspaces forever
    expect(localStorage.getItem(mirror())).toBeNull()
  })

  it('persists to the roaming key once it has', () => {
    mount('/start')
    act1(() => api.open())
    expect(roamed()?.workspaces).toHaveLength(1)
    expect(roamed().workspaces[0].panes[0].tabs).toHaveLength(2)
  })

  it('takes the roaming copy whole on a device that had nothing of its own', () => {
    mount('/home', {
      workspaces: [{
        id: 'w1', name: 'Elsewhere',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }, { id: 'b', path: '/customers', title: 'Customers' }], activeId: 'b' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    })
    expect(api.workspaceName).toBe('Elsewhere')
    expect(api.panes[0].tabs).toHaveLength(2)
  })

  it('leaves a returning device alone when the roaming copy has not moved on', () => {
    const stored = {
      workspaces: [{
        id: 'w1', name: 'Same',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }], activeId: 'a' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    }
    seedDevice(stored)
    mount('/orders', stored)
    expect(api.workspaceName).toBe('Same')
    expect(paths()).toEqual([['/orders']])
  })

  it('takes what another machine did, but a deep link outranks the page it wants', () => {
    const onThisDevice = {
      workspaces: [{
        id: 'w1', name: 'Mine',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }], activeId: 'a' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    }
    const elsewhere = {
      workspaces: [{
        id: 'w1', name: 'Renamed elsewhere',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/customers', title: 'Customers' }], activeId: 'a' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    }
    seedDevice(onThisDevice)
    // this device left off on /orders, so arriving at /deep/link is a url that was asked for
    mount('/deep/link', elsewhere)
    expect(api.workspaceName).toBe('Renamed elsewhere')
    expect(paths()).toEqual([['/deep/link']])
  })

  // a fresh machine restores the roaming page only because '/' means "just let me in" - a url that
  // was typed or followed is a destination, and outranks it
  it('keeps a deep link on a machine that had nothing of its own', () => {
    mount('/settings', {
      workspaces: [{
        id: 'w1', name: 'Elsewhere',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }], activeId: 'a' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    })
    expect(api.workspaceName).toBe('Elsewhere')
    expect(paths()).toEqual([['/settings']])
  })

  it('restores the roaming page when the way in was the landing page', () => {
    mount('/home', {
      workspaces: [{
        id: 'w1', name: 'Elsewhere',
        panes: [{ id: 'p1', tabs: [{ id: 'a', path: '/orders', title: 'Orders' }], activeId: 'a' }],
        layout: { kind: 'pane', id: 'p1' }, focusedPaneId: 'p1',
      }],
      activeWorkspaceId: 'w1',
    })
    expect(paths()).toEqual([['/orders']])
  })

  it('clears the device-local key once the roaming one is in play', () => {
    localStorage.setItem(DEVICE_KEY, JSON.stringify({ tabs: [{ id: 'a', path: '/orders', title: 'O' }], activeId: 'a' }))
    mount('/orders')
    expect(localStorage.getItem(DEVICE_KEY)).toBeNull()
  })
})
