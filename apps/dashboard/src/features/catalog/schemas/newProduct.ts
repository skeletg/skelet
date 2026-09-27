import { z } from 'zod'
import type { ProductAttributeAssignmentInput, ProductKind } from '@/graphql/schema.types'
import { currencyCode, dollarsToCents, money, optionalAbsoluteUrl } from '@/shared/schemas/fields'
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
      kind: z.enum(PRODUCT_KINDS).describe('What sort of product this is - fixes how it is sold and fulfilled'),
      productTypeId: z
        .string()
        .min(1, 'Pick a product type first')
        .describe('ID of the product type the product inherits its attributes and shipping rules from'),
      name: z.string().trim().min(1, 'Name is required').describe('Product name in the translation locale'),
      sku: z.string().describe('Stock keeping unit - empty string generates one from the name'),
      priceDollars: money().describe('Default price in major currency units written as text, such as "12.50"'),
      currencyCode: currencyCode().describe('Currency of the default price, as an upper-case ISO 4217 code such as USD'),
      locale: z
        .string()
        .refine((v) => (v || ctx.fallbackLocale) !== '', 'Pick a translation locale')
        .describe(`Locale code the first translation is written in - empty string uses ${ctx.fallbackLocale || 'the default locale'}`),
      // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
      shortDescription: z.unknown().describe('Short description, as Lexical rich-text JSON, or null'),
      shortDescriptionText: z.string().describe('Plain-text read of the short description - empty string means none'),
      description: z.unknown().describe('Full description, as Lexical rich-text JSON, or null'),
      descriptionText: z.string().describe('Plain-text read of the full description - empty string means none'),
      // The service refuses auto together with a path
      urlAuto: z.boolean().describe("Derive the URL path from the product type's template instead of setting url"),
      url: z.string().describe('Explicit URL path of the product - used only when urlAuto is off'),
      urlTemplateOverride: z
        .string()
        .describe("A URL path template such as {category}/{slug}, in place of the product type's - used only when urlAuto is on"),
      metaTitle: z.string().describe('SEO title - empty string for none'),
      metaDescription: z.string().describe('SEO description - empty string for none'),
      metaKeywords: z.string().describe('SEO keywords - empty string for none'),
      canonicalUrl: optionalAbsoluteUrl().describe('Absolute canonical URL for search engines - empty string for none'),
      seoNoindex: z.boolean().describe('Ask search engines not to index the product page'),
      attrState: z
        .record(z.string(), z.unknown())
        .describe(
          "Attribute values keyed by attribute ID, each { valueIds, scalarString, scalarBool, hasScalarValue } - required attributes of the product type must be set",
        ),
    })
    .describe('A new product: its basics and first translation')
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
