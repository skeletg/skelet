import { expect, type Page } from '@playwright/test'
import { adminApi, test } from './helpers'
import { RUN, cleanup, firstChannel, openScoped, registerCleanups } from './fixtures'

// The campaign composer and the flow builder where jsdom gives out: popovers, a date field, a drag onto the canvas,
// the keyboard, and the move to the saved record. Needs the dev stack up, Signals included (see README.md)

registerCleanups()

const del = (mutation: string, id: string) => () => adminApi(`mutation($id:ID!){ ${mutation}(id:$id) }`, { id })
const RECORD = /\/[0-9a-f-]{36}$/
const PLANNED = '2031-05-06T09:30'

// Playwright dismisses a native confirm on its own, which would refuse the navigation without a trace
function leavePrompts(page: Page): string[] {
  const seen: string[] = []
  page.on('dialog', (d) => {
    seen.push(d.message())
    void d.dismiss()
  })
  return seen
}

test('campaign - a draft keeps what its sentence says through a save and a reload', async ({ page }) => {
  const channel = await firstChannel()
  const { saveLibraryEmail: email } = await adminApi<{ saveLibraryEmail: { id: string } }>('mutation($i: SaveLibraryEmailInput!){ saveLibraryEmail(input:$i){ id } }', {
    i: { channelId: channel.id, name: `${RUN} email`, subject: `${RUN} subject`, blocks: [{ type: 'heading', text: 'Hello' }] },
  })
  cleanup(del('deleteLibraryEmail', email.id))
  const { campaignLimits: limits } = await adminApi<{ campaignLimits: { abTestPercentMin: number; abTestPercentMax: number } }>('{ campaignLimits { abTestPercentMin abTestPercentMax } }')
  const asked = leavePrompts(page)

  await openScoped(page, '/campaigns/new')
  await page.getByLabel('Campaign name').fill(`${RUN} campaign`)
  await page.getByRole('button', { name: 'Email: an email' }).click()
  await page.getByRole('option', { name: `${RUN} email` }).click()
  await expect(page.getByRole('button', { name: `Email: ${RUN} email` })).toBeVisible()
  await expect(page.getByText(`${RUN} subject`).first()).toBeVisible()

  await page.getByRole('button', { name: 'When: now' }).click()
  await page.getByRole('radio', { name: 'At a set time' }).click()
  await page.getByLabel('Date and time').fill(PLANNED)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: /^When: on .*2031/ })).toBeVisible()

  const subjectTest = page.getByRole('switch', { name: 'Test two subject lines first' })
  await subjectTest.click()
  await page.getByRole('button', { name: /^Share of the audience in the test/ }).click()
  const share = page.getByLabel('How much of the audience is in the test')
  await expect(share).toHaveAttribute('min', String(limits.abTestPercentMin))
  await expect(share).toHaveAttribute('max', String(limits.abTestPercentMax))
  await page.keyboard.press('Escape')
  await subjectTest.click()

  await page.getByRole('button', { name: 'Save draft' }).click()
  await page.waitForURL(RECORD)
  expect(asked).toEqual([])
  const id = page.url().split('/').pop()!
  cleanup(del('deleteEmailCampaign', id))

  const { emailCampaign: stored } = await adminApi<{ emailCampaign: { status: string; emailId: string; scheduledFor: string } }>('query($id:ID!){ emailCampaign(id:$id){ status emailId scheduledFor } }', { id })
  expect(stored.status).toBe('draft')
  expect(stored.emailId).toBe(email.id)
  expect(new Date(stored.scheduledFor).getTime()).toBe(await page.evaluate((at) => new Date(at).getTime(), PLANNED))

  await page.reload()
  await expect(page.getByRole('button', { name: /^When: on .*2031/ })).toBeVisible()
  await expect(page.getByRole('button', { name: `Email: ${RUN} email` })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Schedule' })).toBeEnabled()
})

test('flow - a recipe, a step dragged onto the canvas, the keyboard, and what the server will not switch on', async ({ page }) => {
  const asked = leavePrompts(page)
  await openScoped(page, '/flows/new')
  const canvas = page.locator('[data-flow-canvas]')
  const steps = canvas.getByText('Steps', { exact: true }).locator('xpath=following-sibling::span')
  const gallery = page.getByRole('region', { name: 'Start from a recipe' })

  await gallery.getByRole('button', { name: /^Abandoned cart/ }).click()
  await expect(page.getByLabel('Flow name')).toHaveValue('Abandoned cart')
  await expect(gallery).toBeHidden()
  await expect(page.getByRole('button', { name: 'Activate' })).toBeDisabled()
  await page.keyboard.press('Control+z')
  await expect(gallery).toBeVisible()
  await expect(steps).toHaveText('0')

  await page.getByRole('button', { name: 'Add Wait' }).dragTo(canvas, { targetPosition: { x: 320, y: 520 } })
  await expect(steps).toHaveText('1')
  await expect(page.getByLabel('Wait for')).toBeVisible()
  await canvas.locator('.react-flow__node-wait').click()
  await page.keyboard.press('Backspace')
  await expect(steps).toHaveText('0')
  await page.keyboard.press('Control+z')
  await expect(steps).toHaveText('1')

  await page.getByRole('tab', { name: 'Flow', exact: true }).click()
  await expect(page.getByText('Nothing follows the trigger, so nobody would be enrolled.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Activate' })).toBeDisabled()
  await page.getByLabel('Flow name').fill(`${RUN} flow`)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await page.waitForURL(RECORD)
  expect(asked).toEqual([])
  const id = page.url().split('/').pop()!
  cleanup(del('deleteFlow', id))

  const { flow } = await adminApi<{ flow: { status: string; graph: { nodes: { type: string }[] } } }>('query($id:ID!){ flow(id:$id){ status graph } }', { id })
  expect(flow.graph.nodes.map((n) => n.type).sort()).toEqual(['trigger', 'wait'])
  // The same refusal from the server, for a caller that never saw the button
  await expect(adminApi('mutation($id:ID!){ setFlowStatus(id:$id, status:"active"){ id } }', { id })).rejects.toThrow(/nothing follows the trigger/)
  expect((await adminApi<{ flow: { status: string } }>('query($id:ID!){ flow(id:$id){ status } }', { id })).flow.status).toBe(flow.status)
})
