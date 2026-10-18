import { expect } from '@playwright/test'
import { configField, configValue, restoreConfig, setConfigValue, startClean, test } from './helpers'

// A store-information string nothing computes from, so a run that dies mid-edit disturbs nothing - free text, so any
// value saves (the store's phone fields are validated as phone numbers now). Whatever it was is captured and put back
// afterwards, including having had no row at all
const PATH = 'general/store_information/description'
const BASELINE = 'E2E baseline description'
// config paths contain slashes, which an #id selector cannot carry
const INPUT = `[id="cfg-${PATH}"]`
const saveButton = 'main button:has-text("change"), main button:has-text("No changes")'

let original: Awaited<ReturnType<typeof configField>>

test.beforeEach(async ({ page }) => {
  original = await configField(PATH)
  // a known starting value, so the confirm dialog has a real before as well as an after
  await setConfigValue(PATH, BASELINE)
  await startClean(page)
  await page.goto('/settings', { waitUntil: 'domcontentloaded' })
  await expect(page.getByLabel('Filter settings')).toBeVisible()
  // the pane opens on the first section, but only once the config query answers
  await expect(page.locator('main input[id^="cfg-"]').first()).toBeVisible()
})

test.afterEach(async () => {
  await restoreConfig(original)
})

async function findField(page: import('@playwright/test').Page) {
  await page.getByLabel('Filter settings').fill(PATH)
  await expect(page.locator(INPUT)).toBeVisible()
}

test('an exact path filters down to the one field it names', async ({ page }) => {
  const opening = await page.locator('main input[id^="cfg-"]').count()
  expect(opening).toBeGreaterThan(1) // the pane opens on a whole section

  await findField(page)
  await expect.poll(() => page.locator('main :is(input, textarea)[id^="cfg-"]').count()).toBe(1)
})

test('the filter reaches across sections, not just the one on screen', async ({ page }) => {
  // the opening pane is one section; a query answers from all of them at once
  await page.getByLabel('Filter settings').fill('shipping')
  await expect(page.locator(INPUT)).toHaveCount(0)
  await page.getByLabel('Filter settings').fill('store description')
  await expect(page.locator(INPUT)).toBeVisible()
})

test('the filter searches values, not just labels', async ({ page }) => {
  // the reason values are in the haystack at all: searching "remote" missed the fields where it
  // was the VALUE rather than anything written on the row
  await page.getByLabel('Filter settings').fill('remote')
  // Any field kind - the match can be a select as well as a text input
  await expect(page.locator('main [id^="cfg-"]').first()).toBeVisible()
})

test('an edit arms the save button and the dialog spells the change out', async ({ page }) => {
  await findField(page)
  await expect(page.locator(saveButton)).toBeDisabled()

  await page.locator(INPUT).fill('E2E instructions')
  await expect(page.locator(saveButton)).toBeEnabled()
  await expect(page.locator(saveButton)).toHaveText(/Save 1 change/)

  await page.locator(saveButton).click()
  const dialog = page.getByRole('alertdialog')
  await expect(dialog).toContainText('Save 1 change?')
  await expect(dialog).toContainText('E2E instructions')
  await expect(dialog).toContainText(BASELINE)

  // cancelling backs out of the save, not out of the edit
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.locator(saveButton)).toHaveText(/Save 1 change/)
})

test('typing the original value back disarms the save button', async ({ page }) => {
  await findField(page)
  await page.locator(INPUT).fill('E2E instructions')
  await expect(page.locator(saveButton)).toBeEnabled()

  await page.locator(INPUT).fill(BASELINE)
  await expect(page.locator(saveButton)).toBeDisabled()
  await expect(page.locator(saveButton)).toHaveText(/No changes/)
})

test('applying the change persists it', async ({ page }) => {
  await findField(page)
  await page.locator(INPUT).fill('E2E persisted value')
  await page.locator(saveButton).click()
  await page.getByRole('alertdialog').getByRole('button', { name: /Apply 1 change/ }).click()

  await expect(page.getByRole('alertdialog')).toBeHidden()
  await expect(page.locator(saveButton)).toHaveText(/No changes/)
  await expect.poll(() => configValue(PATH), { timeout: 10_000 }).toBe('E2E persisted value')

  await page.reload({ waitUntil: 'domcontentloaded' })
  await findField(page)
  await expect(page.locator(INPUT)).toHaveValue('E2E persisted value')
})

// Saving twice is the case that can leave two rows behind for a GLOBAL setting, after which
// reads answer from whichever one the query plan reaches first
test('saving the same field twice keeps the second value', async ({ page }) => {
  await findField(page)
  for (const value of ['E2E first save', 'E2E second save']) {
    await page.locator(INPUT).fill(value)
    await page.locator(saveButton).click()
    await page.getByRole('alertdialog').getByRole('button', { name: /Apply 1 change/ }).click()
    await expect(page.getByRole('alertdialog')).toBeHidden()
  }

  await expect.poll(() => configValue(PATH), { timeout: 10_000 }).toBe('E2E second save')
  await page.reload({ waitUntil: 'domcontentloaded' })
  await findField(page)
  await expect(page.locator(INPUT)).toHaveValue('E2E second save')
})

test('leaving a page with staged settings asks first', async ({ page }) => {
  await findField(page)
  await page.locator(INPUT).fill('E2E unsaved')

  const prompts: string[] = []
  page.on('dialog', (d) => {
    prompts.push(d.message())
    void d.dismiss()
  })
  await page.keyboard.press('Control+k')
  const search = page.getByPlaceholder('Search pages and settings…')
  await expect(search).toBeVisible()
  await search.fill('All orders')
  await page.locator('[role=option]', { hasText: 'All orders' }).first().click()

  await expect.poll(() => prompts.length).toBeGreaterThan(0)
  expect(prompts[0]).toMatch(/unsaved changes/i)
  // declining kept the page, and the edit with it
  await expect(page.locator(INPUT)).toHaveValue('E2E unsaved')
})
