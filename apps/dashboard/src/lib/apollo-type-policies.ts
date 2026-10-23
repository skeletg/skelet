import type { FieldMergeFunction, TypePolicies } from '@apollo/client'

// Translation rows carry no id, so the cache can't normalise them: a query selecting fewer of their
// fields replaced the rows another query had fetched in full. The category tree's
// `content { locale name }` landing after the detail query handed the category form a blank meta
// title, and the next save wrote that blank over the stored one. Merged by locale instead - the
// incoming list still decides which locales exist (a removed translation goes), while each row
// keeps the fields only the other query selected

type LocaleRow = { locale?: unknown }
type AttributeRow = { attribute?: { __ref?: string } | null }

function mergeRowsBy<Row extends object>(key: (row: Row) => unknown): FieldMergeFunction<readonly Row[], readonly Row[]> {
  return (existing, incoming) => {
    if (!existing?.length) return incoming
    return incoming.map((row) => {
      const before = existing.find((e) => key(e) === key(row))
      return before ? { ...before, ...row } : row
    })
  }
}

export const mergeByLocale = mergeRowsBy<LocaleRow>((r) => r.locale)

// A type's attribute rows are the same trap keyed by the attribute: the product picker's type filter
// selects them without groupId, which left the product page's query incomplete, so it refetched and
// remounted the page - closing the picker it had just opened
export const mergeByAttribute = mergeRowsBy<AttributeRow>((r) => r.attribute?.__ref)

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

const ATTRIBUTE_ROWS: Record<string, string[]> = {
  AttributeGroup: ['attributes'],
  ProductType: ['attributes'],
}

export const typePolicies: TypePolicies = {}
for (const [rows, merge] of [[LOCALE_ROWS, mergeByLocale], [ATTRIBUTE_ROWS, mergeByAttribute]] as const) {
  for (const [type, fields] of Object.entries(rows)) {
    const policy = (typePolicies[type] ??= { fields: {} })
    for (const f of fields) policy.fields![f] = { merge }
  }
}
