import { expect } from '@playwright/test'
import { adminApi, saveThroughDialog, test } from './helpers'
import { RUN, cleanup, firstChannel, hasDraft, lexical, open, openScoped, registerCleanups } from './fixtures'

// The staged forms against the real API - what the unit tests' mocked Apollo can't show: that a Save
// writes what the review dialog listed, that sections sharing one backend row don't undo each other,
// and that a stored draft survives a reload. Every record is created here and deleted afterwards
//
// Needs the dev stack up, like the rest of the suite (see README.md)

registerCleanups()

// ---------- Product ----------

type ProductRead = {
  product: {
    name: string
    defaultPrice: number
    content: { locale: string; name: string; shortDescription: unknown; metaTitle: string | null }[]
    listings: { url: string | null; price: number | null; isPublished: boolean; channel: { id: string } }[]
  }
}

async function seedProduct(suffix: string) {
  const { productTypes } = await adminApi<{ productTypes: { id: string }[] }>('{ productTypes { id } }')
  if (!productTypes[0]) throw new Error('the dev stack has no product type')
  const channel = await firstChannel()
  const { createProduct } = await adminApi<{ createProduct: { id: string } }>(
    'mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }',
    { i: { productTypeId: productTypes[0].id, kind: 'SIMPLE', sku: `${RUN}-${suffix}`, locale: 'en', name: `${RUN} ${suffix}`, defaultPrice: 1500, defaultCurrencyCode: channel.currencyCode } },
  )
  const id = createProduct.id
  cleanup(() => adminApi('mutation($id:ID!){ deleteProduct(id:$id) }', { id }))
  await adminApi(
    `mutation($p:ID!,$c:ID!,$sd:JSON,$n:String!){
      upsertProductContent(productId:$p, locale:"en", input:{ name:$n, shortDescription:$sd, metaTitle:"Seeded meta" }){ id }
      setProductListing(input:{ productId:$p, channelId:$c, isPublished:true, price:1500, url:"${RUN}/${suffix}" }){ id }
    }`,
    { p: id, c: channel.id, sd: lexical('Seeded short description'), n: `${RUN} ${suffix}` },
  )
  return { id, channel }
}

const readProduct = (id: string) =>
  adminApi<ProductRead>(
    'query($id:ID!){ product(id:$id){ name defaultPrice content { locale name shortDescription metaTitle } listings { url price isPublished channel { id } } } }',
    { id },
  )

