import { expect, type Browser, type Page } from '@playwright/test'
import { AUTH_FILE, adminApi, menuAction, navigate, newTab, openWorkspaceMenu, startClean, strips, test, workspaceName } from './helpers'
import { RUN, cleanup, firstChannel, productTypeId, registerCleanups } from './fixtures'

registerCleanups()

type Stored = { activeWorkspaceId: string; workspaces: { id: string; name: string; panes: { tabs: { title: string }[] }[] }[] }

async function seedProduct(): Promise<string> {
  const channel = await firstChannel()
  const { createProduct } = await adminApi<{ createProduct: { id: string } }>('mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }', {
    i: { productTypeId: await productTypeId(), kind: 'SIMPLE', sku: `${RUN}-roaming`, locale: 'en', name: 'Roaming product', defaultPrice: 1500, defaultCurrencyCode: channel.currencyCode },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteProduct(id:$id) }', { id: createProduct.id }))
  return createProduct.id
}

/** The roaming copy as the server holds it: the active workspace's name and its panes' tab titles */
async function stored(): Promise<{ active: string; strips: string[][] }> {
  const { myPreferences } = await adminApi<{ myPreferences: { key: string; value: Stored }[] }>('{ myPreferences { key value } }')
  const value = myPreferences.find((p) => p.key === 'workspaces')?.value
  const active = value?.workspaces.find((w) => w.id === value.activeWorkspaceId)
  return { active: active?.name ?? '', strips: active?.panes.map((p) => p.tabs.map((t) => t.title)) ?? [] }
}

/**
 * Waits for a machine's edit to reach the server. The write is debounced and a busy dev server can
 * take seconds to pass it on - a fixed wait lost that race, and the other machine then read the old
 * copy and wrote it back over the new one
 */
async function landed(page: Page) {
  const shown = await strips(page)
  const name = await workspaceName(page)
  await expect.poll(async () => {
    const s = await stored()
    return name.includes(s.active) && JSON.stringify(s.strips) === JSON.stringify(shown)
  }, { timeout: 30_000 }).toBe(true)
}

// Workspaces roam through the preferences store, so proving it needs two browser contexts that
// share the admin but nothing else - a second context is a second machine as far as the app knows
async function secondDevice(browser: Browser): Promise<Page> {
  const context = await browser.newContext({ storageState: AUTH_FILE, viewport: { width: 1500, height: 900 } })
  const page = await context.newPage()
  // the saved session carries the preference mirror, which is exactly what a new machine lacks
  await page.addInitScript(() => {
    if (sessionStorage.getItem('e2e:wiped')) return
    sessionStorage.setItem('e2e:wiped', '1')
    for (const key of Object.keys(localStorage)) if (key.endsWith(':workspaces')) localStorage.removeItem(key)
  })
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
  return page
}

test('the roaming copy is what a second machine opens into', async ({ page, browser }) => {
  await startClean(page)
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  await menuAction(page, 'New workspace')
  await page.getByLabel('Workspace name').fill('Roaming')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await expect.poll(() => workspaceName(page)).toContain('Roaming')
  await navigate(page, '/segments', 'Segments')
  await landed(page)

  const other = await secondDevice(browser)
  // it opens in the workspace that was active, on the page that was open in it
  await expect.poll(() => workspaceName(other)).toContain('Roaming')
  await expect(other).toHaveURL(/\/segments/)

  await openWorkspaceMenu(other)
  await expect(other.locator('[role=menuitem]:has-text("Workspace 1")')).toBeVisible()
  await other.locator('[role=menuitem]:has-text("Workspace 1")').click()
  await expect.poll(() => strips(other)).toEqual([['All orders', 'All customers']])
  await other.context().close()
})

test('a change made on one machine reaches the other', async ({ page, browser }) => {
  await startClean(page)
  await navigate(page, '/orders', 'All orders')
  await landed(page)

  const other = await secondDevice(browser)
  await expect.poll(() => strips(other)).toEqual([['All orders']])
  await newTab(other)
  const afterEdit = await strips(other)
  await landed(other)

  // the first machine has a stale mirror, and the roaming copy has to win over it
  await page.reload({ waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
  await expect.poll(() => strips(page)).toEqual(afterEdit)
  await other.context().close()
})

test('switching workspace keeps staged edits as a draft instead of asking', async ({ page }) => {
  await startClean(page)
  await menuAction(page, 'New workspace')
  await page.getByLabel('Workspace name').fill('Scratch')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await expect.poll(() => workspaceName(page)).toContain('Scratch')
  await menuAction(page, 'Workspace 1')
  await expect.poll(() => workspaceName(page)).toContain('Workspace 1')
  // The full load below reads the server's copy - it has to hold Scratch by then
  await landed(page)

  // a page that really stages edits - unlike a tab switch, leaving a workspace unmounts it
  await page.goto(`/products/${await seedProduct()}`, { waitUntil: 'domcontentloaded' })
  const name = page.locator('#pd-name')
  await expect(name).toBeVisible()
  await name.fill('E2E dirty edit')

  // The product page keeps its work as a stored draft, so leaving the workspace discards nothing and
  // doesn't ask - coming back offers the edit (see usePersistedForm)
  const prompts: string[] = []
  page.on('dialog', (d) => {
    prompts.push(d.message())
    void d.dismiss()
  })
  await expect.poll(() => page.evaluate(() => Object.keys(localStorage).some((k) => k.startsWith('skelet.draft.')))).toBe(true)
  await menuAction(page, 'Scratch')
  await expect.poll(() => workspaceName(page)).toContain('Scratch')
  expect(prompts).toEqual([])

  await menuAction(page, 'Workspace 1')
  await expect.poll(() => workspaceName(page)).toContain('Workspace 1')
  await page.getByRole('button', { name: 'Review' }).click()
  await expect(page.getByRole('alertdialog').getByText('E2E dirty edit')).toBeVisible()
})
