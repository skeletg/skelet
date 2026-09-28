import { expect, type Page } from '@playwright/test'
import { adminApi, test } from './helpers'
import { RUN, cleanup, draftKeys, open, openScoped, registerCleanups, submitCreate } from './fixtures'

// The create pages against the real API: fill a page in, press its create button, and read back what
// the server stored. A create writes straight through - there is no review dialog to catch a field the
// mutation dropped or a value it mangled - so this is the only place a create's payload is checked.
// Also: the page's stored draft for "new" is gone once the record exists, and a rejected create writes
// nothing
//
// Needs the dev stack up (see README.md)

registerCleanups()

const del = (mutation: string, value: string, arg = 'id', type = 'ID!') => () => adminApi(`mutation($v:${type}){ ${mutation}(${arg}:$v) }`, { v: value })

/** The id a create page moved to - the last path segment, or the one before a trailing /edit */
const idFrom = (path: string) => path.replace(/\/edit$/, '').split('/').pop() ?? ''

async function fill(page: Page, fields: Record<string, string>) {
  for (const [selector, value] of Object.entries(fields)) await page.locator(selector).fill(value)
}

/** Picks an option in a shadcn Select, by the trigger's id and the option's visible text */
async function choose(page: Page, triggerId: string, option: string | RegExp) {
  await page.locator(triggerId).click()
  await page.getByRole('option', { name: option }).first().click()
}

/** No stored draft for a "new" page survives the create */
async function expectNoNewDraft(page: Page) {
  await expect.poll(async () => (await draftKeys(page)).filter((k) => k.includes(':new'))).toEqual([])
}

test('customer', async ({ page }) => {
  const email = `${RUN}-new-customer@e2e.test`
  await openScoped(page, '/customers/new')
  await fill(page, { '#c-first': 'Created', '#c-last': 'Customer', '#c-email': email, '#c-phone': '+381601112233', '#c-password': 'E2e-password-1234!' })
  const id = idFrom(await submitCreate(page, 'Create customer'))
  cleanup(del('deleteCustomer', id))
  const { customer } = await adminApi<{ customer: Record<string, unknown> }>('query($id:ID!){ customer(id:$id){ firstName lastName email phone isActive } }', { id })
  expect(customer).toMatchObject({ firstName: 'Created', lastName: 'Customer', email, phone: '+381601112233' })
  await expectNoNewDraft(page)
})

test('blog post', async ({ page }) => {
  await openScoped(page, '/blog/posts/new')
  await fill(page, { '#title': 'Created post', '#slug': `${RUN}-new-post`, '#excerpt': 'Created excerpt' })
  const id = idFrom(await submitCreate(page, 'Create post'))
  cleanup(del('deleteBlogPost', id))
  const { blogPost } = await adminApi<{ blogPost: { slug: string; allContent: { locale: string; title: string; excerpt: unknown }[] } }>(
    'query($id:ID!){ blogPost(id:$id, includeUnpublished:true){ slug allContent { locale title excerpt } } }',
    { id },
  )
  expect(blogPost.slug).toBe(`${RUN}-new-post`)
  expect(blogPost.allContent).toHaveLength(1)
  expect(blogPost.allContent[0].title).toBe('Created post')
  expect(JSON.stringify(blogPost.allContent[0].excerpt)).toContain('Created excerpt')
  await expectNoNewDraft(page)
})

test('collection', async ({ page }) => {
  await openScoped(page, '/collections/new')
  await fill(page, { '#collection-name': 'Created collection', '#collection-slug': `${RUN}-new-collection`, '#collection-meta-title': 'Created meta' })
  // The locale falls back to the default once the channels load
  await expect(page.locator('#collection-locale')).toContainText('en')
  const id = idFrom(await submitCreate(page, 'Create collection'))
  cleanup(del('deleteCollection', id))
  const { collection } = await adminApi<{ collection: { slug: string; content: { name: string; metaTitle: string }[] } }>(
    'query($id:ID!){ collection(id:$id){ slug content { locale name metaTitle } } }',
    { id },
  )
  expect(collection.slug).toBe(`${RUN}-new-collection`)
  expect(collection.content).toEqual([expect.objectContaining({ name: 'Created collection', metaTitle: 'Created meta' })])
  await expectNoNewDraft(page)
})

