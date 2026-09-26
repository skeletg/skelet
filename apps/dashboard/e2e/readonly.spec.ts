import { expect, type Browser, type Page } from '@playwright/test'
import { adminApi, test } from './helpers'
import { RUN, cleanup, draftKeys, firstChannel, registerCleanups } from './fixtures'

// An admin who may look but not change: every staged-save page they can open must render read-only -
// no header Save, every input disabled - and must not write drafts for work they can't save. Signs in
// as a throwaway admin on a view-only role, once, since the login is rate limited
//
// Needs the dev stack up (see README.md)

registerCleanups()

// Viewing, never managing - wide enough to open every list page that offers a create button
const VIEW_ONLY = [
  'catalog.view',
  'products.view',
  'customers.view',
  'blog.view',
  'cms.view',
  'orders.view',
  'promotions.view',
  'locales.view',
  'currencies.view',
  'countries.view',
  'admins.view',
  'experiments.view',
  'campaigns.view',
  'flows.view',
  'webforms.view',
  'emails.view',
]

const PASSWORD = 'E2e-readonly-1234!'
const email = `${RUN}-readonly@e2e.test`

type Seeds = { product: string; group: string; blogCategory: string; cmsAttribute: string }
let seeds: Seeds
let viewer: Page

test.beforeAll(async ({ browser }) => {
  const { createRole } = await adminApi<{ createRole: { id: string; slug: string } }>('mutation($i:CreateRoleInput!){ createRole(input:$i){ id slug } }', {
    i: {
      name: 'E2E read-only',
      slug: `${RUN}-readonly`,
      scopes: VIEW_ONLY.map((permissionSlug) => ({ permissionSlug })),
    },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteRole(id:$id) }', { id: createRole.id }))
  const { createAdmin } = await adminApi<{ createAdmin: { id: string } }>('mutation($i:CreateAdminInput!){ createAdmin(input:$i){ id } }', {
    i: { email, password: PASSWORD, firstName: 'Read', lastName: 'Only', roleSlugs: [createRole.slug] },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteAdmin(id:$id) }', { id: createAdmin.id }))

  seeds = await seed()
  viewer = await signIn(browser)
})

test.afterAll(async () => {
  await viewer?.context().close()
})

async function seed(): Promise<Seeds> {
  const channel = await firstChannel()
  const { productTypes } = await adminApi<{ productTypes: { id: string }[] }>('{ productTypes { id } }')
  const { createProduct } = await adminApi<{ createProduct: { id: string } }>('mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }', {
    i: { productTypeId: productTypes[0]?.id, kind: 'SIMPLE', sku: `${RUN}-readonly`, locale: 'en', name: 'Read-only product', defaultPrice: 1500, defaultCurrencyCode: channel.currencyCode },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteProduct(id:$id) }', { id: createProduct.id }))
  const { createCustomerGroup } = await adminApi<{ createCustomerGroup: { id: string } }>('mutation($i:CreateCustomerGroupInput!){ createCustomerGroup(input:$i){ id } }', {
    i: { slug: `${RUN}-readonly-group`, name: 'Read-only group' },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteCustomerGroup(id:$id) }', { id: createCustomerGroup.id }))
  const { createBlogCategory } = await adminApi<{ createBlogCategory: { id: string } }>('mutation($i:CreateBlogCategoryInput!){ createBlogCategory(input:$i){ id } }', {
    i: { slug: `${RUN}-readonly-category`, locale: 'en', name: 'Read-only category' },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteBlogCategory(id:$id) }', { id: createBlogCategory.id }))
  const { createCMSAttribute } = await adminApi<{ createCMSAttribute: { id: string } }>('mutation($i:CreateCMSAttributeInput!){ createCMSAttribute(input:$i){ id } }', {
    i: { slug: `${RUN}-readonly-attr`, inputType: 'TEXT', defaultValue: 'Read-only default' },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteCMSAttribute(id:$id) }', { id: createCMSAttribute.id }))
  return { product: createProduct.id, group: createCustomerGroup.id, blogCategory: createBlogCategory.id, cmsAttribute: createCMSAttribute.id }
}

async function signIn(browser: Browser): Promise<Page> {
  const context = await browser.newContext({ storageState: { cookies: [], origins: [] } })
  const page = await context.newPage()
  await page.goto('/login', { waitUntil: 'domcontentloaded' })
  await page.fill('#email', email)
  await page.fill('#password', PASSWORD)
  await page.click('button[type=submit]')
  await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 45_000 })
  return page
}

/** Opens a page as the read-only admin and waits for it to settle */
async function view(path: string, ready: string) {
  await viewer.goto(path, { waitUntil: 'domcontentloaded' })
  await viewer.waitForLoadState('networkidle')
  await expect(viewer.locator(ready)).toBeVisible()
}

async function expectReadOnly(inputs: string[]) {
  await expect(viewer.getByRole('button', { name: /^(Save changes|No changes)$/ })).toHaveCount(0)
  for (const selector of inputs) await expect(viewer.locator(selector)).toBeDisabled()
  // Nothing typed, nothing to keep - and a page that can't save must not start a draft on mount
  await viewer.waitForTimeout(800)
  expect(await draftKeys(viewer)).toEqual([])
}

test('product page', async () => {
  await view(`/products/${seeds.product}`, '#pd-name')
  await expect(viewer.locator('#pd-name')).toHaveValue('Read-only product')
  await expectReadOnly(['#pd-name', '#pd-price'])
})

test('customer group page', async () => {
  await view(`/customers/groups/${seeds.group}`, '#cg-name')
  await expectReadOnly(['#cg-name', '#cg-desc'])
})

test('blog category page', async () => {
  await view(`/blog/categories/${seeds.blogCategory}`, '#name')
  await expectReadOnly(['#name', '#slug'])
})

test('CMS attribute page', async () => {
  await view(`/cms/attributes/${seeds.cmsAttribute}`, '#attr-default')
  await expectReadOnly(['#attr-default'])
})

// Every list with a create button, opened by someone who may only look. A "New …" there led to an
// access-restricted page or a refused save - the button has to follow the same permission its route
// or mutation checks
const LISTS = [
  '/customers',
  '/orders',
  '/carts',
  '/products',
  '/catalog/attributes',
  '/catalog/product-types',
  '/cms/attributes',
  '/locales',
  '/currencies',
  '/countries',
  '/admins',
  '/promotions',
  '/segments',
  '/prism',
  '/campaigns',
  '/flows',
  '/cms/forms',
  '/emails',
]

test('no list offers to create what the admin may only view', async () => {
  test.setTimeout(LISTS.length * 20_000)
  // A store picked in the header - several of these only show their create button in one
  const channel = await firstChannel()
  await viewer.evaluate(
    (scope) => localStorage.setItem('skelet-dash:config-scope', scope),
    JSON.stringify({ mode: 'CHANNEL', channelId: channel.id, locale: 'en' }),
  )
  for (const path of LISTS) {
    await test.step(path, async () => {
      await view(path, 'main main')
      const page = viewer.locator('main').last()
      await expect.soft(page.getByRole('button', { name: /^(New|Create)\b/ }), `${path} offers a create button`).toHaveCount(0)
      // By where it points - a row can be called "New Zealand"
      await expect.soft(page.locator('a[href$="/new"], a[href*="/new?"]'), `${path} offers a create link`).toHaveCount(0)
      // Row selection only exists to feed bulk actions - disable, delete, publish
      await expect.soft(page.getByLabel('Select all'), `${path} offers bulk actions`).toHaveCount(0)
    })
  }
})

test('an email block can be looked at and previewed, not changed', async () => {
  const channel = await firstChannel()
  const slug = `${RUN}-readonly-block`
  await adminApi('mutation($i:SaveEmailBlockInput!){ saveEmailBlock(input:$i){ slug } }', {
    i: { channelId: channel.id, slug, name: 'Read-only block', blocks: [], htmlTemplate: '<p>Read-only block</p>' },
  })
  cleanup(() => adminApi('mutation($c:ID!,$s:String!){ deleteEmailBlock(channelId:$c, slug:$s) }', { c: channel.id, s: slug }))

  // Blocks are per store - the header has to have one picked
  await viewer.evaluate(
    (scope) => localStorage.setItem('skelet-dash:config-scope', scope),
    JSON.stringify({ mode: 'CHANNEL', channelId: channel.id, locale: 'en' }),
  )
  await view(`/emails/blocks/${slug}`, '#block-name')
  await expect(viewer.locator('#block-name')).toHaveValue('Read-only block')
  await expect(viewer.locator('#block-name')).toBeDisabled()
  await expect(viewer.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0)
  await expect(viewer.getByRole('button', { name: 'Delete', exact: true })).toHaveCount(0)
  // Looking includes previewing
  await expect(viewer.getByRole('button', { name: 'Preview' })).toBeEnabled()
})

test('admins and roles can be looked at, not changed', async () => {
  const { admins } = await adminApi<{ admins: { id: string; email: string }[] }>('{ admins { id email } }')
  const other = admins.find((a) => a.email !== email)
  const { roles } = await adminApi<{ roles: { id: string }[] }>('{ roles { id } }')

  await view('/admins', 'main main')
  const actions = viewer.getByRole('button', { name: /^Actions for / }).first()
  await actions.click()
  await expect(viewer.getByRole('menuitem', { name: 'View' })).toBeVisible()
  await expect(viewer.getByRole('menuitem', { name: /^Delete/ })).toHaveCount(0)
  await viewer.keyboard.press('Escape')

  if (other) {
    await view(`/admins/${other.id}`, 'main main')
    await expect(viewer.getByRole('button', { name: /^Delete admin/ })).toHaveCount(0)
    await expect(viewer.getByRole('button', { name: /^(Save changes|No changes)$/ })).toHaveCount(0)
  }
  await view(`/admins/roles/${roles[0].id}`, 'main main')
  await expect(viewer.getByRole('link', { name: 'Edit' })).toHaveCount(0)
  await expect(viewer.getByRole('button', { name: 'Delete' })).toHaveCount(0)
  await expect(viewer.getByRole('button', { name: /Add members/ })).toHaveCount(0)
})

test('the read-only admin can still change their own password', async () => {
  await viewer.goto('/', { waitUntil: 'domcontentloaded' })
  await viewer.waitForLoadState('networkidle')
  await viewer.getByRole('button', { name: new RegExp(email.replace(/[.+]/g, '\\$&')) }).last().click()
  await viewer.getByRole('menuitem', { name: 'Change password' }).click()
  const dialog = viewer.getByRole('dialog')
  const next = 'E2e-readonly-5678!'
  await dialog.locator('#cp-current').fill(PASSWORD)
  await dialog.locator('#cp-new').fill(next)
  await dialog.locator('#cp-confirm').fill(next)
  await dialog.getByRole('button', { name: 'Change password' }).click()
  await expect(dialog.getByText('Password changed')).toBeVisible()
  await dialog.getByRole('button', { name: 'Done' }).click()
  await expect(dialog).toBeHidden()
  expect(await draftKeys(viewer)).toEqual([])
  // The new password signs in over the API - and the old one no longer does
  const login = (password: string) =>
    viewer.request
      .post(`${process.env.E2E_API_URL ?? 'http://localhost:8080'}${process.env.ADMIN_API_SLUG ? `/api/admin/${process.env.ADMIN_API_SLUG}/graphql` : '/api/admin/graphql'}`, {
        data: { query: 'mutation($e:String!,$p:String!){ adminLogin(input:{email:$e,password:$p}){ accessToken } }', variables: { e: email, p: password } },
      })
      .then((r) => r.json() as Promise<{ data?: { adminLogin?: { accessToken?: string } } }>)
  expect((await login(next)).data?.adminLogin?.accessToken).toBeTruthy()
  expect((await login(PASSWORD)).data?.adminLogin?.accessToken).toBeFalsy()
})
