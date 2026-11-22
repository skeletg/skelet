import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { LinePrice } from './LinePrice'

function setup(props: Partial<Parameters<typeof LinePrice>[0]> = {}) {
  const onCommit = vi.fn()
  const onReset = vi.fn()
  render(<LinePrice unitPrice={1500} reason="" override canReset onCommit={onCommit} onReset={onReset} {...props} />)
  return { onCommit, onReset }
}

describe('LinePrice', () => {
  it('sends a new price with the note beside it', () => {
    const { onCommit } = setup({ reason: 'Trade price' })
    const price = screen.getByLabelText('Unit price')
    fireEvent.change(price, { target: { value: '12.50' } })
    fireEvent.blur(price)
    expect(onCommit).toHaveBeenCalledWith(1250, 'Trade price')
  })

  it('sends a changed note with the price as it stands, and nothing when the note is the same', () => {
    const { onCommit } = setup({ reason: 'Trade price' })
    const note = screen.getByLabelText('Reason for the price')
    fireEvent.blur(note)
    expect(onCommit).not.toHaveBeenCalled()
    fireEvent.change(note, { target: { value: '  Matched a quote  ' } })
    fireEvent.blur(note)
    expect(onCommit).toHaveBeenCalledWith(1500, 'Matched a quote')
  })

  it('asks for no note on a catalog price, nor on a line that takes none', () => {
    setup({ override: false })
    expect(screen.queryByLabelText('Reason for the price')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reset' })).toBeNull()
  })

  it('takes a note on a set price that has no catalog price to go back to', () => {
    setup({ canReset: false })
    expect(screen.getByLabelText('Reason for the price')).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Reset' })).toBeNull()
  })

  it('leaves a custom line without a note field', () => {
    setup({ reason: null, canReset: false })
    expect(screen.queryByLabelText('Reason for the price')).toBeNull()
  })
})
