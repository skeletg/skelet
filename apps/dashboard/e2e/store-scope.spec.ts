import { expect, type Browser, type Page } from '@playwright/test'
import { adminApi, spendFirstLogin, test } from './helpers'
import { RUN, cleanup, firstChannel, registerCleanups } from './fixtures'

// An admin whose roles reach one store only. Everything else in the suite runs as an admin with
// global grants, so nothing else would notice a page that shows another store's customers or
// orders, or a scope picker that offers a store the admin can't act in. The spec opens a store of
// its own, scopes a throwaway admin to it, and checks the Default Store's records stay out of reach
//
// Needs the dev stack up, with at least one order in the first store (see README.md)

registerCleanups()

const PASSWORD = 'E2e-scoped-1234!'
const email = `${RUN}-scoped@e2e.test`

type Seeds = { home: string; homeName: string; other: string; otherName: string; homeCustomer: string; otherCustomer: string; otherOrder: string | null }
let seeds: Seeds
let viewer: Page

test.beforeAll(async ({ browser }) => {
  const other = await firstChannel()
  const { createChannel } = await adminApi<{ createChannel: { id: string; name: string } }>('mutation($i:CreateChannelInput!){ createChannel(input:$i){ id name } }', {
    i: { slug: `${RUN}-scoped-store`, name: `${RUN} scoped store`, defaultLocale: 'en', locales: ['en'], currencyCode: other.currencyCode },
  })
  const home = createChannel.id
  // Registered first so it goes last - its customer has to go before it
  cleanup(() => adminApi('mutation($id:ID!){ deleteChannel(id:$id) }', { id: home }))

  const { createRole } = await adminApi<{ createRole: { id: string; slug: string } }>('mutation($i:CreateRoleInput!){ createRole(input:$i){ id slug } }', {
    i: {
      name: 'E2E one store',
      slug: `${RUN}-one-store`,
      scopes: ['customers.view', 'customers.manage', 'orders.view'].map((permissionSlug) => ({ permissionSlug, channelId: home })),
    },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteRole(id:$id) }', { id: createRole.id }))
  const { createAdmin } = await adminApi<{ createAdmin: { id: string } }>('mutation($i:CreateAdminInput!){ createAdmin(input:$i){ id } }', {
    i: { email, password: PASSWORD, firstName: 'One', lastName: 'Store', roleSlugs: [createRole.slug] },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteAdmin(id:$id) }', { id: createAdmin.id }))
  await spendFirstLogin(email, PASSWORD)

  const customer = async (channelId: string, who: string) => {
    const { adminCreateCustomer } = await adminApi<{ adminCreateCustomer: { id: string } }>('mutation($i:AdminCreateCustomerInput!){ adminCreateCustomer(input:$i){ id } }', {
      i: { channelId, email: `${RUN}-${who}@e2e.test`, password: 'E2e-password-1234!', firstName: who, lastName: 'Customer' },
    })
    cleanup(() => adminApi('mutation($id:ID!){ deleteCustomer(id:$id) }', { id: adminCreateCustomer.id }))
    return adminCreateCustomer.id
  }
  const { orders } = await adminApi<{ orders: { id: string }[] }>('query($c:ID){ orders(channelId:$c){ id } }', { c: other.id })
  seeds = {
    home,
    homeName: createChannel.name,
    other: other.id,
    otherName: other.name,
    homeCustomer: await customer(home, 'home'),
    otherCustomer: await customer(other.id, 'other'),
    otherOrder: orders[0]?.id ?? null,
  }
  viewer = await signIn(browser)
})

test.afterAll(async () => {
  await viewer?.context().close()
})

async function signIn(browser: Browser): Promise<Page> {
  const context = await browser.newContext({ storageState: { cookies: [], origins: [] } })
  const page = await context.newPage()
  await page.goto('/login', { waitUntil: 'domcontentloaded' })
  await page.fill('#email', email)
  await page.fill('#password', PASSWORD)
  await page.click('button[type=submit]')
  await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 45_000 })
  await page.waitForLoadState('networkidle')
  return page
}

async function view(path: string) {
  await viewer.goto(path, { waitUntil: 'domcontentloaded' })
  await viewer.waitForLoadState('networkidle')
}

test('its own store is the only one the scope picker offers', async () => {
  await view('/')
  await viewer.locator('[data-sidebar="header"] button, button:has-text("Admin panel")').first().click()
  await expect(viewer.getByText(seeds.homeName).first()).toBeVisible()
  await expect(viewer.getByText(seeds.otherName, { exact: true })).toHaveCount(0)
  await viewer.keyboard.press('Escape')
})

test('a customer of its own store opens', async () => {
  await view(`/customers/${seeds.homeCustomer}`)
  await expect(viewer.locator('#cd-email')).toHaveValue(`${RUN}-home@e2e.test`)
})

test("another store's customer stays out of reach", async () => {
  await view(`/customers/${seeds.otherCustomer}`)
  await expect(viewer.getByText("Not allowed for this customer's channel")).toBeVisible()
  await expect(viewer.getByText(`${RUN}-other@e2e.test`)).toHaveCount(0)
  await expect(viewer.locator('#cd-email')).toHaveCount(0)
})

test("the customer list holds its own store's customers only", async () => {
  await view('/customers')
  await viewer.getByPlaceholder(/search/i).first().fill(RUN)
  await expect(viewer.getByText(`${RUN}-home@e2e.test`)).toBeVisible()
  await expect(viewer.getByText(`${RUN}-other@e2e.test`)).toHaveCount(0)
})

test("another store's order stays out of reach, in the list and by its address", async () => {
  test.skip(!seeds.otherOrder, 'the first store has no order to test against')
  const { order } = await adminApi<{ order: { number: string } }>('query($id:ID!){ order(id:$id){ number } }', { id: seeds.otherOrder })

  await view('/orders')
  // The list rendered - an empty result proves nothing when the table never loaded
  await expect(viewer.getByRole('columnheader', { name: /Order/ }).first()).toBeVisible()
  await expect(viewer.getByText(order.number, { exact: true })).toHaveCount(0)
  // orders.view alone - nothing to create or export with, so no buttons that lead to "access restricted"
  await expect(viewer.getByRole('link', { name: /New order/ })).toHaveCount(0)
  await expect(viewer.getByRole('link', { name: /Export CSV/ })).toHaveCount(0)

  await view(`/orders/${seeds.otherOrder}`)
  await expect(viewer.getByText("Not allowed for this order's channel")).toBeVisible()
  await expect(viewer.getByText(order.number)).toHaveCount(0)
  await expect(viewer.getByRole('button', { name: 'Edit order' })).toHaveCount(0)
})
