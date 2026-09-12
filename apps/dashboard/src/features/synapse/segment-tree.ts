// Pure model plus helpers for the segment definition tree - no JSX, which keeps the builder
// component fast-refresh clean - Mirrors the backend SegmentNode
import type { SegmentNodeInput, SegmentNodeKind, SegmentBoolOp } from '@/graphql/schema.types'
import { eventLabel } from './eventLabel'

export type TraitLeaf = {
  lifecycle: string[]
  minRecencyScore: number
  minFrequencyScore: number
  minMonetaryScore: number
  minEngagement: number
}

// The five trait fields all persist as a TRAIT leaf with one constraint set, but each stays its
// own clearly-labelled choice rather than a cryptic combined "trait" box
export type FieldKind = 'event' | 'lifecycle' | 'recency' | 'frequency' | 'monetary' | 'engagement' | 'affinity'

export const FIELD_LABELS: { value: FieldKind; label: string }[] = [
  { value: 'event', label: 'Did / didn’t (event)' },
  { value: 'lifecycle', label: 'Lifecycle stage' },
  { value: 'recency', label: 'Recency score' },
  { value: 'frequency', label: 'Frequency score' },
  { value: 'monetary', label: 'Monetary score' },
  { value: 'engagement', label: 'Engagement score' },
  { value: 'affinity', label: 'Attribute affinity' },
]

// A GROUP combines children with `op`, a leaf is one condition - `field` is a UI-only
// discriminator, dropped when serialising to the backend
export type SegNode = {
  kind: SegmentNodeKind
  field?: FieldKind
  op?: SegmentBoolOp
  children?: SegNode[]
  event?: string
  negate?: boolean
  count?: number
  days?: number
  trait?: TraitLeaf
  affinity?: { attr: string; value: string; days: number }
}

export const LIFECYCLES = ['prospect', 'new', 'active', 'at_risk', 'lapsed']
export const emptyTrait = (): TraitLeaf => ({ lifecycle: [], minRecencyScore: 0, minFrequencyScore: 0, minMonetaryScore: 0, minEngagement: 0 })

export const newEvent = (event = 'order_placed'): SegNode => ({ kind: 'EVENT', field: 'event', event, negate: false, count: 1, days: 30 })
export const newGroup = (op: SegmentBoolOp = 'AND'): SegNode => ({ kind: 'GROUP', op, children: [newEvent()] })

// Defaults make a new leaf immediately valid - scores 3, engagement 50, while lifecycle and
// affinity start empty and prompt the user
export function newCondition(field: FieldKind, events: string[]): SegNode {
  switch (field) {
    case 'event':
      return newEvent(events[0])
    case 'lifecycle':
      return { kind: 'TRAIT', field, trait: emptyTrait() }
    case 'recency':
      return { kind: 'TRAIT', field, trait: { ...emptyTrait(), minRecencyScore: 3 } }
    case 'frequency':
      return { kind: 'TRAIT', field, trait: { ...emptyTrait(), minFrequencyScore: 3 } }
    case 'monetary':
      return { kind: 'TRAIT', field, trait: { ...emptyTrait(), minMonetaryScore: 3 } }
    case 'engagement':
      return { kind: 'TRAIT', field, trait: { ...emptyTrait(), minEngagement: 50 } }
    case 'affinity':
      return { kind: 'AFFINITY', field, affinity: { attr: '', value: '', days: 90 } }
  }
}

// conditionField recovers a leaf's field type (from the UI hint, else inferred)
export function conditionField(n: SegNode): FieldKind {
  if (n.field) return n.field
  if (n.kind === 'EVENT') return 'event'
  if (n.kind === 'AFFINITY') return 'affinity'
  const t = n.trait
  if (t?.minRecencyScore) return 'recency'
  if (t?.minFrequencyScore) return 'frequency'
  if (t?.minMonetaryScore) return 'monetary'
  if (t?.minEngagement) return 'engagement'
  return 'lifecycle'
}

export function traitActive(t?: TraitLeaf): boolean {
  return !!t && (t.lifecycle.length > 0 || t.minRecencyScore > 0 || t.minFrequencyScore > 0 || t.minMonetaryScore > 0 || t.minEngagement > 0)
}

// Safe to preview or save - every group has at least one child and every leaf its required fields
export function nodeComplete(n: SegNode): boolean {
  switch (n.kind) {
    case 'GROUP':
      return (n.children?.length ?? 0) > 0 && (n.children ?? []).every(nodeComplete)
    case 'EVENT':
      return !!n.event
    case 'TRAIT':
      return traitActive(n.trait)
    case 'AFFINITY':
      return !!n.affinity?.attr.trim() && !!n.affinity?.value.trim()
    default:
      return false
  }
}

