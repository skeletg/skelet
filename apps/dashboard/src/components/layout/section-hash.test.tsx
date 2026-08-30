import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useState, type ReactNode } from 'react'
import { DetailPageLayout, DetailPageSection } from './detail-page'

vi.mock('@/lib/admin-prefs', () => ({
  useOptionalPreference: (_key: string, fallback: unknown) => fallback,
}))

vi.mock('@/nav/active-path', () => ({ useParentPage: () => null }))

// jsdom has no layout, so scrollTo is the only observable - record what each call asked for
const scrolled: Array<{ top: number; behavior?: ScrollBehavior }> = []

function Harness({ children, initialEntry }: { children: ReactNode; initialEntry: string }) {
  return (
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route
          path="/orders/:id"
          element={<DetailPageLayout>{children}</DetailPageLayout>}
        />
      </Routes>
    </MemoryRouter>
  )
}

function HashProbe() {
  const { hash } = useLocation()
  return <span data-testid="hash">{hash}</span>
}

beforeEach(() => {
  scrolled.length = 0
  // Every element reports the same box, so the delta is 0 and the assertions are about WHETHER a
  // scroll happened and how, not where it landed
  Element.prototype.getBoundingClientRect = vi.fn(() => new DOMRect(0, 0, 800, 600))
  Element.prototype.scrollTo = vi.fn((opts: ScrollToOptions) => {
    scrolled.push({ top: opts.top ?? 0, behavior: opts.behavior })
  }) as unknown as typeof Element.prototype.scrollTo
  // The layout's own scroller is found by computed overflow, which jsdom reports from the class-less
  // default - pin it so findScrollParent stops at the first candidate
  vi.spyOn(window, 'getComputedStyle').mockImplementation(
    () => ({ overflowY: 'auto' }) as CSSStyleDeclaration,
  )
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    cb(0)
    return 0
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('section deep links', () => {
  it('scrolls to the section named by the incoming hash', () => {
    render(
      <Harness initialEntry="/orders/1#payment">
        <DetailPageSection title="Summary">a</DetailPageSection>
        <DetailPageSection title="Payment">b</DetailPageSection>
      </Harness>,
    )
    expect(scrolled).toHaveLength(1)
    // Arriving already pointed at a section is a jump, not a journey
    expect(scrolled[0].behavior).toBe('auto')
  })

  it('leaves the page alone when there is no hash', () => {
    render(
      <Harness initialEntry="/orders/1">
        <DetailPageSection title="Summary">a</DetailPageSection>
        <DetailPageSection title="Payment">b</DetailPageSection>
      </Harness>,
    )
    expect(scrolled).toHaveLength(0)
  })

  it('ignores a hash that names no section', () => {
    render(
      <Harness initialEntry="/orders/1#nothing-here">
        <DetailPageSection title="Summary">a</DetailPageSection>
      </Harness>,
    )
    expect(scrolled).toHaveLength(0)
  })

  // The real failure mode this guards: a section behind a fetch isn't in the DOM on the first
  // render, and a one-shot effect would give up before it arrived
  it('waits for a section that mounts after its data loads', () => {
    function LateSection() {
      const [ready, setReady] = useState(false)
      return (
        <>
          <DetailPageSection title="Summary" trailing={
            <button onClick={() => setReady(true)}>load</button>
          }>
            a
          </DetailPageSection>
          {ready ? <DetailPageSection title="Payment">b</DetailPageSection> : null}
        </>
      )
    }
    render(
      <Harness initialEntry="/orders/1#payment">
        <LateSection />
      </Harness>,
    )
    expect(scrolled).toHaveLength(0)

    fireEvent.click(screen.getByText('load'))
    expect(scrolled).toHaveLength(1)
  })

  it('puts the section in the URL when its # is clicked, and scrolls smoothly', () => {
    render(
      <Harness initialEntry="/orders/1">
        <DetailPageSection title="Summary">a</DetailPageSection>
        <DetailPageSection title="Payment Method">b</DetailPageSection>
        <HashProbe />
      </Harness>,
    )
    fireEvent.click(screen.getByLabelText('Link to Payment Method'))

    expect(screen.getByTestId('hash').textContent).toBe('#payment-method')
    expect(scrolled).toHaveLength(1)
    // Moving within a page you are already reading keeps its place - that needs motion
    expect(scrolled[0].behavior).toBe('smooth')
  })

  // Layout components are deliberately renderable without a Router (same reason useParentPage reads
  // the context) - a page's own unit test shouldn't have to grow a MemoryRouter to mount
  it('renders outside a Router', () => {
    expect(() =>
      render(
        <DetailPageLayout>
          <DetailPageSection title="Summary">a</DetailPageSection>
        </DetailPageLayout>,
      ),
    ).not.toThrow()
    expect(scrolled).toHaveLength(0)
  })

  it('scrolls again when the section already in the URL is picked a second time', () => {
    render(
      <Harness initialEntry="/orders/1">
        <DetailPageSection title="Summary">a</DetailPageSection>
        <DetailPageSection title="Payment">b</DetailPageSection>
      </Harness>,
    )
    const anchor = screen.getByLabelText('Link to Payment')
    fireEvent.click(anchor)
    fireEvent.click(anchor)
    expect(scrolled).toHaveLength(2)
  })
})
