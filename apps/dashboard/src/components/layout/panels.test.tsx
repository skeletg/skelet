import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { Empty, Panel, Tile } from './panels'

afterEach(cleanup)

describe('Tile', () => {
  it('renders the label, value and sub line', () => {
    render(<Tile label="Orders" value="1,204" sub="last 30 days" />)
    expect(screen.getByText('Orders')).toBeTruthy()
    expect(screen.getByText('1,204')).toBeTruthy()
    expect(screen.getByText('last 30 days')).toBeTruthy()
  })

  it('omits the sub line when there is none', () => {
    const { container } = render(<Tile label="Orders" value="1,204" />)
    expect(container.querySelectorAll('p')).toHaveLength(2)
  })

  it('holds the value with a skeleton while it loads, rather than showing a zero', () => {
    const { container } = render(<Tile label="Customers" value="0" loading />)
    expect(screen.queryByText('0')).toBeNull()
    expect(container.querySelector('[data-slot="skeleton"]')).toBeTruthy()
  })

  it('fades the value in once a loading tile resolves', () => {
    render(<Tile label="Customers" value="41" loading={false} />)
    expect(screen.getByText('41').hasAttribute('data-arrive')).toBe(true)
  })

  it('colours the value with the warning token for tone="warn"', () => {
    render(<Tile label="Zero results" value="18%" tone="warn" />)
    expect(screen.getByText('18%').className).toContain('text-warning')
  })

  it('leaves the value neutral by default', () => {
    render(<Tile label="Searches" value="900" />)
    expect(screen.getByText('900').className).not.toContain('text-warning')
  })
})

describe('Panel', () => {
  it('renders the title strip above its children', () => {
    render(
      <Panel title="Top pages">
        <span>row</span>
      </Panel>,
    )
    const title = screen.getByText('Top pages')
    const row = screen.getByText('row')
    expect(title.compareDocumentPosition(row) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})

describe('Empty', () => {
  it('renders its message', () => {
    render(<Empty>Nothing yet</Empty>)
    expect(screen.getByText('Nothing yet')).toBeTruthy()
  })
})
