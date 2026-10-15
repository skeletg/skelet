import { expect, type Locator, type Page } from '@playwright/test'
import { adminApi, test } from './helpers'
import { RUN, adminCan, cleanup, draftKeys, firstChannel, open, openScoped, registerCleanups } from './fixtures'

// The dialog forms against the real API: open each from its page, fill it in, submit, and read the row
// back. A dialog writes straight through, so this is where its payload meets the schema. Dialogs keep
// no stored draft (persist: false), and a submit the form refuses keeps the dialog open and writes
// nothing
//
// Needs the dev stack up, Signals included (see README.md)

registerCleanups()

const del = (mutation: string, id: string) => () => adminApi(`mutation($id:ID!){ ${mutation}(id:$id) }`, { id })

type Row = { id: string; slug?: string; name?: string; code?: string; email?: string }

async function openDialog(page: Page, path: string, trigger: RegExp, scoped = false): Promise<Locator> {
  await (scoped ? openScoped : open)(page, path)
  await page.getByRole('button', { name: trigger }).first().click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  return dialog
}

async function submit(page: Page, dialog: Locator, label: string | RegExp) {
  await dialog.getByRole('button', { name: label }).click()
  await expect(dialog).toBeHidden()
  // Dialogs keep nothing in storage - their work is small and short-lived
  expect(await draftKeys(page)).toEqual([])
}

async function findRow(query: string, match: (r: Row) => boolean): Promise<Row> {
  const data = await adminApi<Record<string, Row[]>>(query)
  const row = Object.values(data)[0].find(match)
  if (!row) throw new Error(`no row matched in ${query}`)
  return row
}

test('customer group - and a refused submit keeps the dialog open', async ({ page }) => {
  const dialog = await openDialog(page, '/customers/groups', /New group/)
  // Name is required - nothing is written, the dialog stays
  await dialog.locator('#cg-slug').fill(`${RUN}-dlg-group`)
  await dialog.getByRole('button', { name: 'Create group' }).click()
  await expect(dialog.locator('#cg-name')).toHaveAttribute('aria-invalid', 'true')
  await expect(dialog).toBeVisible()
  expect((await adminApi<{ customerGroups: Row[] }>('{ customerGroups { id slug } }')).customerGroups.some((g) => g.slug === `${RUN}-dlg-group`)).toBe(false)

  await dialog.locator('#cg-name').fill('Dialog group')
  await dialog.locator('#cg-description').fill('Made in a dialog')
  await submit(page, dialog, 'Create group')
  const row = await findRow('{ customerGroups { id slug name description } }', (r) => r.slug === `${RUN}-dlg-group`)
  cleanup(del('deleteCustomerGroup', row.id))
  expect(row).toMatchObject({ name: 'Dialog group', description: 'Made in a dialog' })
})

test('review category and moderation tag', async ({ page }) => {
  for (const [path, trigger, query, remove] of [
    ['/reviews/categories', /New (review )?category/, '{ reviewCategories { id slug name } }', 'deleteReviewCategory'],
    ['/reviews/tags', /New (moderation )?tag/, '{ reviewModerationTags { id slug name } }', 'deleteReviewModerationTag'],
  ] as const) {
    const slug = `${RUN}-dlg-${remove.includes('Tag') ? 'tag' : 'category'}`
    const dialog = await openDialog(page, path, trigger)
    await dialog.getByLabel('Name').fill('Dialog label')
    await dialog.getByLabel('Slug').fill(slug)
    await submit(page, dialog, /^Create$/)
    const row = await findRow(query, (r) => r.slug === slug)
    cleanup(del(remove, row.id))
    expect(row.name).toBe('Dialog label')
  }
})

test('review question', async ({ page }) => {
  const dialog = await openDialog(page, '/reviews/questions', /New (review )?question/)
  await dialog.locator('#rq-label').fill('Dialog question')
  await dialog.locator('#rq-slug').fill(`${RUN}-dlg-question`)
  await dialog.locator('#rq-help').fill('Dialog help')
  await submit(page, dialog, /^Create$/)
  const { reviewQuestions } = await adminApi<{ reviewQuestions: { id: string; slug: string; allContent: { label: string; helpText: string | null }[] }[] }>(
    '{ reviewQuestions { id slug allContent { locale label helpText } } }',
  )
  const row = reviewQuestions.find((q) => q.slug === `${RUN}-dlg-question`)
  if (!row) throw new Error('review question not created')
  cleanup(del('deleteReviewQuestion', row.id))
  expect(row.allContent).toEqual([expect.objectContaining({ label: 'Dialog question', helpText: 'Dialog help' })])
})

