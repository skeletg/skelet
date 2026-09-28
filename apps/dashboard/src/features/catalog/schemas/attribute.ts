import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'
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

const baseSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required').describe('Stable identifier of the attribute - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the attribute'),
    inputType: z
      .string()
      .describe(`How values are entered, one of ${INPUT_TYPE_OPTIONS.join(', ')} - set on create, never changed`),
    relationshipTarget: z
      .string()
      .describe(`What a REFERENCE attribute's values point at, one of ${REFERENCE_TARGET_OPTIONS.join(', ')} - ignored for other types`),
    unit: z.string().describe('Unit shown after values, such as cm or kg - empty string for none'),
    valueRequired: z.boolean().describe('Products must provide a value for this attribute'),
    filterable: z.boolean().describe('Show the attribute as a filter facet on the storefront'),
    searchable: z.boolean().describe("Include the attribute's values in full-text product search"),
    isPublic: z.boolean().describe('Display the attribute on the product detail page'),
    facetCollapsed: z.boolean().describe('Start the filter facet collapsed rather than open - ignored unless filterable'),
    isIndexable: z
      .boolean()
      .describe('Master SEO opt-in: a filter value is indexable only when this and the per-value flag are both on'),
    h1Position: z
      .enum(['BEFORE', 'AFTER'])
      .describe('Where the value sits in the page H1 of an indexable filter URL: BEFORE "Olive / Apparel", AFTER "Apparel / Olive"'),
    sortOrder: wholeNumber().describe('Facet order on the storefront - lower first'),
    // Create-only default - the toggle, then a raw text value or a boolean depending on the type
    hasDefault: z.boolean().describe('Give new products a default value - only settable on create for scalar input types'),
    defaultRaw: z.string().describe('The default value as text, for PLAIN_TEXT, NUMERIC, DATE and DATE_TIME attributes'),
    defaultBool: z.boolean().describe('The default value for BOOLEAN attributes'),
  })
  .describe('A catalog attribute: a named property products carry, such as colour or size')

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
