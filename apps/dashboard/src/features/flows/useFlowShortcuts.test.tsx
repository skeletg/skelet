import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useFlowShortcuts } from './useFlowShortcuts'

// What this pins: save from anywhere, undo anywhere but a field, and Backspace in the panel never takes a step

function Page({ actions }: { actions: Parameters<typeof useFlowShortcuts>[0] }) {
  useFlowShortcuts(actions)
  return (
    <>
      <div data-flow-canvas>
        <button>step</button>
      </div>
      <input aria-label="name" />
      <button>panel</button>
    </>
  )
}

function setup() {
  const actions = { undo: vi.fn(), redo: vi.fn(), duplicate: vi.fn(), remove: vi.fn(), deselect: vi.fn(), save: vi.fn() }
  const view = render(<Page actions={actions} />)
  return { actions, step: view.getByText('step'), field: view.getByLabelText('name'), panel: view.getByText('panel') }
}

describe('flow builder shortcuts', () => {
  it('saves from a field, but leaves undo and delete to the field', () => {
    const { actions, field } = setup()
    fireEvent.keyDown(field, { key: 's', ctrlKey: true })
    fireEvent.keyDown(field, { key: 'z', ctrlKey: true })
    fireEvent.keyDown(field, { key: 'Backspace' })
    expect(actions.save).toHaveBeenCalledTimes(1)
    expect(actions.undo).not.toHaveBeenCalled()
    expect(actions.remove).not.toHaveBeenCalled()
  })

  it('undoes and redoes from the panel, but removes nothing from there', () => {
    const { actions, panel } = setup()
    fireEvent.keyDown(panel, { key: 'z', metaKey: true })
    fireEvent.keyDown(panel, { key: 'z', metaKey: true, shiftKey: true })
    fireEvent.keyDown(panel, { key: 'y', ctrlKey: true })
    fireEvent.keyDown(panel, { key: 'Backspace' })
    fireEvent.keyDown(panel, { key: 'd', ctrlKey: true })
    expect(actions.undo).toHaveBeenCalledTimes(1)
    expect(actions.redo).toHaveBeenCalledTimes(2)
    expect(actions.remove).not.toHaveBeenCalled()
    expect(actions.duplicate).not.toHaveBeenCalled()
  })

  it('acts on the selection while the canvas has the keys', () => {
    const { actions, step } = setup()
    fireEvent.keyDown(step, { key: 'Backspace' })
    fireEvent.keyDown(step, { key: 'Delete' })
    fireEvent.keyDown(step, { key: 'd', ctrlKey: true })
    fireEvent.keyDown(step, { key: 'Escape' })
    expect(actions.remove).toHaveBeenCalledTimes(2)
    expect(actions.duplicate).toHaveBeenCalledTimes(1)
    expect(actions.deselect).toHaveBeenCalledTimes(1)
  })
})
