import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import { summarizeRules, type Rules } from '../rules'

// Customer segment page - THE field list (docs/conventions.md, "Record forms"). Identity plus the
// membership rules; null rules mean manual membership

export type ExistingSegment = {
  slug: string
  name: string
  description: string
  isActive: boolean
  rules: unknown
}

export const segmentSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  slug: z.string().trim().min(1, 'Slug is required'),
  description: z.string(),
  isActive: z.boolean(),
  // Opaque to the form - the rules builder owns its shape (../rules)
  rules: z.unknown(),
})

export type SegmentValues = z.infer<typeof segmentSchema>

export const SEGMENT_LABELS: Record<keyof SegmentValues, string> = {
  name: 'Name',
  slug: 'Slug',
  description: 'Description',
  isActive: 'Active',
  rules: 'Rules',
}

export function parseRules(raw: unknown): Rules | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Rules
  return { logic: r.logic === 'or' ? 'or' : 'and', conditions: r.conditions ?? [] }
}

export function toSegmentValues(segment: ExistingSegment): SegmentValues {
  return {
    name: segment.name,
    slug: segment.slug,
    description: segment.description,
    isActive: segment.isActive,
    rules: parseRules(segment.rules),
  }
}

export const segmentRules = (v: SegmentValues) => v.rules as Rules | null

export function generalChanged(before: SegmentValues, after: SegmentValues): boolean {
  return (
    before.name !== after.name ||
    before.slug !== after.slug ||
    before.description !== after.description ||
    before.isActive !== after.isActive
  )
}

export function rulesChanged(before: SegmentValues, after: SegmentValues): boolean {
  return JSON.stringify(before.rules) !== JSON.stringify(after.rules)
}

export function toSegmentInput(v: SegmentValues, id: string) {
  return { id, name: v.name.trim(), slug: v.slug.trim(), description: v.description, isActive: v.isActive }
}

// Rules with no conditions left turn the segment back to manual
export function toSegmentRules(v: SegmentValues): Rules | null {
  const rules = segmentRules(v)
  return rules && rules.conditions.length > 0 ? rules : null
}

export function segmentDiff(original: SegmentValues, draft: SegmentValues): SectionDiffEntry[] {
  const rules = rulesChanged(original, draft)
    ? changed(SEGMENT_LABELS.rules, summarizeRules(segmentRules(original)), summarizeRules(segmentRules(draft)))
    : null
  return [
    ...fieldDiff(original, draft, [
      { label: SEGMENT_LABELS.name, value: (v) => v.name },
      { label: SEGMENT_LABELS.slug, value: (v) => v.slug },
      { label: SEGMENT_LABELS.description, value: (v) => v.description },
      { label: SEGMENT_LABELS.isActive, value: (v) => v.isActive },
    ]),
    ...(rules ? [rules] : []),
  ]
}
