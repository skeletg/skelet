import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { SelectionsCard } from './SelectionsCard'
import type { PageTypeAttr } from './shared'

// What these pin: a single select swaps its one value, and only the attributes that changed are written

const attr = (id: string, inputType: 'SELECT' | 'MULTISELECT', values: string[]) =>
  ({
    isRequired: false,
    attribute: {
      id,
      slug: id,
      inputType,
      isRequiredDefault: false,
      content: { name: id },
      values: values.map((v) => ({ id: `${id}-${v}`, slug: v })),
    },
  }) as unknown as PageTypeAttr

describe('SelectionsCard', () => {
  it('swaps a single select and writes only that attribute', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderSection(
      <SelectionsCard
        pageId="pg1"
        selectAttrs={[attr('layout', 'SELECT', ['wide', 'narrow']), attr('audience', 'MULTISELECT', ['b2b', 'b2c'])]}
        selections={[
          { attributeId: 'layout', valueId: 'layout-wide' },
          { attributeId: 'audience', valueId: 'audience-b2b' },
        ]}
        canManage
        onSave={onSave}
      />,
    )
    fireEvent.click(screen.getByLabelText('narrow'))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('wide')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith('layout', ['layout-narrow']), SLOW)
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
