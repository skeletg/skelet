import { z } from 'zod'
import type { AttributeInputType, AttributeReferenceTarget } from '@/graphql/schema.types'

// Catalog attribute dialog. Slug and input type are fixed after create (a new input type would
// orphan existing values). A default value is only settable here for scalar types - selectable
// defaults need their value rows, which don't exist until the attribute does

export type ExistingAttribute = {
  id: string
  slug: string
  name: string
  inputType: AttributeInputType
  relationshipTarget?: AttributeReferenceTarget | null
  unit: string
  valueRequired: boolean
  filterable: boolean
  searchable: boolean
  isPublic: boolean
  facetCollapsed: boolean
  isIndexable: boolean
  h1Position: 'BEFORE' | 'AFTER'
  sortOrder: number
}

export const INPUT_TYPE_OPTIONS: AttributeInputType[] = [
  'PLAIN_TEXT',
  'NUMERIC',
  'BOOLEAN',
  'DATE',
  'DATE_TIME',
  'DROPDOWN',
  'MULTISELECT',
  'SWATCH',
  'REFERENCE',
]

// Entity types a REFERENCE attribute can point at - mirrors the backend enum
export const REFERENCE_TARGET_OPTIONS: AttributeReferenceTarget[] = ['PRODUCT', 'CATEGORY', 'COLLECTION']

const SCALAR_INPUT_TYPES: AttributeInputType[] = ['PLAIN_TEXT', 'NUMERIC', 'BOOLEAN', 'DATE', 'DATE_TIME']

export function isScalarInputType(t: AttributeInputType): boolean {
  return SCALAR_INPUT_TYPES.includes(t)
}

const baseSchema = z.object({
  slug: z.string().trim().min(1, 'Slug is required'),
  name: z.string().trim().min(1, 'Name is required'),
  inputType: z.string(),
  relationshipTarget: z.string(),
  unit: z.string(),
  valueRequired: z.boolean(),
  filterable: z.boolean(),
  searchable: z.boolean(),
  isPublic: z.boolean(),
  facetCollapsed: z.boolean(),
  isIndexable: z.boolean(),
  h1Position: z.enum(['BEFORE', 'AFTER']),
  sortOrder: z.number().int('Whole numbers only'),
  // Create-only default - the toggle, then a raw text value or a boolean depending on the type
  hasDefault: z.boolean(),
  defaultRaw: z.string(),
  defaultBool: z.boolean(),
})

export type AttributeValues = z.infer<typeof baseSchema>

export const editAttributeSchema = baseSchema

// The required => default invariant, checked where it can be: new products auto-fill from the
// default, which is what makes "required" actually hold
export const createAttributeSchema = baseSchema.superRefine((v, ctx) => {
  const type = v.inputType as AttributeInputType
  if (v.valueRequired && !isScalarInputType(type)) {
    ctx.addIssue({
      code: 'custom',
      path: ['valueRequired'],
      message:
        'Required selectable attributes need a default - create this with "Value required" off, add values, then turn required on alongside the default on the detail page.',
    })
    return
  }
  if (v.valueRequired && !v.hasDefault) {
    ctx.addIssue({ code: 'custom', path: ['hasDefault'], message: 'Required attributes must have a default value.' })
  }
  if (!v.hasDefault || type === 'BOOLEAN') return
  if (type === 'NUMERIC' && !Number.isFinite(Number(v.defaultRaw))) {
    ctx.addIssue({ code: 'custom', path: ['defaultRaw'], message: 'Default value must be a number.' })
  } else if (!v.defaultRaw.trim()) {
    ctx.addIssue({ code: 'custom', path: ['defaultRaw'], message: 'Default value cannot be empty.' })
  }
})

export function toAttributeValues(a?: ExistingAttribute): AttributeValues {
  return {
    slug: a?.slug ?? '',
    name: a?.name ?? '',
    inputType: a?.inputType ?? 'PLAIN_TEXT',
    relationshipTarget: a?.relationshipTarget ?? 'PRODUCT',
    unit: a?.unit ?? '',
    valueRequired: a?.valueRequired ?? false,
    filterable: a?.filterable ?? false,
    searchable: a?.searchable ?? false,
    isPublic: a?.isPublic ?? true,
    facetCollapsed: a?.facetCollapsed ?? false,
    isIndexable: a?.isIndexable ?? false,
    h1Position: a?.h1Position ?? 'BEFORE',
    sortOrder: a?.sortOrder ?? 0,
    hasDefault: false,
    defaultRaw: '',
    defaultBool: false,
  }
}

function defaultValue(v: AttributeValues): unknown {
  const type = v.inputType as AttributeInputType
  if (!v.hasDefault || !isScalarInputType(type)) return undefined
  if (type === 'BOOLEAN') return v.defaultBool
  if (type === 'NUMERIC') return Number(v.defaultRaw)
  return v.defaultRaw.trim()
}

const flags = (v: AttributeValues) => ({
  valueRequired: v.valueRequired,
  filterable: v.filterable,
  searchable: v.searchable,
  isPublic: v.isPublic,
  facetCollapsed: v.facetCollapsed,
  isIndexable: v.isIndexable,
  h1Position: v.h1Position,
  sortOrder: v.sortOrder,
})

export function toAttributeCreateInput(v: AttributeValues) {
  const inputType = v.inputType as AttributeInputType
  return {
    slug: v.slug.trim(),
    name: v.name.trim(),
    inputType,
    relationshipTarget: inputType === 'REFERENCE' ? (v.relationshipTarget as AttributeReferenceTarget) : undefined,
    unit: v.unit || undefined,
    ...flags(v),
    defaultValue: defaultValue(v),
  }
}

export function toAttributeUpdateInput(v: AttributeValues, id: string) {
  return { id, name: v.name.trim(), unit: v.unit, ...flags(v) }
}
