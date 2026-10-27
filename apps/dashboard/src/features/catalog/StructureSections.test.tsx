import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderSection } from '@/test/page-harness'
import { BundleStructureSection } from './BundleStructureSection'
import { GroupedStructureSection } from './GroupedStructureSection'
import { KitStructureSection } from './KitStructureSection'

// What this pins: each structure section's remove confirm runs through the shared ConfirmDialog - confirming
// only stages the change, so the dialog closes at once and the header save picks it up

const widget = { id: 'p2', sku: 'W-1', name: 'Widget', kind: 'SIMPLE', defaultPrice: 500, defaultCurrencyCode: 'EUR' }

async function confirmIn(label: string, confirm: string) {
  fireEvent.click(await screen.findByRole('button', { name: label }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: confirm }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect((screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement).disabled).toBe(false)
}

describe('structure section remove confirms', () => {
  it('stages a grouped member removal', async () => {
    renderSection(<GroupedStructureSection canManage product={{ id: 'p1', groupedMembers: [{ id: 'gm1', member: widget, defaultQuantity: 1, sortOrder: 0 }] }} />)
    await confirmIn('Remove member', 'Stage remove')
  })

  it('stages a kit component removal', async () => {
    renderSection(<KitStructureSection canManage product={{ id: 'p1', kitComponents: [{ id: 'kc1', component: widget, quantity: 1, sortOrder: 0 }] }} />)
    await confirmIn('Remove component', 'Stage remove')
  })

  it('stages a bundle option delete and a selection removal', async () => {
    const selection = { id: 's1', product: widget, quantity: 1, isDefault: false, sortOrder: 0 }
    const option = { id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, title: 'Size', content: [{ locale: 'en', title: 'Size' }], selections: [selection] }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', bundleOptions: [option] }} />)
    await confirmIn('Remove selection', 'Stage remove')
    await confirmIn('Delete option', 'Stage delete')
  })
})
