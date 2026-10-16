import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { CartOfferFieldsFragment } from '../Carts.gql'
import { OfferActions } from './OfferActions'

vi.mock('@apollo/client/react', () => ({
  useMutation: () => [vi.fn(), { loading: false }],
}))

const offer = {
  state: 'SENT', sendCount: 1, link: 'https://shop.test/offer/credential',
} as CartOfferFieldsFragment

describe('offer credentials', () => {
  it('hides preview and copy actions from view-only admins even with cached link data', () => {
    render(<MemoryRouter><OfferActions cartId="cart" offer={offer} canEdit={false} hasLines isDirty={false} /></MemoryRouter>)
    expect(screen.queryByRole('button', { name: 'Preview email' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Copy link' })).toBeNull()
  })

  it('shows preview and copy to editors', () => {
    render(<MemoryRouter><OfferActions cartId="cart" offer={offer} canEdit hasLines isDirty={false} /></MemoryRouter>)
    expect(screen.getByRole('button', { name: 'Preview email' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Copy link' })).toBeTruthy()
  })
})
