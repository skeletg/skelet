import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import type { FacetAttr } from '../facet-indexing'

// Facet indexing - THE field list (docs/conventions.md, "Record forms"): the per-facet and per-value
// index opt-ins (catalog-wide) and the indexable combinations (a config value, per scope). The
// guard fields are generic config-registry values and stage through useConfigDrafts instead

export const COMBINATIONS_PATH = 'seo/facets/indexable_combinations'
export const MAX_SLOTS = 4

export const facetIndexingSchema = z
  .object({
    facets: z
      .record(z.string(), z.boolean())
      .describe('Per-attribute SEO opt-in, keyed by attribute ID - a filter URL is indexable only when its attribute and value are both on'),
    values: z.record(z.string(), z.boolean()).describe('Per-value SEO opt-in, keyed by attribute value ID'),
    // A blank slot is one not picked yet
    combos: z
      .array(z.array(z.string()))
      .superRefine((rows, ctx) => {
        rows.forEach((row, i) => {
          if (row.some((slug) => slug === '')) {
            ctx.addIssue({ code: 'custom', path: [i], message: 'Pick a facet for every slot, or remove the row' })
          }
        })
      })
      .describe(`Multi-facet filter combinations that may be indexed, each a list of 2 to ${MAX_SLOTS} facet slugs`),
  })
  .describe('Which storefront filter URLs search engines may index')

export type FacetIndexingValues = z.infer<typeof facetIndexingSchema>

export const FACET_INDEXING_LABELS: Record<keyof FacetIndexingValues, string> = {
  facets: 'Facet opt-ins',
  values: 'Value opt-ins',
  combos: 'Indexable combinations',
}

// The stored `slug + slug` lines the storefront parses - a line with fewer than two facets means nothing
export function parseCombos(raw: string): string[][] {
  return raw
    .split('\n')
    .map((line) =>
      line
        .split('+')
        .map((s) => s.trim())
        .filter(Boolean),
    )
    .filter((combo) => combo.length >= 2)
}

export function serializeCombos(rows: string[][]): string {
  return rows
    .map((row) => row.filter(Boolean))
    .filter((row) => row.length >= 2)
    .map((row) => row.join(' + '))
    .join('\n')
}

export function toFacetIndexingValues(attrs: FacetAttr[], storedCombos: string): FacetIndexingValues {
  return {
    facets: Object.fromEntries(attrs.map((a) => [a.id, a.isIndexable])),
    values: Object.fromEntries(attrs.flatMap((a) => a.values.map((v) => [v.id, v.isIndexable]))),
    combos: parseCombos(storedCombos),
  }
}

const flipped = (before: Record<string, boolean>, after: Record<string, boolean>) =>
  Object.keys(after).filter((id) => before[id] !== undefined && before[id] !== after[id])

// The writes the edits need: facet switches one by one, values in one call per direction, and the
// combinations as one config value
export function facetIndexingChanges(before: FacetIndexingValues, after: FacetIndexingValues) {
  const values = flipped(before.values, after.values)
  return {
    facets: flipped(before.facets, after.facets).map((id) => ({ id, isIndexable: after.facets[id] })),
    valuesOn: values.filter((id) => after.values[id]),
    valuesOff: values.filter((id) => !after.values[id]),
    combos: serializeCombos(before.combos) !== serializeCombos(after.combos) ? serializeCombos(after.combos) : null,
  }
}

const state = (on: boolean) => (on ? 'indexable' : 'not indexable')

export function facetIndexingDiff(attrs: FacetAttr[]) {
  return (original: FacetIndexingValues, draft: FacetIndexingValues): SectionDiffEntry[] => {
    const changes = facetIndexingChanges(original, draft)
    const out: SectionDiffEntry[] = changes.facets.map(({ id, isIndexable }) => ({
      kind: 'change',
      label: attrs.find((a) => a.id === id)?.name ?? id,
      before: state(!isIndexable),
      after: state(isIndexable),
    }))
    // Values roll up per facet - a 40-row diff would drown the confirm dialog
    for (const a of attrs) {
      const on = a.values.filter((v) => changes.valuesOn.includes(v.id)).length
      const off = a.values.filter((v) => changes.valuesOff.includes(v.id)).length
      if (on + off === 0) continue
      out.push({
        kind: 'change',
        label: `${a.name} values`,
        before: `${on + off} changing`,
        after: [on ? `${on} indexable` : '', off ? `${off} not indexable` : ''].filter(Boolean).join(', '),
      })
    }
    if (changes.combos !== null) {
      out.push({
        kind: 'change',
        label: FACET_INDEXING_LABELS.combos,
        before: serializeCombos(original.combos).replace(/\n/g, ' · ') || 'none',
        after: changes.combos.replace(/\n/g, ' · ') || 'none',
      })
    }
    return out
  }
}
