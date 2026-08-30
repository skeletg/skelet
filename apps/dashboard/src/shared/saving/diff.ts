// Standard builders for SectionDiffEntry - THE way sections present pending changes to the
// page-level save flow - One entry is one human-readable change ("Name: a -> b"), never a JSON
// dump, and entries describe REAL changes only: changed() returns null on equal normalized
// values and the registry drops no-ops centrally, so round-tripped form state can't light up Save

import type { SectionDiffEntry } from './saveRegistry'

const CLIP = 120

export function display(v: unknown): string {  // empty-ish becomes '' (the dialog renders an em dash), booleans yes/no, arrays join, objects stringify, all clipped
  if (v === null || v === undefined) return ''
  if (typeof v === 'boolean') return v ? 'yes' : 'no'
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : ''
  if (typeof v === 'string') return clip(v.trim())
  if (Array.isArray(v)) return clip(v.map((x) => display(x)).filter(Boolean).join(', '))
  if (v instanceof Date) return v.toISOString()
  try {
    return clip(JSON.stringify(v) ?? '')
  } catch {
    return String(v)
  }
}

function clip(s: string): string {
  return s.length > CLIP ? `${s.slice(0, CLIP - 1)}…` : s
}

export function changed(label: string, before: unknown, after: unknown): SectionDiffEntry | null {  // null when the displayed values match, so callers can spread-filter
  const b = display(before)
  const a = display(after)
  if (b === a) return null
  return { kind: 'change', label, before: b, after: a }
}

export function added(label: string, summary?: unknown): SectionDiffEntry {
  return { kind: 'add', label, before: '', after: display(summary) }
}

export function removed(label: string, summary?: unknown): SectionDiffEntry {
  return { kind: 'remove', label, before: display(summary), after: '' }
}

export type FieldSpec<T> = {
  label: string
  value: (obj: T) => unknown
  format?: (v: unknown) => string
}

export function fieldDiff<T>(original: T, draft: T, fields: FieldSpec<T>[]): SectionDiffEntry[] {  // display()'s normalization (trim, ''=null=undefined, yes/no) is what stops equal-but-differently-typed values producing phantom changes
  const out: SectionDiffEntry[] = []
  for (const f of fields) {
    const fmt = f.format ?? display
    const entry = changed(f.label, fmt(f.value(original)), fmt(f.value(draft)))
    if (entry) out.push(entry)
  }
  return out
}
