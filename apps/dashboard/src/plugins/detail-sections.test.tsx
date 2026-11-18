import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { __resetDetailSectionsForTests, hasDetailSections, registerDetailSection } from './detail-section-registry'
import { PluginAside, PluginSections } from './detail-sections'

// The side panel shows from Tailwind's xl up
function screenIs(wide: boolean) {
  vi.stubGlobal('matchMedia', () => ({ matches: wide, addEventListener: () => {}, removeEventListener: () => {} }))
}

afterEach(() => {
  __resetDetailSectionsForTests()
  vi.unstubAllGlobals()
})

describe('plugin detail sections', () => {
  it('places each section where it asked, and mounts a side section once - in the panel when wide, the body when not', () => {
    registerDetailSection({ id: 'acme:top', target: 'customer', placement: 'before', render: () => <p>top</p> })
    registerDetailSection({ id: 'acme:end', target: 'customer', render: () => <p>end</p> })
    registerDetailSection({
      id: 'acme:side',
      target: 'customer',
      placement: 'side',
      render: ({ placement }) => <p>side in {placement}</p>,
    })
    const page = (sideInBody = false) => (
      <>
        <PluginSections target="customer" entity={{}} mode="edit" placement="before" />
        <main>
          <PluginSections target="customer" entity={{}} mode="edit" sideInBody={sideInBody} />
        </main>
        <PluginAside target="customer" entity={{}} mode="edit" />
      </>
    )

    screenIs(true)
    const wide = render(page())
    expect(screen.getByText('top')).toBeTruthy()
    expect(screen.getByText('end')).toBeTruthy()
    expect(screen.getAllByText('side in side')).toHaveLength(1)
    expect(wide.container.querySelector('aside')?.textContent).toBe('side in side')
    wide.unmount()

    screenIs(false)
    const narrow = render(page())
    expect(screen.getAllByText('side in side')).toHaveLength(1)
    expect(narrow.container.querySelector('aside')).toBeNull()
    expect(narrow.container.querySelector('main')?.textContent).toBe('endside in side')
    narrow.unmount()

    // A page whose side panel is something else keeps them in the body at any width
    screenIs(true)
    const preview = render(
      <main>
        <PluginSections target="customer" entity={{}} mode="edit" sideInBody />
      </main>,
    )
    expect(preview.container.querySelector('main')?.textContent).toBe('endside in side')
    expect(hasDetailSections('customer', 'edit', 'side')).toBe(true)
  })

  it('draws a side panel only when a plugin has something for it', () => {
    screenIs(true)
    const { container, rerender } = render(<PluginAside target="order" entity={{}} mode="edit" />)
    expect(container.innerHTML).toBe('')

    registerDetailSection({ id: 'acme:erp', target: 'order', placement: 'side', render: () => <p>erp</p> })
    rerender(<PluginAside target="order" entity={{ id: 'o1' }} mode="edit" />)
    expect(container.querySelector('aside')?.textContent).toBe('erp')
  })

  it('keeps a section that throws to itself', () => {
    screenIs(true)
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    registerDetailSection({
      id: 'acme:broken',
      target: 'customer',
      render: () => {
        throw new Error('undefined is not a function')
      },
    })
    registerDetailSection({ id: 'acme:fine', target: 'customer', render: () => <p>still here</p> })
    render(
      <>
        <p>the page</p>
        <PluginSections target="customer" entity={{}} mode="edit" />
      </>,
    )
    expect(screen.getByText('the page')).toBeTruthy()
    expect(screen.getByText('still here')).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain('acme:broken')
    quiet.mockRestore()
  })
})
