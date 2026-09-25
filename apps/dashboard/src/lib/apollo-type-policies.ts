import type { FieldMergeFunction, TypePolicies } from '@apollo/client'

// Translation rows carry no id, so the cache can't normalise them: a query selecting fewer of their
// fields replaced the rows another query had fetched in full. The category tree's
// `content { locale name }` landing after the detail query handed the category form a blank meta
// title, and the next save wrote that blank over the stored one. Merged by locale instead - the
// incoming list still decides which locales exist (a removed translation goes), while each row
// keeps the fields only the other query selected

type LocaleRow = { locale?: unknown }

export const mergeByLocale: FieldMergeFunction<readonly LocaleRow[], readonly LocaleRow[]> = (existing, incoming) => {
  if (!existing?.length) return incoming
  return incoming.map((row) => {
    const before = existing.find((e) => e.locale === row.locale)
    return before ? { ...before, ...row } : row
  })
}

// Every id-less, locale-keyed list the admin schema has - found by introspection: object types
// with an id whose list field's element type has a `locale` and no `id`
const LOCALE_ROWS: Record<string, string[]> = {
  Attribute: ['descriptions'],
  AttributeValue: ['descriptions'],
  BlogAuthor: ['allContent'],
  BlogCategory: ['allContent'],
  BlogPost: ['allContent'],
  BlogTag: ['allContent'],
  BundleOption: ['content'],
  CMSAttribute: ['allContent'],
  CMSAttributeValue: ['allContent'],
  CMSPage: ['allContent'],
  CMSPageType: ['allContent'],
  Category: ['content'],
  Collection: ['content'],
  MenuItem: ['translations'],
  Product: ['content'],
  ProductOption: ['content'],
  ProductOptionValue: ['content'],
  PromotionRule: ['contents'],
  ReviewQuestion: ['allContent'],
}

export const typePolicies: TypePolicies = Object.fromEntries(
  Object.entries(LOCALE_ROWS).map(([type, fields]) => [type, { fields: Object.fromEntries(fields.map((f) => [f, { merge: mergeByLocale }])) }]),
)
