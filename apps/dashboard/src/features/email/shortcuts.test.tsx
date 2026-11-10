import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { TabPanelContext } from '@/lib/tab-panel-context'
import { useCanvasShortcuts, type CanvasActions } from './shortcuts'

function actions(id: string): CanvasActions {
  return { sel: id, order: [id], select: vi.fn(), remove: vi.fn(), duplicate: vi.fn(), move: vi.fn(), undo: vi.fn(), redo: vi.fn() }
}

function Editor({ actions }: { actions: CanvasActions }) {
  useCanvasShortcuts(actions)
  return <div data-email-canvas data-testid={actions.sel} tabIndex={-1} />
}

function Tabs({ first, second, active }: { first: CanvasActions; second: CanvasActions; active: 'first' | 'second' }) {
  return (
    <>
      <TabPanelContext.Provider value={{ active: active === 'first', visible: active === 'first', id: 'first', paneId: 'pane' }}>
        <Editor actions={first} />
      </TabPanelContext.Provider>
      <TabPanelContext.Provider value={{ active: active === 'second', visible: active === 'second', id: 'second', paneId: 'pane' }}>
        <Editor actions={second} />
      </TabPanelContext.Provider>
    </>
  )
}

describe('email canvas shortcuts across workspace tabs', () => {
  it('deletes and undoes only in the active editor, including after switching tabs', () => {
    const first = actions('first-block'), second = actions('second-block')
    const { rerender } = render(<Tabs first={first} second={second} active="second" />)
    fireEvent.keyDown(screen.getByTestId('second-block'), { key: 'Delete' })
    fireEvent.keyDown(screen.getByTestId('second-block'), { key: 'z', ctrlKey: true })
    expect(second.remove).toHaveBeenCalledWith('second-block')
    expect(second.undo).toHaveBeenCalledOnce()
    expect(first.remove).not.toHaveBeenCalled()
    expect(first.undo).not.toHaveBeenCalled()

    rerender(<Tabs first={first} second={second} active="first" />)
    fireEvent.keyDown(screen.getByTestId('first-block'), { key: 'Delete' })
    fireEvent.keyDown(document.body, { key: 'z', ctrlKey: true })
    expect(first.remove).toHaveBeenCalledWith('first-block')
    expect(first.undo).toHaveBeenCalledOnce()
    expect(second.remove).toHaveBeenCalledOnce()
    expect(second.undo).toHaveBeenCalledOnce()
  })
})