// Keeps only the fields relevant to each kind
export function nodeToInput(n: SegNode): SegmentNodeInput {
  switch (n.kind) {
    case 'GROUP':
      return { kind: 'GROUP', op: n.op ?? 'AND', children: (n.children ?? []).map(nodeToInput) }
    case 'EVENT':
      return { kind: 'EVENT', event: n.event ?? '', negate: !!n.negate, count: n.count ?? 1, days: n.days ?? 30 }
    case 'TRAIT':
      return { kind: 'TRAIT', trait: { ...(n.trait ?? emptyTrait()) } }
    case 'AFFINITY':
      return {
        kind: 'AFFINITY',
        affinity: { attr: (n.affinity?.attr ?? '').trim(), value: (n.affinity?.value ?? '').trim(), days: n.affinity?.days ?? 90 },
      }
    default:
      return { kind: n.kind }
  }
}

// Converts a stored definition back into an editable tree, including the UI-only `field`
// discriminator so each trait leaf reopens as its specific condition - empty yields a fresh group
export function definitionToTree(n?: SummaryNode | null): SegNode {
  if (!n) return newGroup('AND')
  switch (n.kind) {
    case 'GROUP':
      return {
        kind: 'GROUP',
        op: n.op === 'OR' ? 'OR' : 'AND',
        children: (n.children ?? []).filter(Boolean).map((c) => definitionToTree(c)),
      }
    case 'EVENT':
      return { kind: 'EVENT', field: 'event', event: n.event ?? '', negate: !!n.negate, count: n.count ?? 1, days: n.days ?? 30 }
    case 'AFFINITY':
      return { kind: 'AFFINITY', field: 'affinity', affinity: { attr: n.affinity?.attr ?? '', value: n.affinity?.value ?? '', days: n.affinity?.days ?? 90 } }
    case 'TRAIT': {
      const t = n.trait
      const trait: TraitLeaf = {
        lifecycle: t?.lifecycle ?? [],
        minRecencyScore: t?.minRecencyScore ?? 0,
        minFrequencyScore: t?.minFrequencyScore ?? 0,
        minMonetaryScore: t?.minMonetaryScore ?? 0,
        minEngagement: t?.minEngagement ?? 0,
      }
      const field: FieldKind = trait.minRecencyScore
        ? 'recency'
        : trait.minFrequencyScore
          ? 'frequency'
          : trait.minMonetaryScore
            ? 'monetary'
            : trait.minEngagement
              ? 'engagement'
              : 'lifecycle'
      return { kind: 'TRAIT', field, trait }
    }
    default:
      return newGroup('AND')
  }
}

// Read-only structural view as returned by the SynapseSegments query - the generated per-level types differ, so summarizeNode takes this loose shape
export type SummaryNode = {
  kind: string
  op?: string | null
  children?: readonly (SummaryNode | null)[] | null
  event?: string | null
  negate?: boolean | null
  count?: number | null
  days?: number | null
  trait?: { lifecycle: string[]; minRecencyScore: number; minFrequencyScore: number; minMonetaryScore: number; minEngagement: number } | null
  affinity?: { attr: string; value: string; days: number } | null
}

// summarizeNode renders a definition node as a readable string for the list
export function summarizeNode(n?: SummaryNode | null): string {
  if (!n) return '-'
  switch (n.kind) {
    case 'GROUP': {
      const parts = (n.children ?? []).map((c) => summarizeNode(c ?? undefined)).filter((p) => p && p !== '-')
      if (parts.length === 0) return '-'
      if (parts.length === 1) return parts[0]
      return '(' + parts.join(n.op === 'OR' ? ' OR ' : ' AND ') + ')'
    }
    case 'EVENT':
      return `${n.negate ? 'did not' : 'did'} ${eventLabel(n.event ?? '')}${n.negate ? '' : ` ≥${n.count ?? 1}×`} in ${n.days ?? 30}d`
    case 'TRAIT': {
      const t = n.trait
      if (!t) return 'traits'
      const bits: string[] = []
      if (t.lifecycle.length) bits.push(`lifecycle ${t.lifecycle.join('/')}`)
      if (t.minRecencyScore) bits.push(`recency ≥${t.minRecencyScore}`)
      if (t.minFrequencyScore) bits.push(`frequency ≥${t.minFrequencyScore}`)
      if (t.minMonetaryScore) bits.push(`monetary ≥${t.minMonetaryScore}`)
      if (t.minEngagement) bits.push(`engagement ≥${t.minEngagement}`)
      return bits.length ? bits.join(' & ') : 'traits'
    }
    case 'AFFINITY':
      return `affinity ${n.affinity?.attr}=${n.affinity?.value}`
    default:
      return '-'
  }
}
