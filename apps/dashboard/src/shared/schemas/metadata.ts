import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'
import { detailFieldFor, detailFieldText } from '@/plugins/field-registry'

// Free-form key/value metadata - the JSONB column half the records carry. One schema, one cleanup
// and one diff, so every form lists "Metadata · carrier: dhl → ups" the same way and none of them
// forgets to diff it at all (several used to)

export const metadataSchema = z
  .array(
    z.object({
      key: z.string().describe('Metadata key - unique within the record'),
      value: z.string().describe('Metadata value'),
    }),
  )
  .describe('Free-form key/value metadata stored on the record')

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
// `target` names the page, so a plugin field's key reads by the field's label and its value as the field shows it
export function metadataDiff(
  before: readonly MetadataEntry[],
  after: readonly MetadataEntry[],
  label = 'Metadata',
  target?: string,
): SectionDiffEntry[] {
  const asMap = (rows: readonly MetadataEntry[]) => new Map(cleanMetadata(rows).map((e) => [e.key, e.value]))
  const was = asMap(before)
  const now = asMap(after)
  const name = (key: string) => detailFieldFor(target, key)?.label ?? `${label} · ${key}`
  const text = (key: string, v: string) => {
    const f = detailFieldFor(target, key)
    return f ? detailFieldText(f, v) : v
  }
  const out: SectionDiffEntry[] = []
  for (const [key, value] of now) {
    if (!was.has(key)) out.push(added(name(key), text(key, value)))
    else {
      const entry = changed(name(key), text(key, was.get(key) ?? ''), text(key, value))
      if (entry) out.push(entry)
    }
  }
  for (const [key, value] of was) if (!now.has(key)) out.push(removed(name(key), text(key, value)))
  return out
}

// Value without a key, or a key used twice - the backend keys metadata by name. `path` is the list's
// field, `label` names it in the message
export function metadataIssues(rows: readonly MetadataEntry[], path: string, ctx: z.RefinementCtx, label = 'Metadata') {
  if (rows.some((r) => !r.key.trim() && r.value.trim())) {
    ctx.addIssue({ code: 'custom', path: [path], message: `${label} entries with a value need a key` })
    return
  }
  const keys = cleanMetadata(rows).map((r) => r.key)
  const dupe = keys.find((k, i) => keys.indexOf(k) !== i)
  if (dupe) ctx.addIssue({ code: 'custom', path: [path], message: `Duplicate ${label.toLowerCase()} key: "${dupe}"` })
}
