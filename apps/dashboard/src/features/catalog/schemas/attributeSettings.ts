import { z } from 'zod'
import type { AttributeInputType } from '@/graphql/schema.types'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'

// The attribute's own page - name, unit, behaviour flags and the default value. The indexable master
// switch isn't here: flipping it cascades to every value and product override, so it's an action
// with its own confirm. Descriptions, search tuning and value rows are their own staged sections

// The default as the editor holds it, per input type:
//   PLAIN_TEXT / DATE / DATE_TIME / NUMERIC: the typed text
//   BOOLEAN:                                 'true' | 'false'
//   DROPDOWN / SWATCH:                       a value id
//   MULTISELECT:                             newline-separated value ids
const SELECTABLE: AttributeInputType[] = ['DROPDOWN', 'SWATCH', 'MULTISELECT']

export function serializeDefault(inputType: AttributeInputType, value: unknown): string {
  if (value == null) return inputType === 'BOOLEAN' ? 'false' : ''
  switch (inputType) {
    case 'BOOLEAN':
      return value === true ? 'true' : 'false'
    case 'NUMERIC':
      return typeof value === 'number' ? String(value) : ''
    case 'MULTISELECT':
      return Array.isArray(value) ? value.filter((v) => typeof v === 'string').join('\n') : ''
    default:
      return typeof value === 'string' ? value : ''
  }
}

type Decoded = { ok: true; value: unknown } | { ok: false; message: string }

export function decodeDefault(inputType: AttributeInputType, draft: string): Decoded {
  switch (inputType) {
    case 'BOOLEAN':
      return { ok: true, value: draft === 'true' }
    case 'NUMERIC': {
      const n = Number(draft)
      return draft.trim() !== '' && Number.isFinite(n) ? { ok: true, value: n } : { ok: false, message: 'Default value must be a number' }
    }
    case 'MULTISELECT': {
      const ids = draft.split('\n').map((s) => s.trim()).filter(Boolean)
      return ids.length > 0 ? { ok: true, value: ids } : { ok: false, message: 'Pick at least one default value' }
    }
    case 'PLAIN_TEXT':
    case 'DATE':
    case 'DATE_TIME':
    case 'DROPDOWN':
    case 'SWATCH':
      return draft.trim() ? { ok: true, value: draft.trim() } : { ok: false, message: 'Pick or enter a default value' }
    default:
      return { ok: false, message: 'This input type takes no default' }
  }
}

export function attributeSettingsSchema(inputType: AttributeInputType) {
  return z
    .object({
      name: z.string().trim().min(1, 'Name is required'),
      unit: z.string(),
      sortOrder: z.number().int('Whole numbers only'),
      valueRequired: z.boolean(),
      filterable: z.boolean(),
      searchable: z.boolean(),
      isPublic: z.boolean(),
      facetCollapsed: z.boolean(),
      h1Position: z.enum(['BEFORE', 'AFTER']),
      synapseTrack: z.boolean(),
      autoSegment: z.boolean(),
      hasDefault: z.boolean(),
      defaultDraft: z.string(),
    })
    .superRefine((v, ctx) => {
      // Required => default: new products auto-fill from the default, which is what makes "required" hold
      if (v.valueRequired && !v.hasDefault) {
        ctx.addIssue({ code: 'custom', path: ['hasDefault'], message: 'Required attributes must have a default value' })
      }
      if (!v.hasDefault) return
      const decoded = decodeDefault(inputType, v.defaultDraft)
      if (!decoded.ok) ctx.addIssue({ code: 'custom', path: ['defaultDraft'], message: decoded.message })
    })
}

export type AttributeSettingsValues = z.infer<ReturnType<typeof attributeSettingsSchema>>

export const ATTRIBUTE_SETTINGS_LABELS: Record<keyof AttributeSettingsValues, string> = {
  name: 'Name',
  unit: 'Unit',
  sortOrder: 'Sort order',
  valueRequired: 'Required',
  filterable: 'Filterable',
  searchable: 'Searchable',
  isPublic: 'Public',
  facetCollapsed: 'Facet collapsed',
  h1Position: 'H1 position',
  synapseTrack: 'Track in Synapse',
  autoSegment: 'Auto-create segments',
  hasDefault: 'Default value',
  defaultDraft: 'Default value',
}