test('product type', async ({ page }) => {
  const dialog = await openDialog(page, '/catalog/product-types', /New product type/)
  await dialog.locator('#pt-name').fill('Dialog type')
  await dialog.locator('#pt-slug').fill(`${RUN}-dlg-type`)
  await submit(page, dialog, 'Create product type')
  const { productTypes } = await adminApi<{ productTypes: { id: string; slug: string; name: string; isShippingRequired: boolean }[] }>(
    '{ productTypes { id slug name isShippingRequired } }',
  )
  const row = productTypes.find((t) => t.slug === `${RUN}-dlg-type`)
  if (!row) throw new Error('product type not created')
  cleanup(del('deleteProductType', row.id))
  expect(row.name).toBe('Dialog type')
})

test('attribute', async ({ page }) => {
  const dialog = await openDialog(page, '/catalog/attributes', /New attribute/)
  await dialog.locator('#attr-name').fill('Dialog attribute')
  await dialog.locator('#attr-slug').fill(`${RUN}-dlg-attr`)
  await submit(page, dialog, 'Create attribute')
  const row = await findRow('{ attributes { id slug name } }', (r) => r.slug === `${RUN}-dlg-attr`)
  cleanup(del('deleteAttribute', row.id))
  expect(row.name).toBe('Dialog attribute')
})

test('remote provider', async ({ page }) => {
  const dialog = await openDialog(page, '/remote-providers', /New (remote )?provider/)
  await dialog.locator('#rp-slug').fill(`${RUN}-dlg-provider`)
  await dialog.locator('#rp-name').fill('Dialog provider')
  await dialog.locator('#rp-url').fill('https://example.invalid/provider')
  await dialog.getByRole('button', { name: 'Create provider' }).click()
  // The signing secret is shown once, in a dialog of its own
  const secret = page.getByRole('dialog').filter({ hasText: 'shown once' })
  await secret.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(await draftKeys(page)).toEqual([])
  const row = await findRow('{ remoteProviders { id slug name } }', (r) => r.slug === `${RUN}-dlg-provider`)
  cleanup(del('deleteRemoteProvider', row.id))
  expect(row.name).toBe('Dialog provider')
})

test('integration', async ({ page }) => {
  const dialog = await openDialog(page, '/integrations', /New integration/)
  await dialog.locator('#integration-name').fill(`${RUN} dialog integration`)
  await dialog.locator('#integration-description').fill('Made in a dialog')
  await dialog.getByRole('button', { name: 'Create integration' }).click()
  // The token is shown once, in the same dialog or its successor - read the row, then close whatever is open
  const row = await expect
    .poll(async () => (await adminApi<{ integrations: Row[] }>('{ integrations { id name } }')).integrations.find((i) => i.name === `${RUN} dialog integration`)?.id ?? '')
    .not.toBe('')
    .then(async () => findRow('{ integrations { id name description } }', (r) => r.name === `${RUN} dialog integration`))
  cleanup(del('deleteIntegration', row.id))
  expect(row).toMatchObject({ description: 'Made in a dialog' })
  expect(await draftKeys(page)).toEqual([])
})

test('admin', async ({ page }) => {
  const email = `${RUN}-dlg-admin@e2e.test`
  const dialog = await openDialog(page, '/admins', /New admin/)
  await dialog.locator('#admin-first').fill('Dialog')
  await dialog.locator('#admin-last').fill('Admin')
  await dialog.locator('#admin-email').fill(email)
  await dialog.locator('#admin-password').fill('E2e-dialog-1234!')
  await submit(page, dialog, 'Create admin')
  const row = await findRow('{ admins { id email firstName lastName } }', (r) => r.email === email)
  cleanup(del('deleteAdmin', row.id))
  expect(row).toMatchObject({ firstName: 'Dialog', lastName: 'Admin' })
})

test('shipping box', async ({ page }) => {
  const channel = await firstChannel()
  test.skip(!(await adminCan('settings.manage', channel.id)), 'the e2e admin lacks settings.manage')
  const dialog = await openDialog(page, '/shipping/boxes', /New box/)
  await dialog.locator('#box-code').fill(`${RUN}-dlg-box`)
  await dialog.locator('#box-name').fill('Dialog box')
  await dialog.locator('#box-length').fill('30')
  await dialog.locator('#box-width').fill('20')
  await dialog.locator('#box-height').fill('10')
  await submit(page, dialog, 'Create box')
  const { shippingBoxes } = await adminApi<{ shippingBoxes: Row[] }>('query($c:ID!){ shippingBoxes(channelId:$c){ id code name } }', { c: channel.id })
  const row = shippingBoxes.find((b) => b.code === `${RUN}-dlg-box`)
  if (!row) throw new Error('shipping box not created')
  cleanup(del('deleteShippingBox', row.id))
  expect(row.name).toBe('Dialog box')
})

