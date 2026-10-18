import { expect } from '@playwright/test'
import { dividers, dragTabToEdge, navigate, newTab, paneCount, startClean, test } from './helpers'

// Motion here is CSS, so what can silently break is the wiring: a renamed data-slot leaves the
// rules matching nothing and everything simply snaps, with no test failing anywhere else
const animation = (sel: string, page: import('@playwright/test').Page) =>
  page.locator(sel).first().evaluate((el) => {
    const s = getComputedStyle(el)
    return { name: s.animationName, duration: s.animationDuration, transition: s.transitionProperty }
  })

test.beforeEach(async ({ page }) => {
  await startClean(page)
})

test('a tab pill carries the enter animation', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  const tab = await animation('[data-slot="workspace-tab"]', page)
  expect(tab.name).toBe('workspace-tab-in')
  expect(tab.duration).not.toBe('0s')
})

test('a pane fades in and eases its focus border', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await dragTabToEdge(page, 'All orders', 'right')
  expect(await paneCount(page)).toBe(2)

  const pane = await animation('[data-slot="workspace-pane"]', page)
  expect(pane.name).toBe('workspace-pane-in')
  expect(pane.transition).toContain('border-color')
})

// A pane closing has no exit animation - its siblings growing into the space IS the close
test('pane slots ease their growth, and drop it while a divider is being dragged', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  await dragTabToEdge(page, 'All orders', 'right')

  const slot = await animation('[data-slot="workspace-pane-slot"]', page)
  expect(slot.transition).toContain('flex-grow')

  const divider = (await dividers(page)).find((d) => d.orientation === 'vertical')!
  await page.mouse.move(divider.x + divider.w / 2, divider.y + 300)
  await page.mouse.down()
  await page.mouse.move(divider.x - 120, divider.y + 300, { steps: 10 })
  await expect(page.locator('[data-slot="workspace-split"][data-resizing="true"]')).toHaveCount(1)
  const dragging = await animation('[data-slot="workspace-pane-slot"]', page)
  expect(dragging.transition === 'none' || dragging.transition === 'all').toBe(true)
  await page.mouse.up()

  await expect(page.locator('[data-slot="workspace-split"][data-resizing="true"]')).toHaveCount(0)
})

test('the app’s own reduce-motion preference collapses it', async ({ page }) => {
  await navigate(page, '/orders', 'All orders')
  await page.evaluate(() => document.documentElement.setAttribute('data-motion', 'reduced'))
  await newTab(page)
  const tab = await animation('[data-slot="workspace-tab"]', page)
  // collapsed rather than removed, so anything waiting on animationend still fires
  expect(tab.duration).toBe('0.001s')
})

test('the OS asking for less motion collapses it too', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await navigate(page, '/orders', 'All orders')
  await newTab(page)
  expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
  const tab = await animation('[data-slot="workspace-tab"]', page)
  expect(tab.duration).toBe('0.001s')
})
