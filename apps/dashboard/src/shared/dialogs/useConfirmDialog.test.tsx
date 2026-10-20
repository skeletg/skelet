import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useConfirmDialog } from './useConfirmDialog'

// What this pins: ask() is the window.confirm replacement - nothing runs until the admin confirms, Cancel runs nothing,
// and a failure keeps the dialog open with its message instead of vanishing

function Page({ onConfirm }: { onConfirm: () => void | Promise<void> }) {
  const [ask, dialog] = useConfirmDialog()
  return (
    <>
      <button onClick={() => ask({ title: 'Delete it?', confirmLabel: 'Delete', destructive: true, onConfirm })}>Remove</button>
      {dialog}
    </>
  )
}

describe('useConfirmDialog', () => {
  it('runs the action only once confirmed, then closes', async () => {
    const onConfirm = vi.fn()
    render(<Page onConfirm={onConfirm} />)
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    expect(onConfirm).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('runs nothing on Cancel', async () => {
    const onConfirm = vi.fn()
    render(<Page onConfirm={onConfirm} />)
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('names the dismiss button when told to', async () => {
    function Cancelable() {
      const [ask, dialog] = useConfirmDialog()
      return (
        <>
          <button onClick={() => ask({ title: 'Cancel send?', confirmLabel: 'Cancel send', cancelLabel: 'Keep', onConfirm: vi.fn() })}>Stop</button>
          {dialog}
        </>
      )
    }
    render(<Cancelable />)
    fireEvent.click(screen.getByRole('button', { name: 'Stop' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Keep' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })

  it('stays open and shows why when the action fails', async () => {
    render(<Page onConfirm={() => Promise.reject(new Error('Still in use'))} />)
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('Still in use')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBeDefined()
  })
})
