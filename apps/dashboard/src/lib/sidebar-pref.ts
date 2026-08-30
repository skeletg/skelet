import { usePreference } from '@/lib/admin-prefs'

// Whether the sidebar sits expanded or collapsed to the icon rail
//
// shadcn's SidebarProvider writes a `sidebar_state` cookie for SSR to read back on the next render,
// which in a client-only SPA nothing ever reads - so collapsing the sidebar didn't survive a
// reload. Controlling it from here fixes that AND makes the choice roam. Mobile's overlay stays
// ephemeral: it belongs to one visit, not to the account
export const SIDEBAR_OPEN_PREF_KEY = 'ui:sidebarOpen'

export function useSidebarPref(): { open: boolean; setOpen: (open: boolean) => void } {
  const [open, setStored] = usePreference<boolean>(SIDEBAR_OPEN_PREF_KEY, true)
  // expanded is the default, so choosing it keeps no row
  return { open, setOpen: (next) => setStored(next ? undefined : false) }
}

// How wide the expanded rail sits, in px, dragged from its edge or picked in Preferences
//
// px rather than rem: the drag reports px, and storing what the pointer measured keeps the handle
// from drifting under a non-default text scale
export const SIDEBAR_WIDTH_PREF_KEY = 'ui:sidebarWidth'

export const SIDEBAR_WIDTH_DEFAULT = 272
export const SIDEBAR_WIDTH_MIN = 208
export const SIDEBAR_WIDTH_MAX = 480

// Presets give the same range a keyboard and a screen reader can reach - dragging is a mouse-only
// affordance and can't be the only way to set this
export const SIDEBAR_WIDTH_PRESETS: { label: string; value: number }[] = [
  { label: 'Compact', value: 224 },
  { label: 'Default', value: SIDEBAR_WIDTH_DEFAULT },
  { label: 'Wide', value: 320 },
  { label: 'Widest', value: 400 },
]

export function clampSidebarWidth(px: number): number {
  if (!Number.isFinite(px)) return SIDEBAR_WIDTH_DEFAULT
  return Math.round(Math.min(SIDEBAR_WIDTH_MAX, Math.max(SIDEBAR_WIDTH_MIN, px)))
}

export function useSidebarWidthPref(): {
  width: number
  setWidth: (px: number) => void
  resetWidth: () => void
} {
  const [stored, setStored] = usePreference<number>(SIDEBAR_WIDTH_PREF_KEY, SIDEBAR_WIDTH_DEFAULT)
  const width = clampSidebarWidth(stored)
  return {
    width,
    // the default keeps no row, matching how the open/collapsed pref stores itself
    setWidth: (px) => {
      const next = clampSidebarWidth(px)
      setStored(next === SIDEBAR_WIDTH_DEFAULT ? undefined : next)
    },
    resetWidth: () => setStored(undefined),
  }
}
