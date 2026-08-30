// Client-side diffing for the page-editor's revision compare view - Pure
// functions (no React, no network): the History panel fetches two revision
// snapshots and renders the diff these produce - field-level word diffs for
// title/meta and a block-level diff of the body Document (added / removed /
// reordered / modified, with per-prop word diffs inside changed blocks)
import { CMS_BLOCK_TYPES, type CmsBlock, type CmsDocument } from './page-blocks'

export type DiffSpan = { kind: 'eq' | 'add' | 'del'; text: string }

export type FieldChange = { key: string; label: string; spans: DiffSpan[] }

export type PropChange = { key: string; label: string; spans: DiffSpan[] }

export type BlockChange =
  | { kind: 'added'; id: string; label: string }
  | { kind: 'removed'; id: string; label: string }
  | { kind: 'reordered'; id: string; label: string; from: number; to: number }
  | {
      kind: 'modified'
      id: string
      label: string
      props: PropChange[]
      childCount?: { from: number; to: number }
    }

export type DocDiff = { changes: BlockChange[]; freeform: boolean }

// What diffFields/diffDocuments accept - the subset of a revision they read
export type RevisionLike = {
  title?: string | null
  metaTitle?: string | null
  metaDescription?: string | null
  metaKeywords?: string | null
  body?: unknown
}

const BLOCK_LABEL = new Map(CMS_BLOCK_TYPES.map((d) => [d.type, d.label]))
const PROP_LABELS = new Map(
  CMS_BLOCK_TYPES.map((d) => [d.type, new Map(d.fields.map((f) => [f.key, f.label]))]),
)

function blockLabel(type: string): string {
  return BLOCK_LABEL.get(type) ?? type
}

function propLabel(type: string, key: string): string {
  return PROP_LABELS.get(type)?.get(key) ?? key
}

function tokenize(s: string): string[] {
  return s.length ? s.split(/(\s+)/).filter((t) => t.length > 0) : []
}

// diffWords is a word-level LCS diff producing coalesced eq/add/del spans - For pathologically long values it falls
// back to a whole-value replace so the O(n·m) table can't blow up
export function diffWords(a: string, b: string): DiffSpan[] {
  const at = tokenize(a)
  const bt = tokenize(b)
  const n = at.length
  const m = bt.length
  const spans: DiffSpan[] = []
  const push = (kind: DiffSpan['kind'], text: string) => {
    const last = spans[spans.length - 1]
    if (last && last.kind === kind) last.text += text
    else spans.push({ kind, text })
  }
  if (n === 0 && m === 0) return []
  if (n * m > 20000) {
    if (a) push('del', a)
    if (b) push('add', b)
    return spans
  }
  // LCS length table
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = at[i] === bt[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (at[i] === bt[j]) {
      push('eq', at[i])
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      push('del', at[i])
      i++
    } else {
      push('add', bt[j])
      j++
    }
  }
  while (i < n) push('del', at[i++])
  while (j < m) push('add', bt[j++])
  return spans
}

const FIELDS: { key: keyof RevisionLike; label: string }[] = [
  { key: 'title', label: 'Title' },
  { key: 'metaTitle', label: 'Meta title' },
  { key: 'metaDescription', label: 'Meta description' },
  { key: 'metaKeywords', label: 'Meta keywords' },
]

// diffFields compares the scalar text fields and returns only those that changed
export function diffFields(prev: RevisionLike, next: RevisionLike): FieldChange[] {
  const out: FieldChange[] = []
  for (const f of FIELDS) {
    const a = (prev[f.key] ?? '') as string
    const b = (next[f.key] ?? '') as string
    if (a !== b) out.push({ key: String(f.key), label: f.label, spans: diffWords(a, b) })
  }
  return out
}

function asDocument(body: unknown): CmsDocument | null {
  if (body && typeof body === 'object' && Array.isArray((body as { blocks?: unknown }).blocks)) {
    return body as CmsDocument
  }
  return null
}

function stringifyProp(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
}

function diffProps(
  type: string,
  a: Record<string, unknown> = {},
  b: Record<string, unknown> = {},
): PropChange[] {
  const keys = new Set([...Object.keys(a ?? {}), ...Object.keys(b ?? {})])
  const out: PropChange[] = []
  for (const k of keys) {
    if (deepEqual(a?.[k], b?.[k])) continue
    out.push({
      key: k,
      label: propLabel(type, k),
      spans: diffWords(stringifyProp(a?.[k]), stringifyProp(b?.[k])),
    })
  }
  return out.sort((x, y) => x.label.localeCompare(y.label))
}

// diffDocuments compares two block-document bodies, matching top-level blocks by
// their stable id - Non-block freeform bodies are compared coarsely
export function diffDocuments(prevBody: unknown, nextBody: unknown): DocDiff {
  const a = asDocument(prevBody)
  const b = asDocument(nextBody)
  if (!a || !b) {
    const same = deepEqual(prevBody, nextBody)
    return {
      freeform: true,
      changes: same ? [] : [{ kind: 'modified', id: '', label: 'Content', props: [] }],
    }
  }
  const aIdx = new Map(a.blocks.map((bl, i) => [bl.id, { bl, i }]))
  const bIdx = new Map(b.blocks.map((bl, i) => [bl.id, { bl, i }]))
  const changes: BlockChange[] = []

  for (const [id, { bl }] of aIdx) {
    if (!bIdx.has(id)) changes.push({ kind: 'removed', id, label: blockLabel(bl.type) })
  }
  for (const [id, { bl, i }] of bIdx) {
    const prev = aIdx.get(id)
    if (!prev) {
      changes.push({ kind: 'added', id, label: blockLabel(bl.type) })
      continue
    }
    const props = diffProps(bl.type, prev.bl.props, bl.props)
    const childA = childCount(prev.bl)
    const childB = childCount(bl)
    const childChanged = childA !== childB ? { from: childA, to: childB } : undefined
    if (props.length > 0 || childChanged) {
      changes.push({ kind: 'modified', id, label: blockLabel(bl.type), props, childCount: childChanged })
    } else if (prev.i !== i) {
      changes.push({ kind: 'reordered', id, label: blockLabel(bl.type), from: prev.i, to: i })
    }
  }
  return { changes, freeform: false }
}

function childCount(b: CmsBlock): number {
  return b.blocks?.length ?? 0
}
