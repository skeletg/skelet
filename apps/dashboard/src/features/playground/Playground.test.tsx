import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderDialog } from '@/test/page-harness'
import { Playground } from './Playground'

const { resetSurfaceTabs } = vi.hoisted(() => ({ resetSurfaceTabs: vi.fn() }))
vi.mock('./surfaces', async (orig) => ({ ...(await orig<typeof import('./surfaces')>()), resetSurfaceTabs }))
// The IDE itself needs Monaco and workers jsdom doesn't have - the reset only has to remount it
vi.mock('graphiql', () => ({ GraphiQL: Object.assign(() => <div>IDE</div>, { Footer: () => null }) }))
vi.mock('graphiql/setup-workers/vite', () => ({}))
vi.mock('graphiql/style.css', () => ({}))

// What this pins: Reset tabs asks through the shared ConfirmDialog, clears only the current surface's tabs once
// confirmed, and Cancel clears nothing

async function openReset() {
  renderDialog(<Playground />, [meMock()])
  fireEvent.click(await screen.findByRole('button', { name: 'Reset tabs' }, SLOW))
  return screen.findByRole('alertdialog')
}

describe('Playground reset tabs', () => {
  it('clears the current surface once confirmed', async () => {
    const dialog = await openReset()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Reset tabs' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(resetSurfaceTabs).toHaveBeenCalledWith('admin')
  })

  it('clears nothing on Cancel', async () => {
    resetSurfaceTabs.mockClear()
    const dialog = await openReset()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(resetSurfaceTabs).not.toHaveBeenCalled()
  })
})
