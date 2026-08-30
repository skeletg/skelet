import { usePreference } from '@/lib/admin-prefs'

// Pinned sidebar pages - a roaming per-admin list of nav leaf paths, in pin order
const NONE: string[] = []

export function usePinnedPaths() {
  const [pins, setPins] = usePreference<string[]>('nav:pins', NONE)
  return {
    pins,
    isPinned: (path: string) => pins.includes(path),
    toggle: (path: string) =>
      setPins((prev) => {
        const next = prev.includes(path) ? prev.filter((p) => p !== path) : [...prev, path]
        return next.length ? next : undefined // an empty list just deletes the key
      }),
  }
}
