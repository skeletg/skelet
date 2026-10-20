import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SoftDeleteRowAction } from './SoftDeleteActions'

// What this pins: the row delete confirms through the shared ConfirmDialog - a refused delete keeps it open with
// the reason, a successful one closes it - and a deleted row offers Restore instead

function renderAction(onDelete: () => Promise<void>, deletedAt: string | null = null, onRestore = vi.fn(async () => {})) {
  render(<SoftDeleteRowAction deletedAt={deletedAt} entityLabel="flow" name="Welcome" consequence="Runs are kept." onDelete={onDelete} onRestore={onRestore} />)
  return onRestore
}

async function confirm() {
  fireEvent.click(screen.getByTitle('Delete flow'))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
  return dialog
}

describe('SoftDeleteRowAction', () => {
  it('keeps the dialog open and shows a refused delete', async () => {
    renderAction(() => Promise.reject(new Error('flow is running')))
    const dialog = await confirm()
    expect(await within(dialog).findByText('flow is running')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the delete goes through', async () => {
    renderAction(async () => {})
    await confirm()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })

  it('restores a deleted row without asking', async () => {
    const onRestore = renderAction(async () => {}, '2026-10-01T00:00:00Z')
    fireEvent.click(screen.getByRole('button', { name: 'Restore' }))
    await waitFor(() => expect(onRestore).toHaveBeenCalled())
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})
