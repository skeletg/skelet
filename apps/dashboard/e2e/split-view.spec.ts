import { expect } from '@playwright/test'
import { dividers, dragTabToEdge, dragTo, navigate, newTab, paneCount, startClean, strips, tabCentre, test, workspaceBox } from './helpers'

test.beforeEach(async ({ page }) => {
  await startClean(page)
})

/** Two tabs in one pane, which every split below starts from */
async function twoTabs(page: import('@playwright/test').Page) {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
}

test('dragging a tab to the right edge splits the workspace', async ({ page }) => {
  await twoTabs(page)
  await dragTabToEdge(page, 'All orders', 'right')
  expect(await strips(page)).toEqual([['All customers'], ['All orders']])
  expect((await dividers(page)).some((d) => d.orientation === 'vertical')).toBe(true)
})

test('dragging to the bottom edge stacks the panes instead', async ({ page }) => {
  await twoTabs(page)
  await dragTabToEdge(page, 'All orders', 'bottom')
  expect(await paneCount(page)).toBe(2)
  expect((await dividers(page)).some((d) => d.orientation === 'horizontal')).toBe(true)
})

test('a pane will not split off its own only tab', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await dragTabToEdge(page, 'All orders', 'right')
  // it would empty the pane it came from and put it straight back - nothing to do
  expect(await paneCount(page)).toBe(1)
})

test('a drop in the outer gutter still counts as the edge', async ({ page }) => {
  await twoTabs(page)
  const area = await workspaceBox(page)
  // panes are inset from the shell, and the gutter must not be a dead zone
  await dragTo(page, await tabCentre(page, 'All orders'), { x: area.x + area.width - 2, y: area.y + area.height / 2 })
  expect(await paneCount(page)).toBe(2)
})

test('the page a tab carries into a new pane is never reloaded', async ({ page }) => {
  const ops: string[] = []
  page.on('request', (r) => {
    // ADMIN_API_SLUG moves the endpoint to /api/admin/<slug>/graphql
    if (!/\/api\/admin\/(.+\/)?graphql/.test(r.url())) return
    try {
      const body = JSON.parse(r.postData() ?? '{}') as { operationName?: string }
      if (body.operationName) ops.push(body.operationName)
    } catch { /* not a query we care about */ }
  })
  await twoTabs(page)
  const count = (name: string) => ops.filter((o) => o === name).length
  // The list queries once the header scope resolves, a beat after the tab opens
  await expect
    .poll(() => ops.find((o) => /order/i.test(o)), { message: 'the orders page should have issued at least one query' })
    .toBeTruthy()
  const watched = ops.find((o) => /order/i.test(o))

  const before = count(watched!)
  await dragTabToEdge(page, 'All orders', 'right')
  expect(await paneCount(page)).toBe(2)
  // the whole point of the stable mount node - a remount would refetch under the network-only policy
  expect(count(watched!) - before).toBe(0)
})

test('clicking a page inside a pane focuses that pane', async ({ page }) => {
  await twoTabs(page)
  await dragTabToEdge(page, 'All orders', 'right')
  await expect(page).toHaveURL(/\/orders/)

  // deliberately the page BODY, not the pane chrome - pages are portaled, so React's own event
  // path never reaches the pane and only a native listener sees this
  const cell = page.locator('table tbody tr td').first()
  await cell.click({ position: { x: 5, y: 5 } })
  await expect(page).toHaveURL(/\/customers/)
})

test('dropping on another pane strip moves the tab and heals the split', async ({ page }) => {
  await twoTabs(page)
  await dragTabToEdge(page, 'All orders', 'right')
  expect(await paneCount(page)).toBe(2)

  await dragTo(page, await tabCentre(page, 'All customers'), await tabCentre(page, 'All orders'))
  expect(await paneCount(page)).toBe(1)
  expect((await strips(page))[0]).toHaveLength(2)
})

test('the divider resizes the panes, and the sizes survive a reload', async ({ page }) => {
  await twoTabs(page)
  await dragTabToEdge(page, 'All orders', 'right')
  const before = (await dividers(page)).find((d) => d.orientation === 'vertical')!
  await dragTo(page, { x: before.x + before.w / 2, y: before.y + 300 }, { x: before.x - 260, y: before.y + 300 })

  const after = (await dividers(page)).find((d) => d.orientation === 'vertical')!
  expect(Math.abs(after.x - (before.x - 260))).toBeLessThan(40)

  await page.waitForTimeout(1500) // the preference write is debounced
  await page.reload({ waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
  const restored = (await dividers(page)).find((d) => d.orientation === 'vertical')!
  expect(Math.abs(restored.x - after.x)).toBeLessThan(12)
})

test('a split workspace comes back after a reload', async ({ page }) => {
  await twoTabs(page)
  await dragTabToEdge(page, 'All orders', 'right')
  await page.waitForTimeout(1500)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
  expect(await strips(page)).toEqual([['All customers'], ['All orders']])
})

test('New Tab moves into the panes once the header stops owning the strip', async ({ page }) => {
  await twoTabs(page)
  await expect(page.locator('header button:has-text("New Tab")')).toHaveCount(1)

  await dragTabToEdge(page, 'All orders', 'right')
  await expect(page.locator('header button:has-text("New Tab")')).toHaveCount(0)
  await expect(page.locator('button:has-text("New Tab")')).toHaveCount(2) // one per pane
  await expect(page.locator('header')).toContainText('2 panes')
})
