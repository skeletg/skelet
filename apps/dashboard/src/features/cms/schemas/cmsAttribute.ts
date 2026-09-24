import { z } from 'zod'
import type { CmsInputType, CmsRelationshipTarget } from '@/graphql/schema.types'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'

// New CMS attribute - a custom field definition page types reuse. Slug and input type are fixed
// after creation; the display name is seeded for the locale being edited

export const CMS_INPUT_TYPES: { value: CmsInputType; label: string }[] = [
  { value: 'TEXT', label: 'Text (single line)' },
  { value: 'TEXTAREA', label: 'Text (multi-line)' },
  { value: 'RICHTEXT', label: 'Rich text (Lexical)' },
  { value: 'NUMBER', label: 'Number' },
  { value: 'BOOLEAN', label: 'Boolean' },
  { value: 'DATE', label: 'Date' },
  { value: 'DATETIME', label: 'Date + time' },
  { value: 'URL', label: 'URL' },
  { value: 'SELECT', label: 'Select (one option)' },
  { value: 'MULTISELECT', label: 'Multi-select' },
  { value: 'ASSET', label: 'Media asset (one)' },
  { value: 'ASSETS', label: 'Media assets (many)' },
  { value: 'RELATIONSHIP', label: 'Relationship' },
  { value: 'JSON', label: 'JSON (free-form)' },
]

export const CMS_RELATIONSHIP_TARGETS: CmsRelationshipTarget[] = ['CMS_PAGE', 'BLOG_POST', 'PRODUCT', 'COLLECTION', 'CATEGORY']

export const newCmsAttributeSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required'),
    name: z.string().trim().min(1, 'Display name is required'),
    inputType: z.string(),
    translatable: z.boolean(),
    requiredDefault: z.boolean(),
    unit: z.string(),
    // '' until picked - only a relationship needs one
    target: z.string(),
  })
  .refine((v) => v.inputType !== 'RELATIONSHIP' || v.target !== '', {
    message: 'A relationship needs something to point at',
    path: ['target'],
  })

export type NewCmsAttributeValues = z.infer<typeof newCmsAttributeSchema>

export const emptyNewCmsAttribute: NewCmsAttributeValues = {
  slug: '',
  name: '',
  inputType: 'TEXT',
  translatable: false,
  requiredDefault: false,
  unit: '',
  target: '',
}

export function toNewCmsAttributeInput(v: NewCmsAttributeValues) {
  return {
    slug: v.slug.trim(),
    inputType: v.inputType as CmsInputType,
    isTranslatable: v.translatable,
    isRequiredDefault: v.requiredDefault,
    // A unit only means something on a number
    unit: v.inputType === 'NUMBER' ? v.unit.trim() || null : null,
    relationshipTarget: v.inputType === 'RELATIONSHIP' && v.target ? (v.target as CmsRelationshipTarget) : null,
  }
}

// The attribute's own page - its default value (seeded into new pages) and the validation rules
// enforced when a page is saved. Which of them apply depends on the input type, so the schema is
// built per attribute. Numbers are text here, as typed - blank means "no rule"

type Kind = { isNum: boolean; isText: boolean; isBool: boolean }

export function attributeKind(inputType: CmsInputType): Kind {
  return {
    isNum: inputType === 'NUMBER',
    isText: inputType === 'TEXT' || inputType === 'TEXTAREA' || inputType === 'URL' || inputType === 'RICHTEXT',
    isBool: inputType === 'BOOLEAN',
  }
}

const blankOrNumber = (v: string) => v.trim() === '' || Number.isFinite(Number(v))
const blankOrCount = (v: string) => v.trim() === '' || (Number.isInteger(Number(v)) && Number(v) >= 0)

function compiles(pattern: string): boolean {
  try {
    new RegExp(pattern)
    return true
  } catch {
    return false
  }
}