test.describe('product page', () => {
  test('one Save across sections keeps what each section does not own', async ({ page }) => {
    const { id, channel } = await seedProduct('sections')
    await open(page, `/products/${id}`)

    await page.locator('#pd-name').fill(`${RUN} renamed`)
    await page.locator('#pd-price').fill('19.99')
    await page.locator('button[class*="group/collapsible"]').filter({ hasText: /^#SEO/ }).click()
    await page.locator(`#churl-${channel.id}`).fill(`${RUN}/store-url`)

    await saveThroughDialog(page, ['Name', 'Default price', `${channel.name} URL`])

    const { product } = await readProduct(id)
    const en = product.content.find((c) => c.locale === 'en')
    expect(en?.name).toBe(`${RUN} renamed`)
    expect(product.defaultPrice).toBe(1999)
    // The rename used to replace the locale's content row with just a name
    expect(JSON.stringify(en?.shortDescription)).toContain('Seeded short description')
    expect(en?.metaTitle).toBe('Seeded meta')
    // The store URL used to be written with the price and publish flag the page loaded with
    const listing = product.listings.find((l) => l.channel.id === channel.id)
    expect(listing).toMatchObject({ url: `${RUN}/store-url`, price: 1500, isPublished: true })
  })

  test('an unsaved edit survives a reload as an offer, to restore or discard', async ({ page }) => {
    const { id } = await seedProduct('draft')
    await open(page, `/products/${id}`)
    page.on('dialog', (d) => {
      throw new Error(`unexpected browser dialog on reload: ${d.message()}`)
    })
    const typeAndReload = async () => {
      await page.locator('#pd-name').fill(`${RUN} typed`)
      // The draft is written after a short debounce
      await expect.poll(() => hasDraft(page)).toBe(true)
      await page.reload({ waitUntil: 'domcontentloaded' })
      // Offered, not applied: the field still shows what's stored
      await expect(page.getByRole('button', { name: 'Review', exact: true })).toBeVisible()
      await expect(page.locator('#pd-name')).toHaveValue(`${RUN} draft`)
    }

    await typeAndReload()
    await page.getByRole('button', { name: 'Review', exact: true }).click()
    const review = page.getByRole('alertdialog')
    await expect(review.getByText(`${RUN} typed`)).toBeVisible()
    await review.getByRole('button', { name: 'Restore' }).click()
    await expect(page.locator('#pd-name')).toHaveValue(`${RUN} typed`)
    await expect(page.getByRole('button', { name: 'Save changes' })).toBeEnabled()

    // Back to the stored name, then a second draft thrown away from the banner
    await page.locator('#pd-name').fill(`${RUN} draft`)
    await typeAndReload()
    await page.getByRole('button', { name: 'Discard' }).first().click()
    await expect(page.getByRole('button', { name: 'Review', exact: true })).toBeHidden()
    await expect(page.locator('#pd-name')).toHaveValue(`${RUN} draft`)
    await expect.poll(() => hasDraft(page)).toBe(false)
  })

  test('an invalid value keeps the review dialog open and writes nothing', async ({ page }) => {
    const { id } = await seedProduct('invalid')
    await open(page, `/products/${id}`)

    await page.locator('#pd-name').fill(' ')
    await page.getByRole('button', { name: 'Save changes' }).click()
    const dialog = page.getByRole('alertdialog')
    await dialog.getByRole('button', { name: /^Apply/ }).click()
    await expect(dialog.getByText(/Name is required/)).toBeVisible()
    await expect(dialog).toBeVisible()

    const { product } = await readProduct(id)
    expect(product.content.find((c) => c.locale === 'en')?.name).toBe(`${RUN} invalid`)
  })
})

// ---------- The other multi-write pages: edit one field, and check the rest of the record held ----------

type ContentRow = { locale: string; name: string; metaTitle: string | null; description: unknown }

test.describe('record pages', () => {
  // A zone away from UTC, so a datetime that shifts by the offset on each save shows up
  test.use({ timezoneId: 'America/New_York' })

  test('blog post: a new title keeps the body, excerpt and meta', async ({ page }) => {
    const { createBlogPost } = await adminApi<{ createBlogPost: { id: string } }>(
      'mutation($i:CreateBlogPostInput!){ createBlogPost(input:$i){ id } }',
      { i: { slug: `${RUN}-post`, locale: 'en', title: 'Seeded title', excerpt: 'Seeded excerpt', body: lexical('Seeded body'), metaTitle: 'Seeded meta' } },
    )
    const id = createBlogPost.id
    cleanup(() => adminApi('mutation($id:ID!){ deleteBlogPost(id:$id) }', { id }))

    await openScoped(page, `/blog/posts/${id}`)
    await page.locator('#title').fill('Renamed title')
    await saveThroughDialog(page, ['Title'])

    const { blogPost } = await adminApi<{ blogPost: { allContent: { locale: string; title: string; excerpt: string; body: unknown; metaTitle: string }[] } }>(
      'query($id:ID!){ blogPost(id:$id, includeUnpublished:true){ allContent { locale title excerpt body metaTitle } } }',
      { id },
    )
    const en = blogPost.allContent.find((c) => c.locale === 'en')
    expect(en).toMatchObject({ title: 'Renamed title', excerpt: 'Seeded excerpt', metaTitle: 'Seeded meta' })
    expect(JSON.stringify(en?.body)).toContain('Seeded body')
  })

  test('CMS page: a new title keeps the meta copy', async ({ page }) => {
    const { createCMSPageType } = await adminApi<{ createCMSPageType: { id: string } }>(
      'mutation($i:CreateCMSPageTypeInput!){ createCMSPageType(input:$i){ id } }',
      { i: { slug: `${RUN}-type`, locale: 'en', name: `${RUN} type` } },
    )
    const typeId = createCMSPageType.id
    cleanup(() => adminApi('mutation($id:ID!){ deleteCMSPageType(id:$id) }', { id: typeId }))
    const { createCMSPage } = await adminApi<{ createCMSPage: { id: string } }>(
      'mutation($i:CreateCMSPageInput!){ createCMSPage(input:$i){ id } }',
      { i: { pageTypeId: typeId, slug: `${RUN}-page`, locale: 'en', title: 'Seeded title', metaTitle: 'Seeded meta', metaDescription: 'Seeded description' } },
    )
    const id = createCMSPage.id
    cleanup(() => adminApi('mutation($id:ID!){ deleteCMSPage(id:$id) }', { id }))

    await openScoped(page, `/cms/pages/${id}`)
    await page.locator('#title').fill('Renamed title')
    await saveThroughDialog(page, ['Title'])

    const { cmsPage } = await adminApi<{ cmsPage: { allContent: { locale: string; title: string; metaTitle: string; metaDescription: string }[] } }>(
      'query($id:ID!){ cmsPage(id:$id){ allContent { locale title metaTitle metaDescription } } }',
      { id },
    )
    expect(cmsPage.allContent.find((c) => c.locale === 'en')).toMatchObject({ title: 'Renamed title', metaTitle: 'Seeded meta', metaDescription: 'Seeded description' })
  })

  test('promotion rule: a rename leaves the validity window where it was', async ({ page }) => {
    const validFrom = '2030-01-15T10:30:00Z'
    const { createPromotionRule } = await adminApi<{ createPromotionRule: { id: string } }>(
      'mutation($i:CreatePromotionRuleInput!){ createPromotionRule(input:$i){ id } }',
      { i: { slug: `${RUN}-rule`, name: 'Seeded rule', validFrom, isActive: false } },
    )
    const id = createPromotionRule.id
    cleanup(() => adminApi('mutation($id:ID!){ deletePromotionRule(id:$id) }', { id }))

    await open(page, `/promotions/${id}`)
    await page.locator('#name').fill('Renamed rule')
    await saveThroughDialog(page, ['Name'])

    const { promotionRule } = await adminApi<{ promotionRule: { name: string; validFrom: string } }>('query($id:ID!){ promotionRule(id:$id){ name validFrom } }', { id })
    expect(promotionRule.name).toBe('Renamed rule')
    expect(new Date(promotionRule.validFrom).toISOString()).toBe(new Date(validFrom).toISOString())
  })

  test('collection: a new meta title keeps the name and description', async ({ page }) => {
    const { createCollection } = await adminApi<{ createCollection: { id: string } }>(
      'mutation($i:CreateCollectionInput!){ createCollection(input:$i){ id } }',
      { i: { slug: `${RUN}-collection`, locale: 'en', name: 'Seeded collection', description: lexical('Seeded description') } },
    )
    const id = createCollection.id
    cleanup(() => adminApi('mutation($id:ID!){ deleteCollection(id:$id) }', { id }))

    await openScoped(page, `/collections/${id}`)
    await page.locator('#collection-content-meta-title').fill('New meta')
    await saveThroughDialog(page, ['Meta title'])

    // The content rows, not the top-level fields - those resolve through the request's locale
    const { collection } = await adminApi<{ collection: { content: ContentRow[] } }>(
      'query($id:ID!){ collection(id:$id){ content { locale name metaTitle description } } }',
      { id },
    )
    const en = collection.content.find((c) => c.locale === 'en')
    expect(en).toMatchObject({ name: 'Seeded collection', metaTitle: 'New meta' })
    expect(JSON.stringify(en?.description)).toContain('Seeded description')
  })

  test('category: a rename keeps the description', async ({ page }) => {
    const { createCategory } = await adminApi<{ createCategory: { id: string } }>(
      'mutation($i:CreateCategoryInput!){ createCategory(input:$i){ id } }',
      { i: { slug: `${RUN}-category`, locale: 'en', name: 'Seeded category', description: lexical('Seeded description'), metaTitle: 'Seeded meta' } },
    )
    const id = createCategory.id
    cleanup(() => adminApi('mutation($id:ID!){ deleteCategory(id:$id) }', { id }))

    // This install has no global default locale, so Global scope has no translation row to write
    await openScoped(page, `/categories/${id}`)
    await expect(page.locator('#cat-name')).toHaveValue('Seeded category')
    await page.locator('#cat-name').fill('Renamed category')
    await saveThroughDialog(page, ['Name'])

    const { category } = await adminApi<{ category: { content: ContentRow[] } }>(
      'query($id:ID!){ category(id:$id){ content { locale name metaTitle description } } }',
      { id },
    )
    const en = category.content.find((c) => c.locale === 'en')
    expect(en).toMatchObject({ name: 'Renamed category', metaTitle: 'Seeded meta' })
    expect(JSON.stringify(en?.description)).toContain('Seeded description')
  })

  test('menu and customer group: a rename is written', async ({ page }) => {
    const { createMenu } = await adminApi<{ createMenu: { id: string } }>('mutation($i:CreateMenuInput!){ createMenu(input:$i){ id } }', {
      i: { slug: `${RUN}-menu`, name: 'Seeded menu' },
    })
    cleanup(() => adminApi('mutation($id:ID!){ deleteMenu(id:$id) }', { id: createMenu.id }))
    const { createCustomerGroup } = await adminApi<{ createCustomerGroup: { id: string } }>(
      'mutation($i:CreateCustomerGroupInput!){ createCustomerGroup(input:$i){ id } }',
      { i: { slug: `${RUN}-group`, name: 'Seeded group', description: 'Seeded description' } },
    )
    cleanup(() => adminApi('mutation($id:ID!){ deleteCustomerGroup(id:$id) }', { id: createCustomerGroup.id }))

    await open(page, `/menus/${createMenu.id}`)
    await page.locator('#menu-edit-name').fill('Renamed menu')
    await saveThroughDialog(page, ['Name'])
    const { menu } = await adminApi<{ menu: { name: string } }>('query($id:ID!){ menu(id:$id){ name } }', { id: createMenu.id })
    expect(menu.name).toBe('Renamed menu')

    await page.goto(`/customers/groups/${createCustomerGroup.id}`, { waitUntil: 'domcontentloaded' })
    await page.locator('#cg-name').fill('Renamed group')
    await saveThroughDialog(page, ['Name'])
    const { customerGroup } = await adminApi<{ customerGroup: { name: string; description: string } }>(
      'query($id:ID!){ customerGroup(id:$id){ name description } }',
      { id: createCustomerGroup.id },
    )
    expect(customerGroup).toMatchObject({ name: 'Renamed group', description: 'Seeded description' })
  })
})
