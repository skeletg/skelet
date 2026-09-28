// Unsaved form work, kept in localStorage so a refresh or a closed browser doesn't throw it away -
// read and written by shared/saving/usePersistedForm, swept here
//
// Keyed admin -> workspace tab -> page, so a draft only ever comes back in the tab it was typed in,
// for the admin who typed it. Only the fields that moved are stored, each with the value it moved
// FROM - that's what lets a restore tell "the server didn't change" from "someone else saved too"
//
// Every storage touch is wrapped: private mode, a full quota or blocked site data just means no
// persistence, never a broken page

const PREFIX = 'skelet.draft.'
const VERSION = 1
export const DRAFT_TTL_MS = 7 * 24 * 60 * 60 * 1000

export type TouchedField = { base: unknown; value: unknown }

export type StoredDraft = {
  v: typeof VERSION
  savedAt: number
  touched: Record<string, TouchedField>
}

export function draftKey(adminId: string, tabId: string | null, routeKey: string): string {
  return `${PREFIX}${adminId}.${tabId ?? '_'}.${routeKey}`
}

// Deep equality for plain form values - both sides come out of the same mappers, so key order matches
export function sameValue(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

// The top-level fields of `values` that differ from `baseline`, minus the ones never to be stored
export function touchedFields<T extends object>(
  baseline: T,
  values: T,
  exclude: readonly string[] = [],
): Record<string, TouchedField> {
  const out: Record<string, TouchedField> = {}
  for (const key of Object.keys(values)) {
    if (exclude.includes(key)) continue
    const base = (baseline as Record<string, unknown>)[key]
    const value = (values as Record<string, unknown>)[key]
    if (!sameValue(base, value)) out[key] = { base, value }
  }
  return out
}

export function readDraft(key: string, now = Date.now()): StoredDraft | null {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<StoredDraft>
    if (parsed.v !== VERSION || typeof parsed.savedAt !== 'number' || !parsed.touched) {
      localStorage.removeItem(key)
      return null
    }
    if (now - parsed.savedAt > DRAFT_TTL_MS) {
      localStorage.removeItem(key)
      return null
    }
    return parsed as StoredDraft
  } catch {
    return null
  }
}

// An empty field set clears instead - a draft that matches the baseline is no draft at all
export function writeDraft(key: string, touched: Record<string, TouchedField>, now = Date.now()): void {
  try {
    if (Object.keys(touched).length === 0) {
      localStorage.removeItem(key)
      return
    }
    const draft: StoredDraft = { v: VERSION, savedAt: now, touched }
    localStorage.setItem(key, JSON.stringify(draft))
  } catch {
    /* no persistence this time */
  }
}

export function clearDraft(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch {
    /* nothing to clear */
  }
}

function draftKeys(): string[] {
  const out: string[] = []
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (key?.startsWith(PREFIX)) out.push(key)
    }
  } catch {
    /* unreadable storage holds no drafts we can reach */
  }
  return out
}

// [adminId, tabId] out of a key - both are dot-free ids, the route key after them may not be
function owners(key: string): [string, string] {
  const [adminId = '', tabId = ''] = key.slice(PREFIX.length).split('.')
  return [adminId, tabId]
}

// Once per app start - a draft nobody came back to in a week is abandoned, and may hold personal data
export function purgeExpiredDrafts(now = Date.now()): void {
  for (const key of draftKeys()) readDraft(key, now) // readDraft drops what's expired or malformed
}

// A closed workspace tab never comes back, so neither can its drafts
export function purgeDraftsForTab(tabId: string): void {
  for (const key of draftKeys()) if (owners(key)[1] === tabId) clearDraft(key)
}

// Explicit logout - a shared machine shouldn't keep the last admin's half-typed customer records
export function purgeAllDrafts(): void {
  for (const key of draftKeys()) clearDraft(key)
}
