import { z } from 'zod'
import type { ProductAttributeAssignmentInput, ProductKind } from '@/graphql/schema.types'
import { dollarsToCents, money } from '@/shared/schemas/fields'
import { buildAssignment, emptyAttrState, type AttrState, type AttributeSummary } from '../AttributesEditor'
import type { RichTextValue } from '../LexicalRichTextEditor'

// New product - basics and the first translation. Store listings, per-store prices, publishing,
// media and variants are set on the product's page afterwards

export const PRODUCT_KINDS = ['SIMPLE', 'CONFIGURABLE', 'BUNDLE', 'GROUPED', 'KIT', 'GIFTCARD', 'SUBSCRIPTION'] as const satisfies readonly ProductKind[]

const SELECTABLE = ['DROPDOWN', 'MULTISELECT', 'SWATCH']

// A required attribute left empty - selectables need a value picked, booleans are satisfied by any
// explicit value, everything else by non-blank text
function attributeMissing(a: AttributeSummary, s: AttrState | undefined): boolean {
  if (!a.valueRequired) return false
  if (!s) return true
  if (SELECTABLE.includes(a.inputType)) return s.valueIds.length === 0
  if (a.inputType === 'BOOLEAN') return false
  return !s.scalarString.trim()
}

// `locale` falls back to `fallbackLocale` when unpicked. The required attributes come from the chosen
// product type, which the form itself holds - so they are read when validation runs, not baked in.
// `attributes` answers null until that type's attributes have loaded, and the create waits for them
export function newProductSchema(ctx: { fallbackLocale: string; attributes: (productTypeId: string) => AttributeSummary[] | null }) {
  return z
    .object({
      kind: z.enum(PRODUCT_KINDS),
      productTypeId: z.string().min(1, 'Pick a product type first'),
      name: z.string().trim().min(1, 'Name is required'),
      // Blank lets the service generate one from the name
      sku: z.string(),
      priceDollars: money(),
      currencyCode: z.string().regex(/^[A-Z]{3}$/, 'A 3-letter ISO code such as USD'),
      locale: z.string().refine((v) => (v || ctx.fallbackLocale) !== '', 'Pick a translation locale'),
      // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
      shortDescription: z.unknown(),
      shortDescriptionText: z.string(),
      description: z.unknown(),
      descriptionText: z.string(),
      // Auto derives the path from the type's template; the service refuses auto together with a path
      urlAuto: z.boolean(),
      url: z.string(),
      urlTemplateOverride: z.string(),
      metaTitle: z.string(),
      metaDescription: z.string(),
      metaKeywords: z.string(),
      canonicalUrl: z.union([z.literal(''), z.url('An absolute URL, starting with https://')]),
      seoNoindex: z.boolean(),
      attrState: z.record(z.string(), z.unknown()),
    })
    .superRefine((v, rc) => {
      if (!v.productTypeId) return
      const attributes = ctx.attributes(v.productTypeId)
      if (attributes === null) {
        rc.addIssue({ code: 'custom', path: ['productTypeId'], message: "Still loading this type's attributes - try again in a moment" })
        return
      }
      for (const a of attributes) {
        if (attributeMissing(a, v.attrState[a.id] as AttrState | undefined)) {
          rc.addIssue({ code: 'custom', path: ['attrState', a.id], message: `${a.name} is required` })
        }
      }
    })
}

export type NewProductValues = z.infer<ReturnType<typeof newProductSchema>>

export const emptyNewProduct: NewProductValues = {
  kind: 'SIMPLE',
  productTypeId: '',
  name: '',
  sku: '',
  priceDollars: '0.00',
  currencyCode: 'USD',
  locale: '',
  shortDescription: null,
  shortDescriptionText: '',
  description: null,
  descriptionText: '',
  urlAuto: true,
  url: '',
  urlTemplateOverride: '',
  metaTitle: '',
  metaDescription: '',
  metaKeywords: '',
  canonicalUrl: '',
  seoNoindex: false,
  attrState: {},
}

const blankToNull = (v: string) => v.trim() || null

export function toNewProductInput(v: NewProductValues, locale: string) {
  return {
    productTypeId: v.productTypeId,
    kind: v.kind,
    name: v.name.trim(),
    locale,
    shortDescription: v.shortDescriptionText.trim() ? (v.shortDescription as RichTextValue) : undefined,
    descriptionRichText: v.descriptionText.trim() ? (v.description as RichTextValue) : undefined,
    sku: v.sku.trim(),
    defaultPrice: dollarsToCents(v.priceDollars),
    defaultCurrencyCode: v.currencyCode,
    // Auto-generate (optionally with a template override) OR an explicit path - never both
    urlAuto: v.urlAuto,
    url: v.urlAuto ? null : blankToNull(v.url),
    urlTemplateOverride: v.urlAuto ? blankToNull(v.urlTemplateOverride) : null,
    metaTitle: blankToNull(v.metaTitle),
    metaDescription: blankToNull(v.metaDescription),
    metaKeywords: blankToNull(v.metaKeywords),
    canonicalUrl: blankToNull(v.canonicalUrl),
    seoNoindex: v.seoNoindex,
  }
}

// The attribute values worth writing - an untouched attribute sends nothing
export function toAttributeAssignments(v: NewProductValues, attributes: AttributeSummary[]): ProductAttributeAssignmentInput[] {
  return attributes
    .map((a) => buildAssignment(a, (v.attrState[a.id] as AttrState | undefined) ?? emptyAttrState()))
    .filter((a) => (a.valueIds != null ? a.valueIds.length > 0 : a.scalarValue !== null && a.scalarValue !== undefined))
}
