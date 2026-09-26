import { expect } from '@playwright/test'
import { dragTo, navigate, newTab, startClean, strips, tabCentre, test } from './helpers'

test.beforeEach(async ({ page }) => {
  await startClean(page)
})

test('New Tab reads as a label and sits next to the last tab', async ({ page }) => {
  const button = page.locator('header button:has-text("New Tab")')
  await expect(button).toHaveCount(1)

  const pills = page.locator('[role=tab]')
  const last = await pills.nth((await pills.count()) - 1).boundingBox()
  const btn = await button.boundingBox()
  // it used to drift to the far edge of the header, with an invisible drop strip in between
  expect(btn!.x - (last!.x + last!.width)).toBeLessThan(24)
})

test('navigating stays inside the current tab', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  expect(await strips(page)).toEqual([['All orders']])
  await navigate(page, '/customers', 'All customers')
  // the strip is not a history - the tab is repurposed, never added to
  expect(await strips(page)).toEqual([['All customers']])
})

test('a new tab opens on the landing page and takes focus', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  const [strip] = await strips(page)
  expect(strip).toHaveLength(2)
  expect(strip[0]).toBe('All orders')
})

test('closing a tab falls to its neighbour, and the last one resets rather than emptying', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  expect(await strips(page)).toEqual([['All orders', 'All customers']])

  await page.locator('[role=tab]:has-text("All customers") button[aria-label^="Close"]').click()
  expect(await strips(page)).toEqual([['All orders']])

  await page.locator('[role=tab]:has-text("All orders") button[aria-label^="Close"]').click()
  const [strip] = await strips(page)
  expect(strip).toHaveLength(1) // a workspace is never left with no tabs at all
})

test('dragging a pill along the strip reorders it', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  expect(await strips(page)).toEqual([['All orders', 'All customers']])

  const first = await tabCentre(page, 'All orders')
  await dragTo(page, await tabCentre(page, 'All customers'), { x: first.x - 40, y: first.y })
  expect(await strips(page)).toEqual([['All customers', 'All orders']])
})

test('a workspace survives a reload', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  await page.waitForTimeout(1500) // the preference write is debounced
  await page.reload({ waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
  expect(await strips(page)).toEqual([['All orders', 'All customers']])
})
