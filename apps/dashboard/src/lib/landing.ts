import { usePreference } from '@/lib/admin-prefs'
import { useVisiblePages } from '@/nav/use-visible-pages'

// Where the dashboard opens: on login, and for every new workspace tab. Stored as a nav leaf path and validated against the pages this
// admin can actually reach, so a revoked permission or a renamed route silently falls back to the dashboard rather than dead-ending
export const LANDING_PREF_KEY = 'ui:landing'
export const DEFAULT_LANDING = '/'

export function useLandingPref(): {
  stored: string
  landing: string
  title: string
  // true while the page list is still filling in - validating against it early would reject a
  // perfectly good stored path
  resolving: boolean
  setLanding: (path: string) => void
} {
  const [stored, setStored] = usePreference<string>(LANDING_PREF_KEY, DEFAULT_LANDING)
  const { pages, loading } = useVisiblePages()
  const match = pages.find((p) => p.path === stored)
  const landing = stored === DEFAULT_LANDING || match ? stored : DEFAULT_LANDING

  return {
    stored,
    landing,
    title: match?.title ?? 'Dashboard',
    resolving: loading,
    // storing the default just deletes the key, so an untouched account keeps no row
    setLanding: (path: string) => setStored(path === DEFAULT_LANDING ? undefined : path),
  }
}
