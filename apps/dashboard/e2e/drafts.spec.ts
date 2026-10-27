import { expect, type Page } from '@playwright/test'
import { adminApi, login, saveThroughDialog, test } from './helpers'
import { RUN, cleanup, draftKeys, firstChannel, hasDraft, open, productTypeId, registerCleanups } from './fixtures'

// The stored draft's life, in a real browser: what a reload offers when someone else changed the same
// field meanwhile, two sections of one page each offering their own, an edit made instead of deciding,
// and the three exits - logging out, closing the workspace tab, and a sibling tab on the same record.
// The unit tests pin each rule against a fake store; these pin that the page wires them
//
// Needs the dev stack up (see README.md)

registerCleanups()

async function seedGroup(name: string) {
  const { createCustomerGroup } = await adminApi<{ createCustomerGroup: { id: string } }>(
    'mutation($i:CreateCustomerGroupInput!){ createCustomerGroup(input:$i){ id } }',
    { i: { slug: `${RUN}-${name.toLowerCase().replace(/\W+/g, '-')}`, name, description: 'Seeded description' } },
  )
  cleanup(() => adminApi('mutation($id:ID!){ deleteCustomerGroup(id:$id) }', { id: createCustomerGroup.id }))
  return createCustomerGroup.id
}

const readGroupName = async (id: string) =>
  (await adminApi<{ customerGroup: { name: string } }>('query($id:ID!){ customerGroup(id:$id){ name } }', { id })).customerGroup.name

/** Types, waits for the debounced draft write, reloads */
async function typeAndReload(page: Page, selector: string, value: string) {
  await page.locator(selector).fill(value)
  await expect.poll(() => hasDraft(page)).toBe(true)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle')
}

test('a field changed elsewhere meanwhile is flagged, and restoring keeps the admin value', async ({ page }) => {
  // The country page renders the per-field "Server now" note - most pages show the clash in Review only
  await adminApi('mutation($c:String!){ deleteCountry(code:$c) }', { c: 'XT' }).catch(() => undefined)
  await adminApi('mutation($i:CreateCountryInput!){ createCountry(input:$i){ code } }', { i: { code: 'XT', name: 'Clash land', isActive: false } })
  cleanup(() => adminApi('mutation($c:String!){ deleteCountry(code:$c) }', { c: 'XT' }))
  const readName = async () => (await adminApi<{ country: { name: string } }>('query($c:String!){ country(code:$c){ name } }', { c: 'XT' })).country.name

  await open(page, '/countries/XT')
  await expect(page.locator('#country-name')).toHaveValue('Clash land')
  await page.locator('#country-name').fill('Mine')
  await expect.poll(() => hasDraft(page)).toBe(true)

  // Someone else renames it in the meantime
  await adminApi('mutation($i:UpdateCountryInput!){ updateCountry(input:$i){ code } }', { i: { code: 'XT', name: 'Theirs' } })
  await page.reload({ waitUntil: 'domcontentloaded' })

  await expect(page.locator('#country-name')).toHaveValue('Theirs')
  await page.getByRole('button', { name: 'Review', exact: true }).click()
  const review = page.getByRole('alertdialog')
  await expect(review.getByText(/also changed by someone else/)).toBeVisible()
  await expect(review.getByText(/now Theirs/)).toBeVisible()
  await review.getByRole('button', { name: 'Restore' }).click()

  await expect(page.locator('#country-name')).toHaveValue('Mine')
  await expect(page.getByText('Server now: Theirs')).toBeVisible()
  await saveThroughDialog(page, ['Name'])
  expect(await readName()).toBe('Mine')
})

test('an edit made instead of deciding drops the offer and becomes the draft', async ({ page }) => {
  const id = await seedGroup('Moving on group')
  await open(page, `/customers/groups/${id}`)
  await typeAndReload(page, '#cg-name', 'First try')
  await expect(page.getByRole('button', { name: 'Review', exact: true })).toBeVisible()

  await page.locator('#cg-desc').fill('A different edit')
  await expect(page.getByRole('button', { name: 'Review', exact: true })).toBeHidden()
  // The stored draft is now the new edit, not the one on offer
  await expect
    .poll(async () => page.evaluate((keys) => keys.map((k) => localStorage.getItem(k) ?? '').join(), await draftKeys(page)))
    .toContain('A different edit')
  const stored = await page.evaluate((keys) => keys.map((k) => localStorage.getItem(k) ?? '').join(), await draftKeys(page))
  expect(stored).not.toContain('First try')
})