test('CMS page type, then a page of that type', async ({ page }) => {
  await openScoped(page, '/cms/page-types/new')
  await fill(page, { '#pt-name': 'Created page type', '#pt-slug': `${RUN}-new-page-type`, '#pt-description': 'Created description' })
  const typeId = idFrom(await submitCreate(page, 'Create page type'))
  cleanup(del('deleteCMSPageType', typeId))
  const { cmsPageType } = await adminApi<{ cmsPageType: { slug: string; allContent: { name: string; description: string }[] } }>(
    'query($id:ID!){ cmsPageType(id:$id){ slug allContent { locale name description } } }',
    { id: typeId },
  )
  expect(cmsPageType.slug).toBe(`${RUN}-new-page-type`)
  expect(cmsPageType.allContent).toEqual([expect.objectContaining({ name: 'Created page type', description: 'Created description' })])

  await page.goto('/cms/pages/new', { waitUntil: 'domcontentloaded' })
  // The picker names types by slug
  await choose(page, '#page-type', `${RUN}-new-page-type`)
  await fill(page, { '#slug': `${RUN}-new-page`, '#title': 'Created page' })
  const pageId = idFrom(await submitCreate(page, 'Create page'))
  cleanup(del('deleteCMSPage', pageId))
  const { cmsPage } = await adminApi<{ cmsPage: { slug: string; pageTypeId: string; allContent: { title: string }[] } }>(
    'query($id:ID!){ cmsPage(id:$id){ slug pageTypeId allContent { locale title } } }',
    { id: pageId },
  )
  expect(cmsPage).toMatchObject({ slug: `${RUN}-new-page`, pageTypeId: typeId })
  expect(cmsPage.allContent).toEqual([expect.objectContaining({ title: 'Created page' })])
  await expectNoNewDraft(page)
})

test('CMS attribute', async ({ page }) => {
  await openScoped(page, '/cms/attributes/new')
  await fill(page, { '#attr-slug': `${RUN}-new-attr`, '#attr-name': 'Created attribute' })
  await page.getByLabel('Translatable').click()
  const id = idFrom(await submitCreate(page, 'Create attribute'))
  cleanup(del('deleteCMSAttribute', id))
  const { cmsAttributes } = await adminApi<{ cmsAttributes: { id: string; slug: string; isTranslatable: boolean; allContent: { name: string }[] }[] }>(
    '{ cmsAttributes { id slug isTranslatable allContent { locale name } } }',
  )
  const row = cmsAttributes.find((a) => a.id === id)
  expect(row).toMatchObject({ slug: `${RUN}-new-attr`, isTranslatable: true })
  expect(row?.allContent).toEqual([expect.objectContaining({ name: 'Created attribute' })])
  await expectNoNewDraft(page)
})

test('menu', async ({ page }) => {
  await open(page, '/menus/new')
  await fill(page, { '#menu-name': 'Created menu', '#menu-slug': `${RUN}-new-menu` })
  const id = idFrom(await submitCreate(page, 'Create menu'))
  cleanup(del('deleteMenu', id))
  const { menu } = await adminApi<{ menu: { name: string; slug: string } }>('query($id:ID!){ menu(id:$id){ name slug } }', { id })
  expect(menu).toEqual({ name: 'Created menu', slug: `${RUN}-new-menu` })
  await expectNoNewDraft(page)
})

test('role keeps the scopes it was created with', async ({ page }) => {
  await open(page, '/admins/roles/new')
  await fill(page, { '#role-name': 'Created role', '#role-slug': `${RUN}-new-role`, '#role-description': 'Created description' })
  // The first permission on the page, granted everywhere
  await page.getByRole('switch').first().click()
  const id = idFrom(await submitCreate(page, 'Create role'))
  cleanup(del('deleteRole', id))
  const { role } = await adminApi<{ role: { name: string; slug: string; description: string; scopes: unknown[] } }>(
    'query($id:ID!){ role(id:$id){ name slug description scopes { channelId locale permission { slug } } } }',
    { id },
  )
  expect(role).toMatchObject({ name: 'Created role', slug: `${RUN}-new-role`, description: 'Created description' })
  expect(role.scopes.length).toBeGreaterThan(0)
  await expectNoNewDraft(page)
})