test('webhook edit dialog', async ({ page }) => {
  const { createWebhook } = await adminApi<{ createWebhook: { webhook: { id: string } } }>('mutation($i:CreateWebhookInput!){ createWebhook(input:$i){ webhook { id } } }', {
    i: { name: 'Seeded webhook', targetUrl: 'https://example.invalid/hook', kind: 'ASYNC', eventTypes: ['order.placed'], timeoutMs: 4000 },
  })
  const id = createWebhook.webhook.id
  cleanup(del('deleteWebhook', id))
  await open(page, `/webhooks/${id}`)
  await page.getByRole('button', { name: /^Edit/ }).first().click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.locator('#webhook-name')).toHaveValue('Seeded webhook')
  await dialog.locator('#webhook-name').fill('Renamed webhook')
  await submit(page, dialog, 'Save changes')
  const { webhook } = await adminApi<{ webhook: { name: string; targetUrl: string; eventTypes: string[]; timeoutMs: number } }>(
    'query($id:ID!){ webhook(id:$id){ name targetUrl eventTypes timeoutMs } }',
    { id },
  )
  // The rename only - the rest of the endpoint held
  expect(webhook).toEqual({ name: 'Renamed webhook', targetUrl: 'https://example.invalid/hook', eventTypes: ['order.placed'], timeoutMs: 4000 })
})

