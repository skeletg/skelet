import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { DetailPageBody, DetailPageLayout, DetailPageSection } from './detail-page'

// What these pin: the section index stays a column of ticks - each still named for screen readers - until the
// pointer or keyboard focus is on it, so its labels and their blur never spread over a button the admin is reaching
// for; leaving it folds it back

function Where() {
  const { pathname, hash } = useLocation()
  return <p data-testid="where">{pathname + hash}</p>
}

function page() {
  render(
    <MemoryRouter initialEntries={['/catalog/product-types/pt1']}>
      <Where />
      <DetailPageLayout>
        <DetailPageBody>
          <DetailPageSection title="General">x</DetailPageSection>
          <DetailPageSection title="Configurator templates" trailing={<button type="button">Use a template</button>}>y</DetailPageSection>
          <DetailPageSection title="Danger zone">z</DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    </MemoryRouter>,
  )
  const nav = screen.getByRole('navigation', { name: 'Sections', hidden: true })
  const label = (name: string) => within(nav).getByText(name)
  return { nav, label }
}

const folded = (nav: HTMLElement, label: (name: string) => HTMLElement) =>
  !nav.hasAttribute('data-open') && !nav.querySelector('[data-toc-backdrop]') && label('Configurator templates').classList.contains('sr-only')

describe('the section index', () => {
  it('shows only ticks, each still named, until pointed at', () => {
    const { nav, label } = page()
    expect(within(nav).getAllByRole('button', { hidden: true }).map((b) => b.textContent)).toEqual(['General', 'Configurator templates', 'Danger zone'])
    expect(folded(nav, label)).toBe(true)
  })

  it('spreads its labels over the page while the pointer is on it, and folds when it leaves', () => {
    const { nav, label } = page()
    fireEvent.mouseEnter(nav)
    expect(nav.hasAttribute('data-open')).toBe(true)
    expect(nav.querySelector('[data-toc-backdrop]')).not.toBeNull()
    expect(label('Danger zone').classList.contains('sr-only')).toBe(false)
    fireEvent.mouseLeave(nav)
    expect(folded(nav, label)).toBe(true)
  })

  it('opens for keyboard focus and folds once focus moves on', () => {
    const { nav, label } = page()
    const first = within(nav).getAllByRole('button', { hidden: true })[0]
    fireEvent.focus(first)
    expect(nav.hasAttribute('data-open')).toBe(true)
    fireEvent.blur(first, { relatedTarget: within(nav).getAllByRole('button', { hidden: true })[1] })
    expect(nav.hasAttribute('data-open')).toBe(true)
    fireEvent.blur(first, { relatedTarget: screen.getByRole('button', { name: 'Use a template' }) })
    expect(folded(nav, label)).toBe(true)
  })

  it('takes a tick to its section through the address, so the link can be shared', () => {
    const { nav } = page()
    fireEvent.click(within(nav).getByRole('button', { name: 'Danger zone', hidden: true }))
    expect(screen.getByTestId('where').textContent).toBe('/catalog/product-types/pt1#danger-zone')
  })
})
