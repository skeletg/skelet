import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, fieldDiff, removed } from '@/shared/saving/diff'
import { CUSTOM_FIELD_KINDS, parseCustomFields, type CmsBlockField } from '../page-blocks'

// Custom CMS block type form - THE field list (docs/conventions.md, "Record forms")

export type ExistingBlockType = {
  id: string
  key: string
  name: string
  icon: string
  fields: unknown
  htmlTemplate: string
}

// One editable field row - options stay comma-separated text until they're sent
const fieldRowSchema = z
  .object({
    key: z.string().describe('Field key, filled into the template as {{key}} - rows with an empty key are dropped'),
    label: z.string().describe('Label shown to editors'),
    kind: z.string().describe(`Input kind, one of ${CUSTOM_FIELD_KINDS.join(', ')}`),
    required: z.boolean().describe('Editors must fill the field in'),
    placeholder: z.string().describe('Placeholder text in the empty input - empty string for none'),
    description: z.string().describe('Help text shown under the input - empty string for none'),
    options: z.string().describe('Comma-separated choices for a select field - ignored for other kinds'),
  })
  .describe('One editable field of a custom block')

export type FieldRow = z.infer<typeof fieldRowSchema>

export const blockTypeSchema = z
  .object({
    key: z
      .string()
      .trim()
      .min(1, 'Key is required')
      .describe('Stable identifier placed blocks reference - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name in the block picker'),
    icon: z.string().describe('Lucide icon name, such as sparkles - empty string for none'),
    template: z.string().describe('HTML template the block renders, with {{key}} placeholders for its fields'),
    fields: z
      .array(fieldRowSchema)
      .superRefine((rows, ctx) => {
        // Two fields on one key would fight over the same {{key}} placeholder
        const seen = new Set<string>()
        rows.forEach((row, i) => {
          const key = row.key.trim()
          if (!key) return
          if (seen.has(key)) ctx.addIssue({ code: 'custom', message: `Field key "${key}" is used twice`, path: [i, 'key'] })
          seen.add(key)
        })
      })
      .describe('The fields editors fill in, each with a unique key'),
  })
  .describe('A custom CMS block type: an HTML template plus the fields that fill it')

export type BlockTypeValues = z.infer<typeof blockTypeSchema>

export const BLOCK_TYPE_LABELS: Record<keyof BlockTypeValues, string> = {
  key: 'Key',
  name: 'Name',
  icon: 'Icon',
  template: 'HTML template',
  fields: 'Fields',
}

export const emptyFieldRow = (): FieldRow => ({
  key: '',
  label: '',
  kind: 'text',
  required: false,
  placeholder: '',
  description: '',
  options: '',
})

function toRows(fields: CmsBlockField[]): FieldRow[] {
  return fields.map((f) => ({
    key: f.key,
    label: f.label,
    kind: f.kind,
    required: !!f.required,
    placeholder: f.placeholder ?? '',
    description: f.description ?? '',
    options: (f.options ?? []).map((o) => o.value).join(', '),
  }))
}

export function toBlockTypeValues(blockType?: ExistingBlockType): BlockTypeValues {
  return {
    key: blockType?.key ?? '',
    name: blockType?.name ?? '',
    icon: blockType?.icon ?? '',
    template: blockType?.htmlTemplate ?? '',
    fields: toRows(parseCustomFields(blockType?.fields)),
  }
}

// Rows without a key are half-typed and dropped; an unknown kind falls back to text
export function toBlockFields(rows: FieldRow[]): CmsBlockField[] {
  return rows
    .filter((d) => d.key.trim())
    .map((d) => ({
      key: d.key.trim(),
      label: d.label.trim() || d.key.trim(),
      kind: (CUSTOM_FIELD_KINDS as readonly string[]).includes(d.kind) ? (d.kind as CmsBlockField['kind']) : 'text',
      ...(d.required ? { required: true } : {}),
      ...(d.placeholder.trim() ? { placeholder: d.placeholder.trim() } : {}),
      ...(d.description.trim() ? { description: d.description.trim() } : {}),
      ...(d.kind === 'select' && d.options.trim()
        ? { options: d.options.split(',').map((s) => s.trim()).filter(Boolean).map((v) => ({ value: v, label: v })) }
        : {}),
    }))
}

export function toBlockTypeInput(v: BlockTypeValues) {
  return { name: v.name.trim(), icon: v.icon, fields: toBlockFields(v.fields), htmlTemplate: v.template }
}

// "Heading (text, required)" - one field as the dialog shows it
function fieldSummary(f: CmsBlockField): string {
  const extras = [f.kind, f.required ? 'required' : null, f.options?.length ? `options: ${f.options.map((o) => o.value).join(', ')}` : null]
  return `${f.label} (${extras.filter(Boolean).join(', ')})`
}

// One entry per field key - the old key-list comparison never noticed a relabel, a kind change or
// a required flag
function fieldsDiff(before: FieldRow[], after: FieldRow[]): SectionDiffEntry[] {
  const was = new Map(toBlockFields(before).map((f) => [f.key, f]))
  const now = new Map(toBlockFields(after).map((f) => [f.key, f]))
  const out: SectionDiffEntry[] = []
  for (const [key, f] of now) {
    const old = was.get(key)
    if (!old) out.push(added(`Field · ${key}`, fieldSummary(f)))
    else {
      const entry = changed(`Field · ${key}`, fieldSummary(old), fieldSummary(f))
      if (entry) out.push(entry)
    }
  }
  for (const [key, f] of was) if (!now.has(key)) out.push(removed(`Field · ${key}`, fieldSummary(f)))
  return out
}

// Key is immutable after create, so it never appears
export function blockTypeDiff(original: BlockTypeValues, draft: BlockTypeValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(original, draft, [
      { label: BLOCK_TYPE_LABELS.name, value: (o) => o.name },
      { label: BLOCK_TYPE_LABELS.icon, value: (o) => o.icon },
      { label: BLOCK_TYPE_LABELS.template, value: (o) => o.template },
    ]),
    ...fieldsDiff(original.fields, draft.fields),
  ]
}
