import { Trans } from '@lingui/react/macro'
import { Fragment } from 'react'
import { PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { type WorkspacePane, type WorkspaceTab } from '@/lib/workspace-tabs'
import { useWorkspaceTabs } from '@/lib/workspace-tabs-context'
import { StripTail } from '@/components/layout/workspace-dnd'
import { useTabDrag, useWorkspaceDnd } from '@/components/layout/workspace-dnd-context'

// A pane's tab strip - click switches, middle-click or the x closes, + opens a new tab, and
// dragging a pill reorders it, moves it to another pane, or splits the workspace open
//
// Tabs are button-shaped pills, uniform width so nothing reflows on switch, and deliberately
// WITHOUT separators or active-only styling that changes element sizes - both made the row shift

function TabPill({ pane, tab, index, focused }: { pane: WorkspacePane; tab: WorkspaceTab; index: number; focused: boolean }) {
  const { activate, close } = useWorkspaceTabs()
  const { ref, listeners, isDragging, enabled } = useTabDrag(tab.id, pane.id, index, tab.title)
  const current = tab.id === pane.activeId

  return (
    <div
      ref={ref}
      {...listeners}
      data-slot="workspace-tab"
      role="tab"
      tabIndex={0}
      aria-selected={current}
      title={tab.title || tab.path}
      onClick={() => activate(pane.id, tab.id)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          activate(pane.id, tab.id)
        }
      }}
      onMouseDown={(e) => {
        if (e.button === 1) e.preventDefault() // keep middle-click from starting autoscroll
      }}
      onAuxClick={(e) => {
        if (e.button === 1) close(pane.id, tab.id)
      }}
      className={cn(
        'flex h-8 w-24 min-w-16 shrink cursor-pointer items-center gap-1.5 rounded-md border px-2 text-sm select-none sm:w-36 sm:min-w-24 sm:px-2.5',
        enabled && 'touch-none', // dragging needs the gesture, but on a phone the strip keeps its swipe-scroll
        // the pill keeps its slot while the overlay copy follows the pointer - collapsing it would
        // reflow the strip under the drag
        isDragging && 'opacity-30',
        current && focused
          ? 'border-primary/30 bg-primary/10 text-primary' // primary IS the user-customizable accent (the theme customizer labels it "Accent") - a tint of it marks the active tab without the full-strength fill shouting
          : current
            ? 'border-border bg-muted/60 text-foreground' // current in a pane that isn't focused - on screen, but not the page that owns the url
            : 'border-transparent text-muted-foreground hover:bg-muted/40 hover:text-foreground',
      )}
    >
      <span className="min-w-0 flex-1 truncate text-left">{tab.title || 'New tab'}</span>
      <button
        type="button"
        aria-label={`Close ${tab.title || 'tab'}`}
        onPointerDown={(e) => e.stopPropagation()} // the x is a click target, never a drag handle
        onClick={(e) => {
          e.stopPropagation()
          close(pane.id, tab.id)
        }}
        className="shrink-0 rounded-sm p-0.5 opacity-60 hover:bg-foreground/10 hover:opacity-100"
      >
        <XIcon className="size-3.5" />
      </button>
    </div>
  )
}

/** Marks the slot a dragged tab would drop into */
function InsertionCaret() {
  return <div className="h-6 w-0.5 shrink-0 rounded-full bg-primary" />
}

export function WorkspaceTabStrip({ paneId, tour }: { paneId?: string; tour?: boolean }) {
  const { panes, focusedPaneId, open } = useWorkspaceTabs()
  const { dragging, drop } = useWorkspaceDnd()
  const pane = panes.find((p) => p.id === (paneId ?? focusedPaneId))
  if (!pane) return null

  const focused = pane.id === focusedPaneId
  const insertAt = dragging && drop?.kind === 'strip' && drop.paneId === pane.id ? drop.index : null

  return (
    <div data-tour={tour ? 'tabs' : undefined} className="flex min-w-0 flex-1 items-center gap-1">
      <div role="tablist" className="scrollbar-none flex min-w-0 items-center gap-1 overflow-x-auto">
        {pane.tabs.map((t, i) => (
          <Fragment key={t.id}>
            {insertAt === i && <InsertionCaret />}
            <TabPill pane={pane} tab={t} index={i} focused={focused} />
          </Fragment>
        ))}
        {insertAt === pane.tabs.length && <InsertionCaret />}
        {/* inside the tablist, so it stays glued to the last pill instead of drifting to the far
            edge - the strip scrolls it into reach once the tabs outgrow the row */}
        <StripTail paneId={pane.id} count={pane.tabs.length}>
          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1.5 px-2 text-muted-foreground"
            onClick={() => open(pane.id)}
            title="New tab"
          >
            <PlusIcon className="size-4" />
            <Trans>New Tab</Trans>
          </Button>
        </StripTail>
      </div>
    </div>
  )
}

// The header's tab slot. One pane owns the whole content area, so its strip lives up here and the
// pane shows none - split the workspace and every pane grows its own strip, New Tab button and all,
// leaving this slot with nothing to add but the count
export function HeaderWorkspaceTabs() {
  const { panes } = useWorkspaceTabs()
  if (panes.length === 1) return <WorkspaceTabStrip tour />

  return (
    <div data-tour="tabs" className="flex min-w-0 flex-1 items-center">
      <span className="shrink-0 text-sm text-muted-foreground">{panes.length} panes</span>
    </div>
  )
}
