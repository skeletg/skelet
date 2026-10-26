import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AssetGrid, Thumb } from './AssetGrid'

// What this pins: the grid moves like a grid - down is a whole row of however many columns fit, Home and End jump - the
// list moves a row at a time, there is one tab stop, clicks carry their modifiers, Space adds and Enter uses, and none
// of that reaches a tile that can't be picked

const asset = (id: string, over: object = {}) => ({
  __typename: 'MediaAsset' as const, id, path: `promo/${id}.png`, directoryPath: 'promo', filename: `${id}.png`, url: `https://cdn/${id}.png`,
  mimeType: 'image/png', mimeGroup: 'IMAGE' as const, sizeBytes: 2048, width: 40, height: 30, status: 'READY' as const,
  title: '', altText: '', caption: '', notes: '', focalX: null, focalY: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  ...over,
})
const six = ['a', 'b', 'c', 'd', 'e', 'f'].map((id) => asset(id))

function grid(over: Partial<Parameters<typeof AssetGrid>[0]> = {}) {
  const onFocus = vi.fn()
  const onToggle = vi.fn()
  const onUse = vi.fn()
  render(
    <AssetGrid
      assets={six} view="grid" size="large" selectedIds={new Set<string>()} focusedId={null} pickable={() => null}
      onFocus={onFocus} onToggle={onToggle} onUse={onUse} {...over}
    />,
  )
  return { onFocus, onToggle, onUse }
}

// jsdom lays nothing out, so say how many columns fit the way a browser's computed style would
const threeAcross = () => {
  screen.getByRole('listbox').style.gridTemplateColumns = '120px 120px 120px'
}
const tile = (name: string) => screen.getByRole('option', { name: new RegExp(`^${name}`) })

describe('AssetGrid', () => {
  it('moves down a whole row of the columns that fit', () => {
    const { onFocus } = grid()
    threeAcross()
    fireEvent.keyDown(tile('b'), { key: 'ArrowDown' })
    expect(document.activeElement).toBe(tile('e'))
    fireEvent.keyDown(tile('e'), { key: 'ArrowUp' })
    expect(document.activeElement).toBe(tile('b'))
    fireEvent.keyDown(tile('e'), { key: 'ArrowDown' })
    expect(document.activeElement).toBe(tile('f'))
    expect(onFocus).toHaveBeenLastCalledWith('f')
  })

  it('jumps with Home and End, and stops at the edges', () => {
    grid()
    fireEvent.keyDown(tile('c'), { key: 'End' })
    expect(document.activeElement).toBe(tile('f'))
    fireEvent.keyDown(tile('f'), { key: 'ArrowRight' })
    expect(document.activeElement).toBe(tile('f'))
    fireEvent.keyDown(tile('f'), { key: 'Home' })
    expect(document.activeElement).toBe(tile('a'))
    fireEvent.keyDown(tile('a'), { key: 'ArrowLeft' })
    expect(document.activeElement).toBe(tile('a'))
  })

  it('moves a row at a time in the list, whatever the columns say', () => {
    grid({ view: 'list' })
    threeAcross()
    fireEvent.keyDown(tile('a'), { key: 'ArrowDown' })
    expect(document.activeElement).toBe(tile('b'))
  })

  it('has one tab stop, on the focused tile or else the first', () => {
    grid({ focusedId: 'c' })
    expect(screen.getAllByRole('option').filter((o) => o.tabIndex === 0)).toEqual([tile('c')])
  })

  it('falls back to the first tile when the focused one is not on the page', () => {
    grid({ focusedId: 'gone' })
    expect(screen.getAllByRole('option').filter((o) => o.tabIndex === 0)).toEqual([tile('a')])
  })

  it('hands a click its modifiers, Space an additive pick and Enter a use', () => {
    const { onToggle, onUse } = grid()
    fireEvent.click(tile('a'))
    fireEvent.click(tile('b'), { shiftKey: true })
    fireEvent.click(tile('c'), { ctrlKey: true })
    fireEvent.click(tile('d'), { metaKey: true })
    expect(onToggle.mock.calls.map(([a, m]) => [a.id, m])).toEqual([
      ['a', { range: false, additive: false }],
      ['b', { range: true, additive: false }],
      ['c', { range: false, additive: true }],
      ['d', { range: false, additive: true }],
    ])
    fireEvent.keyDown(tile('e'), { key: ' ', shiftKey: true })
    expect(onToggle).toHaveBeenLastCalledWith(expect.objectContaining({ id: 'e' }), { range: true, additive: true })
    fireEvent.keyDown(tile('e'), { key: 'Enter' })
    fireEvent.doubleClick(tile('f'))
    expect(onUse.mock.calls.map(([a]) => a.id)).toEqual(['e', 'f'])
  })

  it("won't pick or use a tile that can't be picked, and says why", () => {
    const { onToggle, onUse } = grid({ pickable: (a) => (a.id === 'b' ? 'Still uploading' : null) })
    const b = tile('b')
    expect([b.getAttribute('aria-disabled'), b.title]).toEqual(['true', 'Still uploading'])
    fireEvent.click(b)
    fireEvent.doubleClick(b)
    fireEvent.keyDown(b, { key: 'Enter' })
    fireEvent.keyDown(b, { key: ' ' })
    expect(onToggle).not.toHaveBeenCalled()
    expect(onUse).not.toHaveBeenCalled()
  })

  it('marks what is picked, in both views', () => {
    grid({ selectedIds: new Set(['b']) })
    expect(screen.getAllByRole('option').map((o) => o.getAttribute('aria-selected'))).toEqual(['false', 'true', 'false', 'false', 'false', 'false'])
  })

  it('names a tile by its title and lists its folder, size and type', () => {
    grid({ view: 'list', assets: [asset('a', { title: 'Red mug', directoryPath: '' })] })
    const row = screen.getByRole('option')
    expect(within(row).getByText('Red mug')).toBeDefined()
    expect(within(row).getByText('Library · 40×30')).toBeDefined()
    expect(within(row).getByText('PNG')).toBeDefined()
    expect(within(row).getByText('2.0 KB')).toBeDefined()
  })
})

describe('Thumb', () => {
  it('shows the picture, or a placeholder while uploading or for a document', () => {
    const { rerender, container } = render(<Thumb asset={asset('a', { altText: 'A red mug' })} />)
    expect(screen.getByRole('img').getAttribute('alt')).toBe('A red mug')
    rerender(<Thumb asset={asset('a', { status: 'PENDING' })} />)
    expect(container.querySelector('img')).toBeNull()
    rerender(<Thumb asset={asset('a', { mimeGroup: 'DOCUMENT', mimeType: 'application/pdf' })} />)
    expect(container.querySelector('img')).toBeNull()
  })
})
