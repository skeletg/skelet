import { usePreference } from '@/lib/admin-prefs'

// Sidebar groups this admin has hidden, by group id
//
// Only the sidebar honours it: ⌘K, the landing preference and direct links all still reach the
// pages, so hiding is tidying rather than a second permission system - and Preferences stays
// reachable from the user menu even with every group hidden
export const HIDDEN_GROUPS_PREF_KEY = 'nav:hiddenGroups'

export function useHiddenGroups() {
  const [hidden, setHidden] = usePreference<string[]>(HIDDEN_GROUPS_PREF_KEY, [])
  return {
    hidden,
    isHidden: (id: string) => hidden.includes(id),
    toggle: (id: string) =>
      setHidden((prev) => {
        const next = prev.includes(id) ? prev.filter((h) => h !== id) : [...prev, id]
        return next.length ? next : undefined // nothing hidden keeps no row
      }),
    showAll: () => setHidden(undefined),
  }
}
