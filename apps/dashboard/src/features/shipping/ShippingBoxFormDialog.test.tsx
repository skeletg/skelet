import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { ShippingBoxFormDialog } from './ShippingBoxFormDialog'
import { UpsertShippingBoxDocument } from './Shipping.gql'
import { boxSchema, toBoxInput, toBoxValues } from './schemas/box'

// Save -> mutate -> close is several async hops - past testing-library's 1s default when the whole
// suite is loading the machine
const SLOW = { timeout: 5000 }

// Radix's Switch measures itself inside a <form>, and jsdom has no ResizeObserver
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver

function renderDialog(onWrite = vi.fn(), onSaved = vi.fn(), onOpenChange = vi.fn()) {
  const mocks = [
    {
      request: {
        query: UpsertShippingBoxDocument,
        variables: (vars: { input: Record<string, unknown> }) => {
          onWrite(vars.input)
          return true
        },
      },
      result: { data: { upsertShippingBox: { __typename: 'ShippingBox', id: 'b1' } } },
    },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <ShippingBoxFormDialog open onOpenChange={onOpenChange} channelId="c1" onSaved={onSaved} />
    </MockedProvider>,
  )
  return { onWrite, onSaved, onOpenChange }
}

describe('box schema', () => {
  it('wants whole-millimetre dimensions and treats blank weights as 0', () => {
    const values = { ...toBoxValues(), code: 'small', length: '300', width: '200', height: '0' }
    expect(boxSchema.safeParse(values).error?.issues.map((i) => i.path.join('.'))).toEqual(['height'])
    expect(toBoxInput({ ...values, height: '100' })).toMatchObject({ innerHeightMm: 100, maxWeightGrams: 0, name: undefined })
  })
})

describe('ShippingBoxFormDialog', () => {
  it('stops an incomplete box, then creates it once filled in', async () => {
    const { onWrite, onSaved, onOpenChange } = renderDialog()
    fireEvent.change(screen.getByLabelText('Code'), { target: { value: 'small-A' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create box' }))
    expect(await screen.findAllByText('Whole millimetres, at least 1')).toHaveLength(3)
    expect(onWrite).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Length'), { target: { value: '300' } })
    fireEvent.change(screen.getByLabelText('Width'), { target: { value: '200' } })
    fireEvent.change(screen.getByLabelText('Height'), { target: { value: '100' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create box' }))

    await vi.waitFor(() => expect(onSaved).toHaveBeenCalled(), SLOW)
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(onWrite).toHaveBeenCalledWith(
      expect.objectContaining({ channelId: 'c1', code: 'small-A', innerLengthMm: 300, innerHeightMm: 100 }),
    )
  })
})
