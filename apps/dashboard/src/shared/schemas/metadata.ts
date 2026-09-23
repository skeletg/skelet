import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'

// Free-form key/value metadata - the JSONB column half the records carry. One schema, one cleanup
// and one diff, so every form lists "Metadata · carrier: dhl → ups" the same way and none of them
// forgets to diff it at all (several used to)

export const metadataSchema = z.array(z.object({ key: z.string(), value: z.string() }))

export type MetadataEntry = z.infer<typeof metadataSchema>[number]

export function toMetadataValues(entries?: readonly MetadataEntry[] | null): MetadataEntry[] {
  return (entries ?? []).map((e) => ({ key: e.key, value: e.value }))
}

// Rows with a blank key are half-typed and dropped; keys are trimmed. Values keep their spacing
// unless the record's own service trims them too
export function cleanMetadata(entries: readonly MetadataEntry[], opts: { trimValues?: boolean } = {}): MetadataEntry[] {
  return entries
    .map((e) => ({ key: e.key.trim(), value: opts.trimValues ? e.value.trim() : e.value }))
    .filter((e) => e.key !== '')
}

// One entry per key - added, removed or changed - over the cleaned rows, so a blank row being typed
// doesn't show up as a change
export function metadataDiff(
  before: readonly MetadataEntry[],
  after: readonly MetadataEntry[],
  label = 'Metadata',
): SectionDiffEntry[] {
  const asMap = (rows: readonly MetadataEntry[]) => new Map(cleanMetadata(rows).map((e) => [e.key, e.value]))
  const was = asMap(before)
  const now = asMap(after)
  const out: SectionDiffEntry[] = []
  for (const [key, value] of now) {
    if (!was.has(key)) out.push(added(`${label} · ${key}`, value))
    else {
      const entry = changed(`${label} · ${key}`, was.get(key), value)
      if (entry) out.push(entry)
    }
  }
  for (const [key, value] of was) if (!now.has(key)) out.push(removed(`${label} · ${key}`, value))
  return out
}
