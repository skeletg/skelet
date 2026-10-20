import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, useNavigate, type Location } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { usePanelLocation } from './panel-location'

// What these pin: a background workspace tab hands its page the SAME location object across navigations elsewhere -
// it used to rebuild it on every navigation, re-rendering every location consumer in every hidden tab

const frozen: Location = { pathname: '/orders', search: '', hash: '', state: null, key: 'tab-2' }
const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter initialEntries={['/products']}>{children}</MemoryRouter>

function usePanel(live: boolean) {
  return { value: usePanelLocation(live, false, frozen), navigate: useNavigate() }
}

describe('usePanelLocation', () => {
  it('keeps a background panel value stable while the router navigates', () => {
    const { result } = renderHook(() => usePanel(false), { wrapper })
    const first = result.current.value
    act(() => result.current.navigate('/customers'))
    act(() => result.current.navigate('/settings'))
    expect(result.current.value).toBe(first)
    expect(result.current.value.location).toBe(frozen)
  })

  it('follows the router while live', () => {
    const { result } = renderHook(() => usePanel(true), { wrapper })
    act(() => result.current.navigate('/customers'))
    expect(result.current.value.location.pathname).toBe('/customers')
  })
})
