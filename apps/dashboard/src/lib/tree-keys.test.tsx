import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { onTreeItemKeyDown } from './tree-keys'

// What this pins: the category and media folder trees were mouse-only rows - these are the keys that now work them

function Tree({ onOpen, onToggle }: { onOpen: (id: string) => void; onToggle: (id: string) => void }) {
  const rows = [
    { id: 'a', expanded: false, hasChildren: true },
    { id: 'b', expanded: true, hasChildren: true },
    { id: 'c', expanded: false, hasChildren: false },
  ]
  return (
    <div role="tree">
      {rows.map((r) => (
        <div key={r.id} role="treeitem" tabIndex={0} aria-selected={false} onKeyDown={(e) => onTreeItemKeyDown(e, { ...r, onOpen: () => onOpen(r.id), onToggle: () => onToggle(r.id) })}>
          {r.id}
          <input aria-label={`rename ${r.id}`} />
        </div>
      ))}
    </div>
  )
}

describe('onTreeItemKeyDown', () => {
  it('opens, expands, collapses and moves focus', () => {
    const onOpen = vi.fn()
    const onToggle = vi.fn()
    render(<Tree onOpen={onOpen} onToggle={onToggle} />)
    const [a, b, c] = screen.getAllByRole('treeitem')

    fireEvent.keyDown(a, { key: 'Enter' })
    fireEvent.keyDown(a, { key: ' ' })
    expect(onOpen.mock.calls).toEqual([['a'], ['a']])

    fireEvent.keyDown(a, { key: 'ArrowRight' }) // collapsed parent expands
    fireEvent.keyDown(b, { key: 'ArrowLeft' }) // expanded parent collapses
    fireEvent.keyDown(a, { key: 'ArrowLeft' }) // already collapsed - nothing
    expect(onToggle.mock.calls).toEqual([['a'], ['b']])

    fireEvent.keyDown(a, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(b)
    fireEvent.keyDown(b, { key: 'ArrowRight' }) // already expanded - steps down instead
    expect(document.activeElement).toBe(c)
    fireEvent.keyDown(c, { key: 'Home' })
    expect(document.activeElement).toBe(a)
    fireEvent.keyDown(a, { key: 'End' })
    expect(document.activeElement).toBe(c)
    fireEvent.keyDown(c, { key: 'ArrowUp' })
    expect(document.activeElement).toBe(b)
  })

  it('leaves keys typed into a control inside the row alone', () => {
    const onOpen = vi.fn()
    render(<Tree onOpen={onOpen} onToggle={vi.fn()} />)
    fireEvent.keyDown(screen.getByLabelText('rename a'), { key: 'Enter' })
    expect(onOpen).not.toHaveBeenCalled()
  })
})
