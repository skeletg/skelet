import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

// What this pins: /login doesn't load the signed-in shell - DashboardLayout pulls in the sidebar, the AI chat (and
// recharts with it), workspace tabs and the account menu's form stack, none of which a signed-out visitor needs

const loaded = vi.hoisted(() => ({ layout: false }))
vi.mock('@/components/layout/DashboardLayout', () => {
  loaded.layout = true
  return { DashboardLayout: () => null }
})

// jsdom has no matchMedia; the theme provider and the auth backdrop ask for the colour scheme and reduced motion
window.matchMedia ??= ((query: string) =>
  ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, onchange: null, dispatchEvent: () => false }) as MediaQueryList)

describe('App on /login', () => {
  it('renders the sign-in form without loading the dashboard layout', async () => {
    window.history.pushState({}, '', '/login')
    const { default: App } = await import('./App')
    render(<App />)
    expect(await screen.findByLabelText(/email/i, {}, { timeout: 5000 })).toBeTruthy()
    expect(loaded.layout).toBe(false)
  })
})