test('language, currency and country go back to their lists with the record stored', async ({ page }) => {
  for (const [kind, code] of [['Locale', 'xv'], ['Currency', 'XVV'], ['Country', 'XV']] as const) {
    await adminApi(`mutation($c:String!){ delete${kind}(code:$c) }`, { c: code }).catch(() => undefined)
  }

  await open(page, '/locales/new')
  await fill(page, { '#locale-code': 'xv', '#locale-name': 'Created language', '#locale-native': 'Created native' })
  await submitCreate(page, 'Create language')
  cleanup(del('deleteLocale', 'xv', 'code', 'String!'))
  expect((await adminApi<{ locale: unknown }>('query($c:String!){ locale(code:$c){ name nativeName } }', { c: 'xv' })).locale).toEqual({
    name: 'Created language',
    nativeName: 'Created native',
  })

  await page.goto('/currencies/new', { waitUntil: 'domcontentloaded' })
  await fill(page, { '#currency-code': 'XVV', '#currency-name': 'Created coin', '#currency-symbol': 'V' })
  await submitCreate(page, 'Create currency')
  cleanup(del('deleteCurrency', 'XVV', 'code', 'String!'))
  expect((await adminApi<{ currency: unknown }>('query($c:String!){ currency(code:$c){ name symbol } }', { c: 'XVV' })).currency).toEqual({
    name: 'Created coin',
    symbol: 'V',
  })

  await page.goto('/countries/new', { waitUntil: 'domcontentloaded' })
  await fill(page, { '#country-code': 'XV', '#country-name': 'Created land', '#country-phone': '+998' })
  await submitCreate(page, 'Create country')
  cleanup(del('deleteCountry', 'XV', 'code', 'String!'))
  expect((await adminApi<{ country: unknown }>('query($c:String!){ country(code:$c){ name phoneCode } }', { c: 'XV' })).country).toEqual({
    name: 'Created land',
    phoneCode: '+998',
  })
  await expectNoNewDraft(page)
})

test('a duplicate language code shows under the field and writes nothing', async ({ page }) => {
  await open(page, '/locales/new')
  await fill(page, { '#locale-code': 'en', '#locale-name': 'Duplicate English' })
  await page.getByRole('button', { name: 'Create language' }).click()
  await expect(page.getByText('Create failed')).toBeVisible()
  // fieldErrorsFrom routes the server's message to the code field
  await expect(page.locator('#locale-code')).toHaveAttribute('aria-invalid', 'true')
  await expect(page).toHaveURL(/\/locales\/new$/)
  const { locale } = await adminApi<{ locale: { name: string } }>('query($c:String!){ locale(code:$c){ name } }', { c: 'en' })
  expect(locale.name).not.toBe('Duplicate English')
})

test('a create with a required field empty writes nothing', async ({ page }) => {
  await open(page, '/menus/new')
  await fill(page, { '#menu-slug': `${RUN}-nameless-menu` })
  await page.getByRole('button', { name: 'Create menu' }).click()
  await expect(page.locator('#menu-name')).toHaveAttribute('aria-invalid', 'true')
  await expect(page).toHaveURL(/\/menus\/new$/)
  const { menus } = await adminApi<{ menus: { slug: string }[] }>('{ menus { slug } }')
  expect(menus.map((m) => m.slug)).not.toContain(`${RUN}-nameless-menu`)
})

test('a new page offers its draft after a reload, and creates from it', async ({ page }) => {
  const email = `${RUN}-drafted-customer@e2e.test`
  await openScoped(page, '/customers/new')
  await fill(page, { '#c-first': 'Drafted', '#c-last': 'Customer', '#c-email': email })
  await expect.poll(async () => (await draftKeys(page, ':new')).length).toBeGreaterThan(0)
  await page.reload({ waitUntil: 'domcontentloaded' })

  // Offered, not applied
  await expect(page.getByRole('button', { name: 'Review', exact: true })).toBeVisible()
  await expect(page.locator('#c-first')).toHaveValue('')
  await page.getByRole('button', { name: 'Review', exact: true }).click()
  const review = page.getByRole('alertdialog')
  await expect(review.getByText('Drafted', { exact: true })).toBeVisible()
  await review.getByRole('button', { name: 'Restore' }).click()
  await expect(page.locator('#c-email')).toHaveValue(email)

  await fill(page, { '#c-password': 'E2e-password-1234!' })
  const id = idFrom(await submitCreate(page, 'Create customer'))
  cleanup(del('deleteCustomer', id))
  const { customer } = await adminApi<{ customer: { firstName: string; email: string } }>('query($id:ID!){ customer(id:$id){ firstName email } }', { id })
  expect(customer).toEqual({ firstName: 'Drafted', email })
  await expectNoNewDraft(page)
})
