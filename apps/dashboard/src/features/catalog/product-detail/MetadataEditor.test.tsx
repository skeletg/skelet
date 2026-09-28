import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { ProductMetadataEditor } from './MetadataEditor'

// What these pin: both lists go out together with blank rows dropped, and a key used twice keeps the
// dialog open instead of silently losing one of the values

function renderMetadata() {
  const onSave = vi.fn(async () => undefined)
  renderSection(
    <ProductMetadataEditor
      productId="p1"
      metadata={[{ key: 'carrier', value: 'dhl' }]}
      privateMetadata={[]}
      canManage
      onSave={onSave}
    />,
  )
  return onSave
}

async function apply(changes: RegExp) {
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: changes }))
  return dialog
}

describe('ProductMetadataEditor', () => {
  it('saves both lists, dropping blank rows', async () => {
    const onSave = renderMetadata()
    fireEvent.change(screen.getByLabelText('Metadata 1 value'), { target: { value: 'ups' } })
    const [, addPrivate] = screen.getAllByRole('button', { name: 'Add row' })
    fireEvent.click(addPrivate)
    fireEvent.click(addPrivate)
    fireEvent.change(screen.getByLabelText('Private metadata 1 key'), { target: { value: 'erp_id' } })
    fireEvent.change(screen.getByLabelText('Private metadata 1 value'), { target: { value: '42' } })
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(onSave).toHaveBeenCalled(), SLOW)
    expect(onSave).toHaveBeenCalledWith({ metadata: [{ key: 'carrier', value: 'ups' }], privateMetadata: [{ key: 'erp_id', value: '42' }] })
  })

  it('refuses a key used twice', async () => {
    const onSave = renderMetadata()
    fireEvent.click(screen.getAllByRole('button', { name: 'Add row' })[0])
    fireEvent.change(screen.getByLabelText('Metadata 2 key'), { target: { value: 'carrier' } })
    fireEvent.change(screen.getByLabelText('Metadata 2 value'), { target: { value: 'ups' } })
    const dialog = await apply(/Apply/)
    expect(await within(dialog).findByText(/"carrier" is used twice/)).toBeDefined()
    expect(onSave).not.toHaveBeenCalled()
  })
})
