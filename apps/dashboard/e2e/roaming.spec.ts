import { expect, test, type Browser, type Page } from '@playwright/test'
import { AUTH_FILE, menuAction, navigate, newTab, openWorkspaceMenu, startClean, strips, workspaceName } from './helpers'

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
  await page.waitForTimeout(2000) // the preference write is debounced, then has to reach the server

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
  await page.waitForTimeout(2000)

  const other = await secondDevice(browser)
  await expect.poll(() => strips(other)).toEqual([['All orders']])
  await newTab(other)
  const afterEdit = await strips(other)
  await other.waitForTimeout(2000)

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

  // a page that really stages edits - unlike a tab switch, leaving a workspace unmounts it
  await page.goto('/products', { waitUntil: 'domcontentloaded' })
  await page.locator('table tbody tr a').first().click()
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
