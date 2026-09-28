import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { ConfigurableVariantsCard } from './ConfigurableVariantsCard'

// What this pins: switching out of List mode and picking an option saves the mode first - the
// backend refuses options while the product is still List

const color = { id: 'a1', slug: 'color', name: 'Color', inputType: 'DROPDOWN' as const }

describe('ConfigurableVariantsCard', () => {
  it('saves the mode before the options', async () => {
    const calls: string[] = []
    const onSaveMode = vi.fn(async (mode: string) => calls.push(`mode:${mode}`))
    const onSaveOptions = vi.fn(async (ids: string[]) => calls.push(`options:${ids.join(',')}`))
    renderSection(
      <ConfigurableVariantsCard
        product={{ id: 'p1', variantMode: 'LIST', children: [], configurableOptions: [], productType: { id: 't1', attributes: [{ attribute: color }] } }}
        canManage
        onSaveMode={onSaveMode}
        onSaveOptions={onSaveOptions}
      />,
    )
    fireEvent.click(screen.getByRole('radio', { name: /Attribute-based/ }))
    fireEvent.click(await screen.findByRole('checkbox', { name: /Color/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(onSaveOptions).toHaveBeenCalled(), SLOW)
    expect(calls).toEqual(['mode:ATTRIBUTE', 'options:a1'])
  })
})
