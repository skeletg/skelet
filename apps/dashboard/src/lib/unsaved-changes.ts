import { useContext, useEffect, useRef, useState } from 'react'
import { TabPanelContext } from '@/lib/tab-panel-context'

// Every surface that can silently drop staged edits asks here first - scope switches, closing a
// workspace tab, and the browser's own close/refresh
//
// Module scoped rather than React state on purpose: the beforeunload listener has to read the set
// at fire time, and a page in a hidden tab is still mounted and still dirty

// `persisted` - the work also lives in a stored draft (lib/form-drafts), so a refresh, a close or a
// scope switch loses nothing and needn't ask - only closing its workspace tab still does
type DirtyEntry = { id: number; label: string | null; tabId: string | null; persisted: boolean }

type Scope = { includePersisted?: boolean }

const dirtyEntries = new Set<DirtyEntry>()
let dirtySeq = 0

// Mirrors the tab strip's labels, same trick as channel-store - lets a confirm name the page
// without this module reaching into the tabs context
const tabTitles = new Map<string, string>()

// Which tab the url currently belongs to - the focused pane's active tab, and so the only page an
// in-app navigation can replace - null falls back to asking app-wide, which over-prompts rather
// than under-prompts if a caller forgets to pass it
let urlOwningTabId: string | null = null

export function mirrorTabTitles(
  tabs: ReadonlyArray<{ id: string; title: string }>,
  urlOwner: string | null = null,
): void {
  tabTitles.clear()
  for (const tab of tabs) if (tab.title) tabTitles.set(tab.id, tab.title)
  urlOwningTabId = urlOwner
}

export function urlOwningTab(): string | undefined {
  return urlOwningTabId ?? undefined
}

function labelFor(entry: DirtyEntry): string {
  if (entry.label) return entry.label
  const title = entry.tabId ? tabTitles.get(entry.tabId) : undefined
  return title ?? 'Unsaved changes'
}

// No tabId means "anywhere" - an entry with a null tabId came from a bare render (tests) and only
// ever counts app-wide, never against a specific tab
function entriesFor(tabId?: string, scope: Scope = {}): DirtyEntry[] {
  const all = [...dirtyEntries].filter((entry) => scope.includePersisted || !entry.persisted)
  return tabId === undefined ? all : all.filter((entry) => entry.tabId === tabId)
}

export function hasUnsavedChanges(tabId?: string, scope?: Scope): boolean {
  return entriesFor(tabId, scope).length > 0
}

// One line per PAGE, not per registration - a page can register twice (its staged-save sections
// plus an explicit guard of its own, which Settings does) and naming it twice reads like two pages
export function unsavedLabels(tabId?: string, scope?: Scope): string[] {
  const perTab = new Map<string, DirtyEntry[]>()
  const untabbed: DirtyEntry[] = []
  for (const entry of entriesFor(tabId, scope)) {
    if (!entry.tabId) {
      untabbed.push(entry)
      continue
    }
    const found = perTab.get(entry.tabId)
    if (found) found.push(entry)
    else perTab.set(entry.tabId, [entry])
  }

  const seen = new Set<string>()
  const out: string[] = []
  const add = (label: string) => {
    if (seen.has(label)) return
    seen.add(label)
    out.push(label)
  }
  // An explicit label wins - the page knows what it is better than the tab strip does
  for (const entries of perTab.values()) add(labelFor(entries.find((e) => e.label) ?? entries[0]))
  for (const entry of untabbed) add(labelFor(entry))
  return out
}

function listed(labels: string[]): string {
  return labels.length > 1 ? `\n\n• ${labels.join('\n• ')}` : ` (${labels[0]})`
}

/**
 * `label` describes what is dirty (e.g. "Product 'Acme Widget'") and shows up in the confirm -
 * left off, the page's workspace-tab title stands in - the registration clears on unmount, so
 * navigating away is automatically forgiven
 *
 * `persisted` marks work a stored draft already keeps (shared/saving/usePersistedForm) - it only
 * prompts when its workspace tab is closed
 */
