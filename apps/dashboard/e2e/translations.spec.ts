import { expect } from '@playwright/test'
import { adminApi, test } from './helpers'
import { RUN, cleanup, dashboardQueries, firstChannel, lexical, productTypeId, registerCleanups } from './fixtures'
import { typePolicies } from '../src/lib/apollo-type-policies'

// Translation rows carry no id, so Apollo keeps them under their parent record - and every query that
// selects the rows REPLACES the ones the cache holds for that record (typePolicies only merges row by
// row). A list query answering fewer locales than the record has therefore wiped the rows the detail
// page had loaded, and the next Save wrote the blanks back: a category's meta title, a collection's
// description. This spec runs every query the dashboard sends, over records seeded with two locales,
// and fails any response that carries a record with fewer locales than the record really has
//
// The fields checked are the typePolicies list itself, so a type added there is checked here too

registerCleanups()

const LOCALE = 'xw'

// Type -> its locale-row fields, straight from the cache config the dashboard runs with
const LOCALE_FIELDS: Record<string, string[]> = Object.fromEntries(
  Object.entries(typePolicies).map(([type, policy]) => [type, Object.keys((policy as { fields?: object }).fields ?? {})]),
)

type Seeded = { type: string; id: string }

async function seedAll(): Promise<Seeded[]> {
  await adminApi('mutation($c:String!){ deleteLocale(code:$c) }', { c: LOCALE }).catch(() => undefined)
  await adminApi('mutation($i:CreateLocaleInput!){ createLocale(input:$i){ code } }', { i: { code: LOCALE, name: 'E2E translations locale', isActive: true, sortOrder: 97 } })
  // Registered first, so it goes last - the rows below hold it by foreign key
  cleanup(() => adminApi('mutation($c:String!){ deleteLocale(code:$c) }', { c: LOCALE }))

  const seeded: Seeded[] = []
  const add = (type: string, id: string, remove: string) => {
    seeded.push({ type, id })
    cleanup(() => adminApi(`mutation($id:ID!){ ${remove}(id:$id) }`, { id }))
  }
  const channel = await firstChannel()
  const doc = lexical('Seeded copy')

  const { createCategory } = await adminApi<{ createCategory: { id: string } }>('mutation($i:CreateCategoryInput!){ createCategory(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-category`, locale: 'en', name: 'Seeded category', metaTitle: 'Seeded meta' },
  })
  add('Category', createCategory.id, 'deleteCategory')
  await adminApi('mutation($id:ID!,$i:UpsertCategoryContentInput!){ upsertCategoryContent(categoryId:$id, input:$i){ id } }', {
    id: createCategory.id,
    i: { locale: LOCALE, name: 'Other category', metaTitle: 'Other meta' },
  })

  const { createCollection } = await adminApi<{ createCollection: { id: string } }>('mutation($i:CreateCollectionInput!){ createCollection(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-collection`, locale: 'en', name: 'Seeded collection', description: doc },
  })
  add('Collection', createCollection.id, 'deleteCollection')
  await adminApi('mutation($id:ID!,$l:String!,$i:UpsertCollectionContentInput!){ upsertCollectionContent(collectionId:$id, locale:$l, input:$i){ id } }', {
    id: createCollection.id,
    l: LOCALE,
    i: { name: 'Other collection', description: doc },
  })

  const typeId = await productTypeId()
  const { createProduct } = await adminApi<{ createProduct: { id: string } }>('mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }', {
    i: { productTypeId: typeId, kind: 'SIMPLE', sku: `${RUN}-tr-product`, locale: 'en', name: 'Seeded product', defaultPrice: 1500, defaultCurrencyCode: channel.currencyCode },
  })
  add('Product', createProduct.id, 'deleteProduct')
  await adminApi('mutation($p:ID!,$l:String!,$i:UpsertProductContentInput!){ upsertProductContent(productId:$p, locale:$l, input:$i){ id } }', {
    p: createProduct.id,
    l: LOCALE,
    i: { name: 'Other product' },
  })
  // Listed on the channel, or the channel-scoped product lists never show it
  await adminApi('mutation($i:SetProductListingInput!){ setProductListing(input:$i){ id } }', { i: { productId: createProduct.id, channelId: channel.id, isPublished: true, price: 1500 } })

  const { createBlogPost } = await adminApi<{ createBlogPost: { id: string } }>('mutation($i:CreateBlogPostInput!){ createBlogPost(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-post`, locale: 'en', title: 'Seeded post', body: doc },
  })
  add('BlogPost', createBlogPost.id, 'deleteBlogPost')
  await adminApi('mutation($i:UpsertBlogPostContentInput!){ upsertBlogPostContent(input:$i){ id } }', { i: { postId: createBlogPost.id, locale: LOCALE, title: 'Other post', body: doc } })

  const { createBlogCategory } = await adminApi<{ createBlogCategory: { id: string } }>('mutation($i:CreateBlogCategoryInput!){ createBlogCategory(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-blog-category`, locale: 'en', name: 'Seeded blog category' },
  })
  add('BlogCategory', createBlogCategory.id, 'deleteBlogCategory')
  await adminApi('mutation($i:UpsertBlogCategoryContentInput!){ upsertBlogCategoryContent(input:$i){ id } }', { i: { categoryId: createBlogCategory.id, locale: LOCALE, name: 'Other blog category' } })

  const { createBlogTag } = await adminApi<{ createBlogTag: { id: string } }>('mutation($i:CreateBlogTagInput!){ createBlogTag(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-tag`, locale: 'en', name: 'Seeded tag' },
  })
  add('BlogTag', createBlogTag.id, 'deleteBlogTag')
  await adminApi('mutation($i:UpsertBlogTagContentInput!){ upsertBlogTagContent(input:$i){ id } }', { i: { tagId: createBlogTag.id, locale: LOCALE, name: 'Other tag' } })

  const { createBlogAuthor } = await adminApi<{ createBlogAuthor: { id: string } }>('mutation($i:CreateBlogAuthorInput!){ createBlogAuthor(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-author`, displayName: 'Seeded author', locale: 'en', name: 'Seeded author' },
  })
  add('BlogAuthor', createBlogAuthor.id, 'deleteBlogAuthor')
  await adminApi('mutation($i:UpsertBlogAuthorContentInput!){ upsertBlogAuthorContent(input:$i){ id } }', { i: { authorId: createBlogAuthor.id, locale: LOCALE, name: 'Other author' } })

  const { createCMSPageType } = await adminApi<{ createCMSPageType: { id: string } }>('mutation($i:CreateCMSPageTypeInput!){ createCMSPageType(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-page-type`, locale: 'en', name: 'Seeded page type' },
  })
  const pageTypeId = createCMSPageType.id
  add('CMSPageType', pageTypeId, 'deleteCMSPageType')
  await adminApi('mutation($i:UpsertCMSPageTypeContentInput!){ upsertCMSPageTypeContent(input:$i){ id } }', { i: { pageTypeId, locale: LOCALE, name: 'Other page type' } })

  const { createCMSPage } = await adminApi<{ createCMSPage: { id: string } }>('mutation($i:CreateCMSPageInput!){ createCMSPage(input:$i){ id } }', {
    i: { pageTypeId, slug: `${RUN}-tr-page`, locale: 'en', title: `${RUN} page`, metaTitle: 'Seeded meta' },
  })
  add('CMSPage', createCMSPage.id, 'deleteCMSPage')
  await adminApi('mutation($i:UpsertCMSPageContentInput!){ upsertCMSPageContent(input:$i){ id } }', { i: { pageId: createCMSPage.id, locale: LOCALE, title: 'Other page' } })
  // cmsPages lists only pages a channel carries
  await adminApi('mutation($i:SetCMSPageChannelListingInput!){ setCMSPageChannelListing(input:$i){ id } }', { i: { pageId: createCMSPage.id, channelId: channel.id, isPublished: false } })

  const { createCMSAttribute } = await adminApi<{ createCMSAttribute: { id: string } }>('mutation($i:CreateCMSAttributeInput!){ createCMSAttribute(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-cms-attr`, inputType: 'TEXT', isTranslatable: true },
  })
  add('CMSAttribute', createCMSAttribute.id, 'deleteCMSAttribute')
  for (const locale of ['en', LOCALE]) {
    await adminApi('mutation($i:UpsertCMSAttributeContentInput!){ upsertCMSAttributeContent(input:$i){ id } }', { i: { attributeId: createCMSAttribute.id, locale, name: `Attribute ${locale}` } })
  }

  const { createAttribute } = await adminApi<{ createAttribute: { id: string } }>('mutation($i:CreateAttributeInput!){ createAttribute(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-attr`, name: 'Seeded attribute', inputType: 'PLAIN_TEXT' },
  })
  add('Attribute', createAttribute.id, 'deleteAttribute')
  for (const locale of ['en', LOCALE]) {
    await adminApi('mutation($id:ID!,$l:String!,$d:String!){ setAttributeDescription(attributeId:$id, locale:$l, description:$d){ locale } }', { id: createAttribute.id, l: locale, d: `Described in ${locale}` })
  }

  const { createReviewQuestion } = await adminApi<{ createReviewQuestion: { id: string } }>('mutation($i:CreateReviewQuestionInput!){ createReviewQuestion(input:$i){ id } }', {
    i: { slug: `${RUN}-tr-question`, inputType: 'YES_NO', isRequired: false, allowComment: false, sortOrder: 0, locale: 'en', label: 'Seeded question' },
  })
  add('ReviewQuestion', createReviewQuestion.id, 'deleteReviewQuestion')
  await adminApi('mutation($i:UpsertReviewQuestionContentInput!){ upsertReviewQuestionContent(input:$i){ id } }', { i: { questionId: createReviewQuestion.id, locale: LOCALE, label: 'Other question' } })

  return seeded
}

