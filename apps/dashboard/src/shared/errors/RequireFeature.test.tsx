import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RequireFeature } from './RequireFeature'

const { features } = vi.hoisted(() => ({ features: { on: [] as string[], loading: false, known: true } }))

vi.mock('@/shared/features/useFeatures', () => ({
  useFeatures: () => ({
    enabled: (f: string) => features.on.includes(f),
    loading: features.loading,
    known: features.known,
    error: features.known ? undefined : new Error('network down'),
    refetch: vi.fn(),
  }),
}))

function renderGate(feature: string) {
  return render(
    <MemoryRouter>
      <RequireFeature feature={feature}>
        <p>page</p>
      </RequireFeature>
    </MemoryRouter>,
  )
}

describe('RequireFeature', () => {
  beforeEach(() => {
    features.on = []
    features.loading = false
    features.known = true
  })
  afterEach(cleanup)

  it('renders the page while the switch is on', () => {
    features.on = ['signals']
    renderGate('signals')
    expect(screen.getByText('page')).toBeTruthy()
  })

  it('says the suite is off instead of mounting a page that can only fail', () => {
    renderGate('signals')
    expect(screen.queryByText('page')).toBeNull()
    expect(screen.getByText('Signals is switched off')).toBeTruthy()
    expect(screen.getByText('SIGNALS_ENABLED=true')).toBeTruthy()
  })

  it('renders nothing while the switches load, so the off screen cannot flash', () => {
    features.on = ['signals']
    features.loading = true
    const { container } = renderGate('signals')
    expect(container.textContent).toBe('')
  })

  it('inverts on a ! prefix', () => {
    renderGate('!signals')
    expect(screen.getByText('page')).toBeTruthy()
  })

  it('reports a failed switch fetch as a load error, not as the suite being off', () => {
    features.known = false
    renderGate('signals')
    expect(screen.queryByText('page')).toBeNull()
    expect(screen.queryByText('Signals is switched off')).toBeNull()
    expect(screen.getByText("Couldn't load the server's feature switches")).toBeTruthy()
  })
})
