import { PrefsContext } from '@/lib/admin-prefs'
import { useState } from 'react'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { RecommendationImage, RecommendationRows } from './RecommendationRows'
import type { RecommendationProduct } from './recommendation-products'

const product = (id: string, name: string): RecommendationProduct => ({ id, name, sku: name.toUpperCase(), thumbnail: null, displayImage: null })
const mug = product('p2', 'Mug'), cap = product('p3', 'Cap')

function Harness({ products = [mug, cap] }: { products?: RecommendationProduct[] }) {
  const [value, onChange] = useState(products.map((p) => p.id))
  return <RecommendationRows value={value} onChange={onChange} byId={new Map(products.map((p) => [p.id, p]))} label="Related" disabled={false} loading={false} />
}

function names() {
  return Array.from(screen.getByRole('table').querySelectorAll('tbody tr')).map((r) => r.querySelector('p')?.textContent)
}

describe('RecommendationRows', () => {
  it('shows every selection beyond the default table page size', () => {
    renderDialog(<PrefsContext.Provider value=""><Harness products={Array.from({ length: 40 }, (_, i) => product(String(i), `Product ${i}`))} /></PrefsContext.Provider>)
    expect(screen.getByRole('table').querySelectorAll('tbody tr')).toHaveLength(40)
    expect(screen.queryByLabelText('Next page')).toBeNull()
  })

  it('keeps focus on an arrow control after reordering', () => {
    renderDialog(<PrefsContext.Provider value=""><Harness products={[mug, cap, product("p4", "Shirt")]} /></PrefsContext.Provider>)
    const down = screen.getByRole('button', { name: 'Move Mug down in Related' })
    down.focus()
    fireEvent.click(down)
    expect(names()).toEqual(['Cap', 'Mug', 'Shirt'])
    expect(document.activeElement).toBe(down)
    expect(screen.getByRole('button', { name: 'Move Mug down in Related' })).toBe(down)
  })

  it('supports keyboard dragging by its handle', async () => {
    const rect = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const row = this.closest('tr')
      const index = row ? Array.from(row.parentElement!.children).indexOf(row) : 0
      return { x: 0, y: index * 80, top: index * 80, bottom: index * 80 + 80, left: 0, right: 300, width: 300, height: 80, toJSON: () => ({}) }
    })
    const height = vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(80)
    const width = vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(300)
    try {
      renderDialog(<PrefsContext.Provider value=""><Harness /></PrefsContext.Provider>)
      const handle = screen.getByRole('button', { name: 'Reorder Mug' })
      handle.focus()
      fireEvent.keyDown(handle, { key: ' ', code: 'Space' })
      await vi.waitFor(() => expect(handle.getAttribute('aria-pressed')).toBe('true'), SLOW)
      // KeyboardSensor attaches its document listener on the next event-loop turn.
      await act(() => new Promise<void>((resolve) => setTimeout(resolve, 0)))
      fireEvent.keyDown(handle, { key: 'ArrowDown', code: 'ArrowDown' })
      await vi.waitFor(() => expect(screen.getByRole('status').textContent).toContain('over Cap, position 2'), SLOW)
      fireEvent.keyDown(handle, { key: ' ', code: 'Space' })
      await vi.waitFor(() => expect(names()).toEqual(['Cap', 'Mug']), SLOW)
      expect(within(screen.getByRole('table')).getByRole('button', { name: 'Move Cap up in Related' })).toHaveProperty('disabled', true)
    } finally {
      rect.mockRestore()
      height.mockRestore()
      width.mockRestore()
    }
  })
})

describe('RecommendationImage', () => {
  it('uses display image when the thumbnail is not ready', () => {
    render(<RecommendationImage product={{ ...mug, thumbnail: { id: 'thumb', url: '/processing.png', altText: '', status: 'PENDING' }, displayImage: { id: 'display', url: '/mug.png', altText: 'Mug', status: 'READY' } }} />)
    expect(screen.getByRole('img').getAttribute('src')).toBe('/mug.png')
  })

  it('replaces broken images with a placeholder and recovers for a new URL', () => {
    const view = render(<RecommendationImage product={{ ...mug, thumbnail: { id: 'thumb', url: '/broken.png', altText: 'Mug', status: 'READY' } }} />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByLabelText('No product image')).toBeTruthy()
    view.rerender(<RecommendationImage product={{ ...mug, thumbnail: { id: 'thumb', url: '/working.png', altText: 'Mug', status: 'READY' } }} />)
    expect(screen.getByRole('img').getAttribute('src')).toBe('/working.png')
  })
})