test('two sections of the product page each offer their own draft', async ({ page }) => {
  const channel = await firstChannel()
  const typeId = await productTypeId()
  const { createProduct } = await adminApi<{ createProduct: { id: string } }>('mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }', {
    i: { productTypeId: typeId, kind: 'SIMPLE', sku: `${RUN}-two-sections`, locale: 'en', name: 'Two sections', defaultPrice: 1500, defaultCurrencyCode: channel.currencyCode },
  })
  const id = createProduct.id
  cleanup(() => adminApi('mutation($id:ID!){ deleteProduct(id:$id) }', { id }))
  await adminApi('mutation($i:SetProductListingInput!){ setProductListing(input:$i){ id } }', { i: { productId: id, channelId: channel.id, isPublished: true, price: 1500, url: `${RUN}/two-sections` } })

  await open(page, `/products/${id}`)
  await page.locator('#pd-name').fill('Drafted name')
  await page.locator('button[class*="group/collapsible"]').filter({ hasText: /^#SEO/ }).click()
  await page.locator(`#churl-${channel.id}`).fill(`${RUN}/drafted-url`)
  await expect.poll(async () => (await draftKeys(page)).length).toBeGreaterThanOrEqual(2)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle')
  // The URL section's offer sits inside the collapsed SEO block
  await page.locator('button[class*="group/collapsible"]').filter({ hasText: /^#SEO/ }).click()

  const offers = page.getByRole('button', { name: 'Review', exact: true })
  await expect(offers).toHaveCount(2)
  // Restoring one leaves the other on offer
  await offers.first().click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Restore' }).click()
  await expect(offers).toHaveCount(1)
  await offers.first().click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Restore' }).click()
  await expect(offers).toHaveCount(0)

  await expect(page.locator('#pd-name')).toHaveValue('Drafted name')
  await expect(page.locator(`#churl-${channel.id}`)).toHaveValue(`${RUN}/drafted-url`)
})

test('closing the workspace tab asks first, and discarding drops the draft', async ({ page }) => {
  const id = await seedGroup('Closing group')
  await open(page, `/customers/groups/${id}`)
  await page.locator('#cg-name').fill('About to be closed')
  await expect.poll(() => hasDraft(page)).toBe(true)

  const prompts: string[] = []
  page.once('dialog', (d) => {
    prompts.push(d.message())
    void d.accept()
  })
  await page.locator('[role=tab]').filter({ hasText: 'Closing group' }).first().getByRole('button', { name: /^Close/ }).click()
  await expect.poll(() => prompts.length).toBe(1)
  expect(prompts[0]).toMatch(/unsaved changes/i)
  await expect.poll(() => hasDraft(page)).toBe(false)
  expect(await readGroupName(id)).toBe('Closing group')
})

test('a draft belongs to its workspace tab - a sibling tab on the same record starts clean', async ({ page }) => {
  const id = await seedGroup('Sibling group')
  await open(page, `/customers/groups/${id}`)
  await page.locator('#cg-name').fill('Typed in the first tab')
  await expect.poll(() => hasDraft(page)).toBe(true)

  await page.locator('button:has-text("New Tab")').first().click()
  await page.evaluate((path) => {
    history.pushState({}, '', path)
    dispatchEvent(new PopStateEvent('popstate'))
  }, `/customers/groups/${id}`)
  await page.waitForLoadState('networkidle')
  const visible = page.locator('#cg-name:visible')
  await expect(visible).toHaveValue('Sibling group')
  await expect(page.getByRole('button', { name: 'Review', exact: true })).toHaveCount(0)

  // The first tab still has its edit
  await page.locator('[role=tab]').first().click()
  await expect(page.locator('#cg-name:visible')).toHaveValue('Typed in the first tab')
})

test('logging out leaves no drafts behind', async ({ browser }) => {
  // Its own sign-in, so logging out can't touch the session the rest of the suite shares
  const context = await browser.newContext({ storageState: { cookies: [], origins: [] } })
  const page = await context.newPage()
  await login(page)
  const id = await seedGroup('Logout group')
  await page.goto(`/customers/groups/${id}`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle')
  await page.locator('#cg-name').fill('Typed before logging out')
  await expect.poll(() => hasDraft(page)).toBe(true)

  await page.getByRole('button', { name: /dev-super@skelet\.test|@/ }).last().click()
  await page.getByRole('menuitem', { name: 'Log out' }).click()
  await page.waitForURL(/\/login/)
  expect(await draftKeys(page)).toEqual([])
  await context.close()
})