// Lists that hide the seeds by default - unpublished, or past the first page - and the filter that shows them
const FILTERS: Record<string, unknown> = {
  BlogPosts: { includeUnpublished: true },
  CmsPages: { search: RUN, onlyPublished: false },
}

type Sighting = { op: string; locales: string[] }

/** Every record in a response that carries a locale-row field: `Type:id.field` -> the locales it listed */
function sightings(data: unknown, op: string, into: Map<string, Sighting[]>) {
  const walk = (node: unknown) => {
    if (Array.isArray(node)) return node.forEach(walk)
    if (!node || typeof node !== 'object') return
    const obj = node as Record<string, unknown>
    const type = obj.__typename as string | undefined
    if (type && typeof obj.id === 'string') {
      for (const field of LOCALE_FIELDS[type] ?? []) {
        const rows = obj[field]
        if (!Array.isArray(rows)) continue
        const key = `${type}:${obj.id}.${field}`
        const list = into.get(key) ?? []
        list.push({ op, locales: rows.map((r) => (r as { locale: string }).locale).sort() })
        into.set(key, list)
      }
    }
    Object.values(obj).forEach(walk)
  }
  walk(data)
}

test('every query answers a record with all of its translation rows', async () => {
  test.setTimeout(5 * 60_000)
  const seeded = await seedAll()
  const seen = new Map<string, Sighting[]>()
  const channel = await firstChannel()

  for (const q of dashboardQueries()) {
    // Queries a seeded id can answer run once per seed; ones that need nothing run once. Anything else
    // needs arguments this spec can't invent
    const known: Record<string, unknown> = { channelId: channel.id, includeUnpublished: true, limit: 500, ...(FILTERS[q.name] ? { filter: FILTERS[q.name] } : {}) }
    const pick = (extra: Record<string, unknown>) =>
      Object.fromEntries(Object.entries({ ...known, ...extra }).filter(([k]) => q.variables.includes(k)))
    const runs: Record<string, unknown>[] = []
    if (q.required.every((v) => v === 'id') && q.variables.includes('id')) for (const s of seeded) runs.push(pick({ id: s.id }))
    if (q.required.length === 0) runs.push({}, pick({}))
    for (const vars of runs) {
      const out = await adminApi(q.document, vars).catch(() => null)
      if (out) sightings(out, q.name, seen)
    }
  }

  const mine = Object.fromEntries(seeded.map((s) => [s.type, Object.fromEntries([...seen].filter(([k]) => k.includes(s.id)))]))
  await test.info().attach('sightings.json', { body: JSON.stringify(mine, null, 2), contentType: 'application/json' })
  if (process.env.E2E_DEBUG) {
    for (const [type, keys] of Object.entries(mine)) {
      const lines = Object.entries(keys).map(([k, list]) => `${k.split('.').pop()}: ${[...new Set(list.map((x) => `${x.op}=[${x.locales}]`))].join(' ')}`)
      console.log(`${type}: ${lines.join(' | ') || 'NO SIGHTINGS'}`)
    }
  }

  const short: string[] = []
  for (const s of seeded) {
    for (const field of LOCALE_FIELDS[s.type] ?? []) {
      const list = seen.get(`${s.type}:${s.id}.${field}`) ?? []
      const all = [...new Set(list.flatMap((x) => x.locales))].sort()
      for (const x of list) {
        if (x.locales.length < all.length) short.push(`${x.op} answered ${s.type}.${field} with [${x.locales.join(', ')}] - the record has [${all.join(', ')}]`)
      }
    }
  }
  // The seeds only prove something if some query read them back with both locales
  for (const s of seeded) {
    const both = (LOCALE_FIELDS[s.type] ?? []).some((f) => (seen.get(`${s.type}:${s.id}.${f}`) ?? []).some((x) => x.locales.includes(LOCALE) && x.locales.includes('en')))
    expect.soft(both, `no dashboard query read ${s.type} ${s.id} back with both locales - the seed or the query set is off`).toBe(true)
  }
  expect([...new Set(short)]).toEqual([])
})
