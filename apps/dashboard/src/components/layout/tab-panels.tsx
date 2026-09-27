import {
  Fragment,
  Suspense,
  memo,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import {
  Route,
  Routes,
  parsePath,
  useLocation,
  UNSAFE_LocationContext,
  type Location,
} from 'react-router-dom'
import { allRouterRoutes } from '@/nav/registry'
import type { NavLeaf } from '@/nav/types'
import { type WorkspacePane, type WorkspaceTab } from '@/lib/workspace-tabs'
import { useWorkspaceTabs } from '@/lib/workspace-tabs-context'
import type { LayoutNode, SplitNode } from '@/lib/workspace-layout'
import { TabPanelContext } from '@/lib/tab-panel-context'
import { WorkspaceTabStrip } from '@/components/layout/workspace-tab-strip'
import { PaneDropZone } from '@/components/layout/workspace-dnd'
import { NotFound } from '@/shared/errors/NotFound'
import { RequireFeature } from '@/shared/errors/RequireFeature'
import { RequirePermission } from '@/shared/errors/RequirePermission'
import { RouteErrorBoundary } from '@/shared/errors/RouteErrorBoundary'

// One mounted page tree PER workspace tab, hidden with plain display:none so switching tabs is a
// pure show/hide - deliberately NOT React's <Activity>: it pauses effects, which makes Apollo
// unsubscribe every query, and with the app-wide network-only fetch policy re-showing a tab then
// refetched everything through a loading state - exactly the "reload" this exists to avoid
//
// Instead each panel keeps its effects (and query subscriptions) warm and gets its OWN location
// context, frozen at the tab's path while it isn't the live one - so background pages never see
// the focused tab's url, their query variables can't churn, and useLocation/useSearchParams/
// useParams stay coherent inside every panel

/** Suspense fallback for lazy pages - pages usually resolve fast enough that anything louder is worse */
function PageLoading() {
  return (
    <div className="flex h-64 items-center justify-center">
      <div className="size-6 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" />
    </div>
  )
}

// The full leaf-route table - the same wrapping every page got when these routes lived in App.tsx: error boundary, lazy-chunk spinner,
// permission gate - matching runs against the panel's location context, live for the focused tab and frozen for the rest
// The feature switch outranks the grant - a switched-off suite says so rather than asking for a permission it can't honour
function gateRoute(leaf: NavLeaf) {
  const page = leaf.permission ? (
    <RequirePermission permission={leaf.permission}>{leaf.element}</RequirePermission>
  ) : (
    leaf.element
  )
  return leaf.feature ? <RequireFeature feature={leaf.feature}>{page}</RequireFeature> : page
}

function TabRoutes() {
  const leaves = allRouterRoutes()
  return (
    <Routes>
      {leaves.map((leaf) => (
        <Route
          key={leaf.path}
          path={leaf.path}
          element={
            <RouteErrorBoundary key={leaf.path}>
              <Suspense fallback={<PageLoading />}>
                {gateRoute(leaf)}
              </Suspense>
            </RouteErrorBoundary>
          }
        />
      ))}
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}

type PanelProps = {
  tab: WorkspaceTab
  paneId: string
  host: HTMLElement | null
  live: boolean
  visible: boolean
  lagging: boolean
}

// memo matters here: dragging a split divider re-renders the layout every frame, and without it
// every mounted page in the workspace would re-render along with it
const TabPanel = memo(function TabPanel({ tab, paneId, host, live, visible, lagging }: PanelProps) {
  const parent = useContext(UNSAFE_LocationContext)

  const frozen = useMemo<Location>(() => {
    const p = parsePath(tab.path)
    return { pathname: p.pathname ?? '/', search: p.search ?? '', hash: p.hash ?? '', state: null, key: tab.id }
  }, [tab.path, tab.id])

  // while a tab-switch navigation is still in flight the router reports the OLD page - the newly
  // live panel keeps its frozen location for that beat so the wrong page can't flash through it
  const value = useMemo(
    () => ({
      location: live && !lagging ? parent.location : frozen,
      navigationType: parent.navigationType,
    }),
    [live, lagging, parent, frozen],
  )

  const panel = useMemo(
    () => ({ active: live, visible, id: tab.id, paneId }),
    [live, visible, tab.id, paneId],
  )

  // The portal target is created ONCE per tab and then moved between panes by hand. A portal whose
  // container prop changes gets a brand new fiber - React would remount the whole page and undo
  // everything this file exists for - but re-parenting the node behind React's back is invisible to it
  const [mount] = useState(() => {
    const el = document.createElement('div')
    el.style.display = 'contents' // a pass-through wrapper, so the panel still lays out against the pane
    return el
  })

  const scroller = useRef<HTMLDivElement | null>(null)
  const offset = useRef(0)

  useLayoutEffect(() => {
    if (!host) return
    host.appendChild(mount)
    // detaching a subtree drops its scroll offsets, so the page lands back where it was left
    if (scroller.current) scroller.current.scrollTop = offset.current
    return () => mount.remove()
  }, [host, mount])

  if (!host) return null

  return createPortal(
    <UNSAFE_LocationContext.Provider value={value}>
      <TabPanelContext.Provider value={panel}>
        {/* hidden panels stay LAID OUT (absolute + invisible, not display:none) so their scroll
            offsets and rendered sizes survive the hide - display:none collapses scrollTop to 0
            content-visibility:hidden is the load-bearing part: `invisible` alone let descendants
            with transition-all (every shadcn button) keep computed visibility for their
            ~150ms transition, ghosting the old page over the new tab - paint containment skips
            the subtree unconditionally, while still preserving its rendering state */}
        <div
          ref={scroller}
          onScroll={(e) => {
            offset.current = e.currentTarget.scrollTop
          }}
          aria-hidden={!visible}
          inert={!visible}
          className={cn(
            'scrollbar-accent flex min-h-0 flex-1 flex-col overflow-y-auto p-4 md:p-6',
            !visible && 'pointer-events-none invisible absolute inset-0 [content-visibility:hidden]',
          )}
        >
          <TabRoutes />
        </div>
      </TabPanelContext.Provider>
    </UNSAFE_LocationContext.Provider>,
    mount,
  )
})

type TreeProps = {
  panes: WorkspacePane[]
  focusedPaneId: string
  split: boolean
  registerHost: (paneId: string, el: HTMLElement | null) => void
}

function PaneView({ pane, focusedPaneId, split, registerHost }: TreeProps & { pane: WorkspacePane }) {
  const { focusPane } = useWorkspaceTabs()
  const focused = pane.id === focusedPaneId
  const hostRef = useCallback(
    (el: HTMLDivElement | null) => registerHost(pane.id, el),
    [pane.id, registerHost],
  )

  // A NATIVE listener, not React's onPointerDownCapture: every page is portaled into this pane, and
  // React replays events along its own tree, where the panels are siblings of the layout rather than
  // children of it - so a React handler here never saw a click that landed on the page itself, and
  // only the pane's own chrome could take focus
  const frame = useRef<HTMLDivElement>(null)
  // focusPane is a fresh closure every provider render, so it rides a ref rather than re-binding the
  // listener each time - focusing the pane that already has focus is a no-op inside activate anyway
  const focus = useRef(focusPane)
  useEffect(() => {
    focus.current = focusPane
  })
  useEffect(() => {
    const el = frame.current
    if (!el) return
    const onDown = () => focus.current(pane.id)
    el.addEventListener('pointerdown', onDown, { capture: true })
    return () => el.removeEventListener('pointerdown', onDown, { capture: true })
  }, [pane.id])

  return (
    <div
      ref={frame}
      data-slot="workspace-pane"
      className={cn(
        'flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden',
        split && 'rounded-md border-2',
        split && focused && 'border-primary/50',
      )}
    >
      {/* a lone pane hands its strip to the app header instead - see DashboardLayout */}
      {split && (
        <div className="flex shrink-0 items-center border-b px-1 py-1">
          <WorkspaceTabStrip paneId={pane.id} />
        </div>
      )}
      <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
        <div ref={hostRef} className="relative flex min-h-0 min-w-0 flex-1 flex-col" />
        <PaneDropZone paneId={pane.id} gutter={split} />
      </div>
    </div>
  )
}

function SplitView({ node, ...tree }: TreeProps & { node: SplitNode }) {
  const { resizeSplit } = useWorkspaceTabs()
  // the drag stays local and commits once on release - routing every frame through the tabs state
  // would rewrite localStorage and wake every consumer on each mousemove
  const [dragSizes, setDragSizes] = useState<number[] | null>(null)
  const box = useRef<HTMLDivElement>(null)
  const sizes = dragSizes ?? node.sizes
  const row = node.dir === 'row'

  const startResize = (i: number) => (e: ReactPointerEvent<HTMLDivElement>) => {
    const el = box.current
    if (!el || e.button !== 0) return
    e.preventDefault()
    const rect = el.getBoundingClientRect()
    const total = Math.max(row ? rect.width : rect.height, 1)
    const origin = row ? e.clientX : e.clientY
    const base = [...sizes]
    const weight = base.reduce((a, b) => a + b, 0)
    const pair = base[i - 1] + base[i]
    // neither side collapses to nothing - a real floor in px, and never more than a third of the pair
    const floor = Math.min(pair / 3, (200 / total) * weight)
    let latest = base

    const onMove = (ev: PointerEvent) => {
      const moved = (((row ? ev.clientX : ev.clientY) - origin) / total) * weight
      const first = Math.min(Math.max(base[i - 1] + moved, floor), pair - floor)
      latest = base.map((s, k) => (k === i - 1 ? first : k === i ? pair - first : s))
      setDragSizes(latest)
    }
    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      setDragSizes(null)
      resizeSplit(node.id, latest)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  return (
    // data-resizing takes the easing off the pane slots: a live drag has to track the pointer
    // exactly, and an eased flex-grow puts the divider a few frames behind the hand
    <div
      ref={box}
      data-slot="workspace-split"
      data-resizing={dragSizes ? 'true' : undefined}
      className={cn('flex min-h-0 min-w-0 flex-1 gap-0', row ? 'flex-row' : 'flex-col')}
    >
      {node.children.map((child, i) => (
        <Fragment key={child.id}>
          {i > 0 && (
            <div
              role="separator"
              aria-orientation={row ? 'vertical' : 'horizontal'}
              onPointerDown={startResize(i)}
              className={cn(
                'group flex shrink-0 touch-none items-stretch justify-center',
                row ? 'w-2 cursor-col-resize' : 'h-2 cursor-row-resize',
              )}
            >
              <div
                className={cn(
                  'rounded-full bg-transparent transition-colors group-hover:bg-primary/50',
                  row ? 'my-1 w-0.5' : 'mx-1 h-0.5',
                )}
              />
            </div>
          )}
          <div
            data-slot="workspace-pane-slot"
            style={{ flexGrow: sizes[i], flexShrink: 1, flexBasis: 0 }}
            className="flex min-h-0 min-w-0"
          >
            <LayoutView node={child} {...tree} />
          </div>
        </Fragment>
      ))}
    </div>
  )
}

function LayoutView({ node, ...tree }: TreeProps & { node: LayoutNode }) {
  if (node.kind === 'split') return <SplitView node={node} {...tree} />
  const pane = tree.panes.find((p) => p.id === node.id)
  if (!pane) return null
  return <PaneView pane={pane} {...tree} />
}

export function TabPanels() {
  useLocation() // re-render on navigation so the live panel's context value follows the router
  const { panes, layout, focusedPaneId, routerLagging } = useWorkspaceTabs()
  // Panes render first and hand their content node back here, so the panels know where to mount -
  // the extra pass is what keeps every page a sibling of the tree rather than a child of it
  const [hosts, setHosts] = useState<Record<string, HTMLElement>>({})

  const registerHost = useCallback((paneId: string, el: HTMLElement | null) => {
    setHosts((prev) => {
      if (!el) {
        if (!(paneId in prev)) return prev
        const next = { ...prev }
        delete next[paneId]
        return next
      }
      return prev[paneId] === el ? prev : { ...prev, [paneId]: el }
    })
  }, [])

  const split = panes.length > 1

  return (
    <>
      {/* Split panes sit inset from the shell and from each other, matching the dividers' own 8px -
          a lone pane still runs edge to edge, since the page inside it brings its own padding */}
      <div className={cn('flex min-h-0 min-w-0 flex-1 flex-col', split && 'p-2')}>
        <LayoutView
          node={layout}
          panes={panes}
          focusedPaneId={focusedPaneId}
          split={split}
          registerHost={registerHost}
        />
      </div>
      {panes.flatMap((pane) =>
        pane.tabs.map((tab) => (
          <TabPanel
            key={tab.id}
            tab={tab}
            paneId={pane.id}
            host={hosts[pane.id] ?? null}
            visible={tab.id === pane.activeId}
            live={pane.id === focusedPaneId && tab.id === pane.activeId}
            lagging={routerLagging}
          />
        )),
      )}
    </>
  )
}