export function cmsAttributeDefaultsSchema(inputType: CmsInputType) {
  const { isNum } = attributeKind(inputType)
  return z
    .object({
      defaultValue: z.string().refine((v) => !isNum || blankOrNumber(v), 'A number'),
      defaultBool: z.boolean(),
      min: z.string().refine(blankOrNumber, 'A number'),
      max: z.string().refine(blankOrNumber, 'A number'),
      minLength: z.string().refine(blankOrCount, 'A whole number, zero or more'),
      maxLength: z.string().refine(blankOrCount, 'A whole number, zero or more'),
      regex: z.string().refine((v) => v.trim() === '' || compiles(v.trim()), 'Not a valid regular expression'),
    })
    .superRefine((v, ctx) => {
      if (v.min.trim() !== '' && v.max.trim() !== '' && Number(v.min) > Number(v.max)) {
        ctx.addIssue({ code: 'custom', path: ['max'], message: 'Max must be at least min' })
      }
      if (v.minLength.trim() !== '' && v.maxLength.trim() !== '' && Number(v.minLength) > Number(v.maxLength)) {
        ctx.addIssue({ code: 'custom', path: ['maxLength'], message: 'Max length must be at least min length' })
      }
    })
}

export type CmsAttributeDefaultsValues = z.infer<ReturnType<typeof cmsAttributeDefaultsSchema>>

export const CMS_ATTRIBUTE_DEFAULTS_LABELS: Record<keyof CmsAttributeDefaultsValues, string> = {
  defaultValue: 'Default value',
  defaultBool: 'Default',
  min: 'Min',
  max: 'Max',
  minLength: 'Min length',
  maxLength: 'Max length',
  regex: 'Regex',
}

const text = (x: unknown) => (x == null ? '' : String(x))

export function toCmsAttributeDefaultsValues(a: { defaultValue?: unknown; validation?: unknown }): CmsAttributeDefaultsValues {
  const v = (a.validation ?? {}) as Record<string, unknown>
  return {
    defaultValue: text(a.defaultValue),
    defaultBool: a.defaultValue === true,
    min: text(v.min),
    max: text(v.max),
    minLength: text(v.minLength),
    maxLength: text(v.maxLength),
    regex: text(v.regex),
  }
}

// Only the rules the input type supports are sent; blank ones are left out
export function toCmsAttributeDefaultsInput(v: CmsAttributeDefaultsValues, inputType: CmsInputType, id: string) {
  const { isNum, isText, isBool } = attributeKind(inputType)
  const validation: Record<string, unknown> = {}
  if (isNum) {
    if (v.min.trim() !== '') validation.min = Number(v.min)
    if (v.max.trim() !== '') validation.max = Number(v.max)
  }
  if (isText) {
    if (v.minLength.trim() !== '') validation.minLength = Number(v.minLength)
    if (v.maxLength.trim() !== '') validation.maxLength = Number(v.maxLength)
    if (v.regex.trim() !== '') validation.regex = v.regex.trim()
  }
  let defaultValue: unknown = null
  if (isBool) defaultValue = v.defaultBool
  else if (isNum) defaultValue = v.defaultValue.trim() === '' ? null : Number(v.defaultValue)
  else if (v.defaultValue.trim() !== '') defaultValue = v.defaultValue
  return { id, validation, defaultValue }
}

export function cmsAttributeDefaultsDiff(inputType: CmsInputType) {
  const { isBool } = attributeKind(inputType)
  return (original: CmsAttributeDefaultsValues, draft: CmsAttributeDefaultsValues): SectionDiffEntry[] =>
    fieldDiff(original, draft, [
      isBool
        ? { label: CMS_ATTRIBUTE_DEFAULTS_LABELS.defaultBool, value: (o) => o.defaultBool }
        : { label: CMS_ATTRIBUTE_DEFAULTS_LABELS.defaultValue, value: (o) => o.defaultValue },
      ...(['min', 'max', 'minLength', 'maxLength', 'regex'] as const).map((k) => ({
        label: CMS_ATTRIBUTE_DEFAULTS_LABELS[k],
        value: (o: CmsAttributeDefaultsValues) => o[k],
      })),
    ])
}
