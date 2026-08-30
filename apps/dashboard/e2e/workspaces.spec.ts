import { expect, test, type Page } from '@playwright/test'
import { closeMenu, dragTabToEdge, menuAction, navigate, newTab, openWorkspaceMenu, paneCount, startClean, strips, workspaceName } from './helpers'

test.beforeEach(async ({ page }) => {
  await startClean(page)
})

async function nameIt(page: Page, name: string, confirmLabel: string) {
  await page.getByLabel('Workspace name').fill(name)
  await page.getByRole('button', { name: confirmLabel, exact: true }).click()
  await expect(page.getByLabel('Workspace name')).toHaveCount(0)
}

async function createWorkspace(page: Page, name: string) {
  await menuAction(page, 'New workspace')
  await nameIt(page, name, 'Create')
  await expect.poll(() => workspaceName(page)).toContain(name)
}

test('the switcher took the place of the merge button', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await dragTabToEdge(page, 'All orders', 'right')

  // Merge is a layout action, so it lives with the workspace it belongs to rather than in the header
  await expect(page.locator('header button:has-text("Merge")')).toHaveCount(0)
  await openWorkspaceMenu(page)
  await expect(page.locator('[role=menuitem]:has-text("Merge panes")')).toBeVisible()
  await page.locator('[role=menuitem]:has-text("Merge panes")').click()
  expect(await paneCount(page)).toBe(1)
})

test('a new workspace starts clean and the old one is still there to go back to', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  const before = await strips(page)

  await createWorkspace(page, 'Fulfilment')
  expect((await strips(page))[0]).toHaveLength(1)

  await menuAction(page, 'Workspace 1')
  await expect.poll(() => workspaceName(page)).toContain('Workspace 1')
  expect(await strips(page)).toEqual(before)
})

test('a workspace keeps its split layout across a switch', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  await dragTabToEdge(page, 'All orders', 'right')
  expect(await paneCount(page)).toBe(2)

  await createWorkspace(page, 'Flat')
  expect(await paneCount(page)).toBe(1)

  await menuAction(page, 'Workspace 1')
  await expect.poll(() => paneCount(page)).toBe(2)
})

test('renaming leaves the tabs alone', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  const before = await strips(page)
  await menuAction(page, 'Rename')
  await nameIt(page, 'Daily ops', 'Rename')
  await expect.poll(() => workspaceName(page)).toContain('Daily ops')
  expect(await strips(page)).toEqual(before)
})

test('a duplicate owns its tabs, so closing one leaves the original whole', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await navigate(page, '/customers', 'All customers')
  const before = await strips(page)

  await menuAction(page, 'Duplicate')
  await nameIt(page, 'Copy', 'Duplicate')
  await expect.poll(() => workspaceName(page)).toContain('Copy')
  expect(await strips(page)).toEqual(before)

  await page.locator('[role=tab]:has-text("All customers") button[aria-label^="Close"]').click()
  expect(await strips(page)).toEqual([['All orders']])

  await menuAction(page, 'Workspace 1')
  await expect.poll(() => workspaceName(page)).toContain('Workspace 1')
  expect(await strips(page)).toEqual(before)
})

test('the only workspace cannot be deleted, and deleting the current one moves you next door', async ({ page }) => {
  await openWorkspaceMenu(page)
  await expect(page.locator('[role=menuitem]:has-text("Delete workspace")')).toHaveCount(0)
  await closeMenu(page)

  await createWorkspace(page, 'Scratch')
  page.on('dialog', (d) => void d.accept())
  await menuAction(page, 'Delete workspace')
  await expect.poll(() => workspaceName(page)).toContain('Workspace 1')

  await openWorkspaceMenu(page)
  await expect(page.locator('[role=menuitem]:has-text("Scratch")')).toHaveCount(0)
})

test('workspaces and the one you were in both survive a reload', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await createWorkspace(page, 'Second')
  await page.waitForTimeout(1500) // the preference write is debounced

  await page.reload({ waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
  await expect.poll(() => workspaceName(page)).toContain('Second')
  await openWorkspaceMenu(page)
  await expect(page.locator('[role=menuitem]:has-text("Workspace 1")')).toBeVisible()
})
