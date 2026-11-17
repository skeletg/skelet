import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { __resetDetailSectionsForTests, hasDetailSections, registerDetailSection } from './detail-section-registry'
import { PluginAside, PluginSections } from './detail-sections'

afterEach(() => __resetDetailSectionsForTests())

describe('plugin detail sections', () => {
  it('places each section where it asked, and carries side sections into the body on a narrow screen', () => {
    registerDetailSection({ id: 'acme:top', target: 'customer', placement: 'before', render: () => <p>top</p> })
    registerDetailSection({ id: 'acme:end', target: 'customer', render: () => <p>end</p> })
    registerDetailSection({
      id: 'acme:side',
      target: 'customer',
      placement: 'side',
      render: ({ placement }) => <p>side in {placement}</p>,
    })

    const { rerender } = render(<PluginSections target="customer" entity={{}} mode="edit" placement="before" />)
    expect(screen.getByText('top')).toBeTruthy()
    expect(screen.queryByText('end')).toBeNull()

    rerender(<PluginSections target="customer" entity={{}} mode="edit" />)
    expect(screen.getByText('end')).toBeTruthy()
    expect(screen.getByText('side in side').parentElement?.className).toBe('xl:hidden')

    rerender(<PluginSections target="customer" entity={{}} mode="edit" sideInBody />)
    expect(screen.getByText('side in side').parentElement?.className).toBe('')
    expect(hasDetailSections('customer', 'edit', 'side')).toBe(true)
  })

  it('draws a side panel only when a plugin has something for it', () => {
    const { container, rerender } = render(<PluginAside target="order" entity={{}} mode="edit" />)
    expect(container.innerHTML).toBe('')

    registerDetailSection({ id: 'acme:erp', target: 'order', placement: 'side', render: () => <p>erp</p> })
    rerender(<PluginAside target="order" entity={{ id: 'o1' }} mode="edit" />)
    expect(container.querySelector('aside')?.textContent).toBe('erp')
  })
})