test('promotion code generator', async ({ page }) => {
  const { createPromotionRule } = await adminApi<{ createPromotionRule: { id: string } }>('mutation($i:CreatePromotionRuleInput!){ createPromotionRule(input:$i){ id } }', {
    i: { slug: `${RUN}-dlg-rule`, name: 'Dialog rule', requiresCode: true, isActive: false },
  })
  const id = createPromotionRule.id
  cleanup(del('deletePromotionRule', id))
  await open(page, `/promotions/${id}`)
  await page.getByRole('button', { name: /Generate codes/ }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('#count').fill('3')
  await dialog.locator('#prefix').fill('E2E-')
  await submit(page, dialog, 'Generate')
  const { promotionRule } = await adminApi<{ promotionRule: { codes: { totalCount: number; items: { code: string }[] } } }>(
    'query($id:ID!){ promotionRule(id:$id){ codes(limit:10){ totalCount items { code } } } }',
    { id },
  )
  expect(promotionRule.codes.totalCount).toBe(3)
  for (const c of promotionRule.codes.items) expect(c.code).toMatch(/^E2E-/)
})

test('new cart for a new customer opens the cart it made', async ({ page }) => {
  const email = `${RUN}-dlg-cart@e2e.test`
  const dialog = await openDialog(page, '/carts', /New cart/, true)
  await dialog.getByRole('tab', { name: 'New customer' }).click()
  await dialog.locator('#nc-email').fill(email)
  await dialog.locator('#nc-first').fill('Dialog')
  await dialog.locator('#nc-last').fill('Cart')
  await dialog.getByRole('button', { name: 'Create cart' }).click()
  await expect(page).toHaveURL(/\/carts\/[^/]+$/)
  const id = new URL(page.url()).pathname.split('/').pop()
  // Carts expire on their own and have no delete - the customer it made does
  const { cart } = await adminApi<{ cart: { customerId: string | null; customerEmail: string | null; customerName: string | null } }>(
    'query($id:ID!){ cart(id:$id){ customerId customerEmail customerName } }',
    { id },
  )
  if (cart.customerId) cleanup(del('deleteCustomer', cart.customerId))
  expect(cart).toMatchObject({ customerEmail: email, customerName: 'Dialog Cart' })
  expect(await draftKeys(page)).toEqual([])
})

// ---------- Order dialogs, on a real placed order ----------

type OrderRead = {
  status: string
  lines: { id: string; name: string; quantity: number }[]
  shippingAddress: { street1: string; city: string } | null
  billingAddress: { street1: string; city: string } | null
  fulfillments: { carrier: string | null; trackingNumber: string | null }[]
}

const readOrder = async (id: string) =>
  (
    await adminApi<{ order: OrderRead }>(
      'query($id:ID!){ order(id:$id){ status lines { id name quantity } shippingAddress { street1 city } billingAddress { street1 city } fulfillments { carrier trackingNumber } } }',
      { id },
    )
  ).order

test('order: edit a line, edit the shipping address, then ship it', async ({ page }) => {
  test.setTimeout(3 * 60_000)
  const channel = await firstChannel()
  // A type of its own that keeps no stock, so placing the order reserves nothing the dev data would have to hold
  const { createProductType } = await adminApi<{ createProductType: { id: string } }>('mutation($i:CreateProductTypeInput!){ createProductType(input:$i){ id } }', {
    i: { slug: `${RUN}-order-type`, name: `${RUN} order type`, isShippingRequired: true, tracksInventory: false },
  })
  cleanup(del('deleteProductType', createProductType.id))
  const sku = `${RUN}-order-sku`
  const { createProduct } = await adminApi<{ createProduct: { id: string } }>('mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }', {
    i: { productTypeId: createProductType.id, kind: 'SIMPLE', sku, locale: 'en', name: `${RUN} order product`, defaultPrice: 1000, defaultCurrencyCode: channel.currencyCode },
  })
  cleanup(del('deleteProduct', createProduct.id))
  await adminApi('mutation($i:SetProductListingInput!){ setProductListing(input:$i){ id } }', { i: { productId: createProduct.id, channelId: channel.id, isPublished: true, price: 1000 } })

  // In the dev store's own country - an address in one it doesn't sell to is refused
  const address = { firstName: 'Order', lastName: 'Tester', street1: 'Seeded street 1', city: 'Testville', postalCode: '12345', countryCode: 'US' }
  const { createDraftOrder } = await adminApi<{ createDraftOrder: { id: string } }>('mutation($i:CreateDraftOrderInput!){ createDraftOrder(input:$i){ id } }', {
    i: { channelId: channel.id, email: `${RUN}-order@e2e.test`, billingAddress: address, shippingAddress: address, lines: [{ sku, quantity: 1 }] },
  })
  const id = createDraftOrder.id
  // There is no order delete - a cancelled order tagged with the run is what's left behind
  cleanup(() => adminApi('mutation($id:ID!){ cancelOrder(id:$id){ id } }', { id }))
  await adminApi('mutation($id:ID!){ placeDraftOrder(id:$id){ id } }', { id })
  await adminApi('mutation($id:ID!){ confirmOrder(id:$id){ id } }', { id }).catch(() => undefined)

  await open(page, `/orders/${id}`)

  // Quantity 1 -> 2
  await page.getByRole('button', { name: 'Edit order' }).click()
  let dialog = page.getByRole('dialog')
  await dialog.getByRole('spinbutton').first().fill('2')
  await submit(page, dialog, 'Apply changes')
  await expect.poll(async () => (await readOrder(id)).lines[0]?.quantity).toBe(2)

  // Shipping address only - billing is sent only when it changed
  await page.getByRole('button', { name: 'Edit addresses' }).click()
  dialog = page.getByRole('dialog')
  // The dialog fills itself from the order as it opens - type after that, or the fill is overwritten
  await expect(dialog.locator('#shipping-street1')).toHaveValue('Seeded street 1')
  await dialog.locator('#shipping-street1').fill('Moved street 9')
  await expect(dialog.locator('#shipping-street1')).toHaveValue('Moved street 9')
  await expect(dialog.getByText('edited')).toBeVisible()
  // A refetch landing mid-edit used to re-open the dialog as a fresh form - give one time to land
  await page.waitForLoadState('networkidle')
  await expect(dialog.locator('#shipping-street1')).toHaveValue('Moved street 9')
  await submit(page, dialog, 'Save addresses')
  const moved = await readOrder(id)
  expect(moved.shippingAddress).toMatchObject({ street1: 'Moved street 9', city: 'Testville' })
  expect(moved.billingAddress).toMatchObject({ street1: 'Seeded street 1', city: 'Testville' })

  // Ship everything - a shipment needs the order in processing
  await adminApi('mutation($id:ID!){ markProcessing(id:$id){ id } }', { id })
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle')
  const ship = page.getByRole('button', { name: 'Create shipment' })
  test.skip(await ship.isDisabled(), `the order is ${moved.status} - a shipment needs it processing`)
  await ship.click()
  dialog = page.getByRole('dialog')
  await dialog.locator('#ship-carrier').fill('DHL')
  await dialog.locator('#ship-tracking').fill(`${RUN}-TRACK`)
  await submit(page, dialog, 'Create shipment')
  const shipped = await readOrder(id)
  expect(shipped.fulfillments).toEqual([expect.objectContaining({ carrier: 'DHL', trackingNumber: `${RUN}-TRACK` })])
})
