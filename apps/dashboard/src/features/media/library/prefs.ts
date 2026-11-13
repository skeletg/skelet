import type { MediaAssetSort } from '@/graphql/schema.types'

export type LibraryView = 'gallery' | 'list'
export type LibraryPrefs = { view: LibraryView; rowHeight: number; sort: MediaAssetSort }

export const ROW_MIN = 110
export const ROW_MAX = 300
const KEY = 'media-library:view'
const SORT_VALUES: MediaAssetSort[] = ['NEWEST', 'OLDEST', 'NAME', 'LARGEST']

export const SORTS: { value: MediaAssetSort; label: string }[] = [
  { value: 'NEWEST', label: 'Newest first' },
  { value: 'OLDEST', label: 'Oldest first' },
  { value: 'NAME', label: 'Name A-Z' },
  { value: 'LARGEST', label: 'Largest first' },
]

export function readPrefs(): LibraryPrefs {
  const fallback: LibraryPrefs = { view: 'gallery', rowHeight: 190, sort: 'NEWEST' }
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? '{}') as Partial<LibraryPrefs>
    const height = Number(raw.rowHeight)
    return {
      view: raw.view === 'list' ? 'list' : 'gallery',
      rowHeight: height >= ROW_MIN && height <= ROW_MAX ? height : fallback.rowHeight,
      sort: SORT_VALUES.includes(raw.sort as MediaAssetSort) ? (raw.sort as MediaAssetSort) : fallback.sort,
    }
  } catch {
    return fallback
  }
}

export function savePrefs(prefs: LibraryPrefs) {
  try {
    localStorage.setItem(KEY, JSON.stringify(prefs))
  } catch {
    // a private window keeps its choice for this visit only
  }
}