type ExistingAttribute = {
  name: string
  unit?: string | null
  sortOrder: number
  valueRequired: boolean
  filterable: boolean
  searchable: boolean
  isPublic: boolean
  facetCollapsed: boolean
  h1Position: 'BEFORE' | 'AFTER'
  synapseTrack: boolean
  autoSegment: boolean
  inputType: AttributeInputType
  defaultValue?: unknown
}

export function toAttributeSettingsValues(a: ExistingAttribute): AttributeSettingsValues {
  return {
    name: a.name,
    unit: a.unit ?? '',
    sortOrder: a.sortOrder,
    valueRequired: a.valueRequired,
    filterable: a.filterable,
    searchable: a.searchable,
    isPublic: a.isPublic,
    facetCollapsed: a.facetCollapsed,
    h1Position: a.h1Position,
    synapseTrack: a.synapseTrack,
    autoSegment: a.autoSegment,
    hasDefault: a.defaultValue != null,
    defaultDraft: serializeDefault(a.inputType, a.defaultValue),
  }
}

// What the default ends up as - '' when there is none, so an unticked default and an emptied one
// read the same
const effectiveDefault = (v: AttributeSettingsValues) => (v.hasDefault ? v.defaultDraft : '')

// Auto-segment only takes effect while tracking is on - compare effective against effective, or a
// stored flag with tracking off reads as a phantom edit
const effectiveAutoSegment = (v: AttributeSettingsValues) => v.synapseTrack && v.autoSegment

export function toAttributeSettingsInput(before: AttributeSettingsValues, v: AttributeSettingsValues, inputType: AttributeInputType, id: string) {
  const defaultChanged = before.hasDefault !== v.hasDefault || effectiveDefault(before) !== effectiveDefault(v)
  const decoded = v.hasDefault ? decodeDefault(inputType, v.defaultDraft) : null
  return {
    id,
    name: v.name.trim(),
    unit: v.unit,
    sortOrder: v.sortOrder,
    valueRequired: v.valueRequired,
    filterable: v.filterable,
    searchable: v.searchable,
    isPublic: v.isPublic,
    facetCollapsed: v.facetCollapsed,
    h1Position: v.h1Position,
    synapseTrack: v.synapseTrack,
    autoSegment: effectiveAutoSegment(v),
    // Left out when unchanged; the schema has already refused an undecodable one
    ...(defaultChanged ? { defaultValue: decoded?.ok ? { value: decoded.value } : { clear: true } } : {}),
  }
}

// Selectable defaults hold value ids - the dialog shows the value NAMES
export function attributeSettingsDiff(inputType: AttributeInputType, values: { id: string; name: string; value: string }[]) {
  const human = (draft: string) =>
    SELECTABLE.includes(inputType)
      ? draft
          .split('\n')
          .filter(Boolean)
          .map((id) => {
            const v = values.find((x) => x.id === id)
            return v ? v.name || v.value : id
          })
          .join(', ')
      : draft
  return (original: AttributeSettingsValues, draft: AttributeSettingsValues): SectionDiffEntry[] =>
    fieldDiff(original, draft, [
      { label: ATTRIBUTE_SETTINGS_LABELS.name, value: (o) => o.name },
      { label: ATTRIBUTE_SETTINGS_LABELS.unit, value: (o) => o.unit },
      { label: ATTRIBUTE_SETTINGS_LABELS.sortOrder, value: (o) => o.sortOrder },
      { label: ATTRIBUTE_SETTINGS_LABELS.valueRequired, value: (o) => o.valueRequired },
      { label: ATTRIBUTE_SETTINGS_LABELS.filterable, value: (o) => o.filterable },
      { label: ATTRIBUTE_SETTINGS_LABELS.searchable, value: (o) => o.searchable },
      { label: ATTRIBUTE_SETTINGS_LABELS.isPublic, value: (o) => o.isPublic },
      { label: ATTRIBUTE_SETTINGS_LABELS.facetCollapsed, value: (o) => o.facetCollapsed },
      { label: ATTRIBUTE_SETTINGS_LABELS.h1Position, value: (o) => o.h1Position },
      { label: ATTRIBUTE_SETTINGS_LABELS.synapseTrack, value: (o) => o.synapseTrack },
      { label: ATTRIBUTE_SETTINGS_LABELS.autoSegment, value: effectiveAutoSegment },
      { label: ATTRIBUTE_SETTINGS_LABELS.defaultDraft, value: (o) => human(effectiveDefault(o)) },
    ])
}
