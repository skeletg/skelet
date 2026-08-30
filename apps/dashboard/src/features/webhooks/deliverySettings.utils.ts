// Pure helpers for the webhook delivery-settings UI, split out so the component file stays fast-refresh-clean

export type HeaderRow = { key: string; value: string }

export function objectToHeaderRows(obj: unknown): HeaderRow[] {
  if (!obj || typeof obj !== 'object') return []
  return Object.entries(obj as Record<string, unknown>)
    .filter(([, v]) => typeof v === 'string')
    .map(([key, value]) => ({ key, value: value as string }))
}

export function headerRowsToObject(rows: HeaderRow[]): Record<string, string> | undefined {
  const entries = rows.filter((r) => r.key.trim() !== '')
  if (entries.length === 0) return undefined
  return Object.fromEntries(entries.map((r) => [r.key.trim(), r.value]))
}

export function isWildcardEntry(entry: string): boolean {
  return entry === '*' || entry.endsWith('.*')
}
