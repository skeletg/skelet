import { useCallback, useSyncExternalStore } from 'react'

// Recently visited pages for the ⌘K palette - deliberately localStorage rather than the roaming preference store: this changes on every
// navigation, and syncing that chatter to the backend buys nothing (recents have near-zero cross-device value)

const MAX_RECENTS = 10

type Recent = { path: string; ts: number }

const subscribers = new Set<() => void>()
let cache: { adminID: string; list: Recent[] } | null = null

function storageKey(adminID: string) {
  return `recents:${adminID}`
}

function read(adminID: string): Recent[] {
  if (cache?.adminID === adminID) return cache.list
  let list: Recent[] = []
  try {
    list = JSON.parse(localStorage.getItem(storageKey(adminID)) ?? '[]') as Recent[]
  } catch {
    /* corrupt entry - start fresh */
  }
  cache = { adminID, list }
  return list
}

export function recordVisit(adminID: string, path: string) {
  if (!adminID || !path) return
  const list = [{ path, ts: Date.now() }, ...read(adminID).filter((r) => r.path !== path)].slice(
    0,
    MAX_RECENTS,
  )
  cache = { adminID, list }
  try {
    localStorage.setItem(storageKey(adminID), JSON.stringify(list))
  } catch {
    /* best-effort */
  }
  for (const cb of subscribers) cb()
}

export function useRecentPaths(adminID: string): string[] {
  const list = useSyncExternalStore(
    useCallback((cb) => {
      subscribers.add(cb)
      return () => subscribers.delete(cb)
    }, []),
    () => read(adminID),
  )
  return list.map((r) => r.path)
}
