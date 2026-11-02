import { fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderDialog } from '@/test/page-harness'
import { CmsPreviewPanel } from './CmsPreviewPanel'

// What these pin: the editor always keeps its room - on a row too narrow for both, the preview starts as a
// rail and opens over the editor until Escape - and hiding it on a wide screen is remembered

const ROW_ATTR = 'data-test-row'

function renderInRow(rowWidth: number) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const width = this.hasAttribute(ROW_ATTR) ? rowWidth : 0
    return { width, height: 0, top: 0, left: 0, right: width, bottom: 0, x: 0, y: 0, toJSON: () => ({}) } as DOMRect
  })
  renderDialog(
    <div {...{ [ROW_ATTR]: '' }}>
      <CmsPreviewPanel pageId="pg1" locale="en" blocks={[]} />
    </div>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
})

describe('CmsPreviewPanel', () => {
  it('folds to a rail when the editor would lose its room, and opens over it until Escape', () => {
    renderInRow(700)
    expect(screen.queryByRole('button', { name: 'Hide preview' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Live preview' }))
    expect(screen.getByRole('button', { name: 'Hide preview' })).toBeDefined()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.queryByRole('button', { name: 'Hide preview' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Live preview' })).toBeDefined()
  })

  it('docks beside a roomy editor and remembers being hidden', () => {
    renderInRow(1400)
    fireEvent.click(screen.getByRole('button', { name: 'Hide preview' }))
    expect(screen.getByRole('button', { name: 'Live preview' })).toBeDefined()
    expect(localStorage.getItem('skelet:cms-preview:collapsed')).toBe('1')
    fireEvent.click(screen.getByRole('button', { name: 'Live preview' }))
    expect(screen.getByRole('button', { name: 'Hide preview' })).toBeDefined()
    expect(localStorage.getItem('skelet:cms-preview:collapsed')).toBe('0')
  })
})
