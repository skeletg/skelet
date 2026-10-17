import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FlatModeDormancyBanner } from './FlatModeDormancyBanner'
const mode = vi.hoisted(() => ({ value: 'MULTI_LOCATION' }))
vi.mock('@apollo/client/react', () => ({ useQuery: () => ({ data: { inventoryMode: mode.value } }) }))
describe('store availability in flat mode', () => {
  it('explains that the directory remains available while stock checking does not', () => {
    mode.value = 'FLAT'
    render(<FlatModeDormancyBanner />)
    expect(screen.getByText('Flat inventory mode is on')).toBeDefined()
    expect(screen.getByText(/per-store stock checking is unavailable/)).toBeDefined()
  })
  it('does not show a dormancy warning in multi-location mode', () => {
    mode.value = 'MULTI_LOCATION'
    render(<FlatModeDormancyBanner />)
    expect(screen.queryByText('Flat inventory mode is on')).toBeNull()
  })
})