export function useUnsavedChangesGuard(
  isDirty: boolean,
  label?: string,
  opts: { persisted?: boolean } = {},
): void {
  const persisted = opts.persisted ?? false
  const { id: tabId } = useContext(TabPanelContext)
  const entryRef = useRef<DirtyEntry | null>(null)
  useEffect(() => {
    if (!isDirty) {
      if (entryRef.current) {
        dirtyEntries.delete(entryRef.current)
        entryRef.current = null
      }
      return
    }
    if (!entryRef.current) {
      entryRef.current = { id: ++dirtySeq, label: label ?? null, tabId, persisted }
      dirtyEntries.add(entryRef.current)
    } else {
      entryRef.current.label = label ?? null
      entryRef.current.tabId = tabId
      entryRef.current.persisted = persisted
    }
    return () => {
      if (entryRef.current) {
        dirtyEntries.delete(entryRef.current)
        entryRef.current = null
      }
    }
  }, [isDirty, label, tabId, persisted])
}

/**
 * Dirty means "these values moved off what they mounted with" - for pages with no loaded entity to
 * diff against - create forms, and editors that mount with their record already in props
 *
 * NOT for state seeded asynchronously after mount: the baseline would be the pre-load blanks, so
 * it would read as dirty the moment the real values landed - those want a diff against what loaded
 */
export function useUnsavedFields(values: readonly unknown[], label?: string): void {
  const current = JSON.stringify(values)
  const [initial] = useState(current) // the first render's values, kept for the life of the form
  useUnsavedChangesGuard(current !== initial, label)
}

export function confirmScopeSwitch(): boolean {
  if (!hasUnsavedChanges()) return true
  return window.confirm(
    `You have unsaved changes${listed(unsavedLabels())}\n\nSwitching scope will discard them. Continue?`,
  )
}

// Scoped to the one tab - sibling tabs stay mounted and keep their edits, so their dirty state is
// none of this prompt's business - persisted drafts count here: the tab id they're stored under dies
// with the tab, so closing it is the one move that really loses them
export function confirmCloseTab(tabId: string): boolean {
  const scope = { includePersisted: true }
  if (!hasUnsavedChanges(tabId, scope)) return true
  return window.confirm(
    `You have unsaved changes${listed(unsavedLabels(tabId, scope))}\n\nClosing this tab will discard them. Continue?`,
  )
}

// Switching workspace unmounts every page in the one being left, all at once - so unlike closing a
// single tab, this prompt has to speak for the whole set
export function confirmSwitchWorkspace(tabIds: readonly string[]): boolean {
  const labels = [...new Set(tabIds.flatMap((id) => unsavedLabels(id)))]
  if (labels.length === 0) return true
  return window.confirm(
    `You have unsaved changes${listed(labels)}\n\nSwitching workspace will discard them. Continue?`,
  )
}

// In-app navigation replaces only the page the url belongs to - sibling tabs and other panes stay
// mounted with their edits intact, so their dirt is not this prompt's business either
export function confirmLeavePage(): boolean {
  const tabId = urlOwningTab()
  if (!hasUnsavedChanges(tabId)) return true
  return window.confirm(
    `You have unsaved changes${listed(unsavedLabels(tabId))}\n\nLeaving this page will discard them. Continue?`,
  )
}

// Close and refresh get the browser's own wording - preventDefault is the only lever a page has
export function installUnloadGuard(): () => void {
  const onBeforeUnload = (event: BeforeUnloadEvent) => {
    if (!hasUnsavedChanges()) return
    event.preventDefault()
    event.returnValue = '' // still what older browsers gate the prompt on
  }
  window.addEventListener('beforeunload', onBeforeUnload)
  return () => window.removeEventListener('beforeunload', onBeforeUnload)
}

export function useUnloadGuard(): void {
  useEffect(installUnloadGuard, [])
}
