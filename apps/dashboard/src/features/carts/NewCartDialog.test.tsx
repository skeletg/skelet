import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderPage } from '@/test/page-harness'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { NewCartDialog } from './NewCartDialog'
import { AdminCreateCartDocument, AdminCreateCartOfferDocument, CartsDocument } from './Carts.gql'
import { newCartSchema } from './schemas/newCart'

describe('new cart schema', () => {
  it('asks for what the chosen mode needs', () => {
    const issues = (v: Parameters<typeof newCartSchema.safeParse>[0]) => newCartSchema.safeParse(v).error?.issues.map((i) => i.path.join('.'))
    const base = { customer: null, email: '', firstName: '', lastName: '', offer: false, guestEmail: '' }
    expect(issues({ ...base, mode: 'existing' })).toEqual(['customer'])
    expect(issues({ ...base, mode: 'new', email: 'x' })).toEqual(['email', 'firstName', 'lastName'])
    expect(issues({ ...base, mode: 'guest' })).toBeUndefined()
  })
})

describe('NewCartDialog', () => {
  it('creates a guest cart and opens it', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/carts',
      routes: {
        '/carts': <NewCartDialog open onOpenChange={() => {}} channelId="c1" />,
        '/carts/:id': <p>cart page</p>,
      },
      mocks: [
        { request: { query: CustomersPagedDocument, variables: () => true }, result: { data: { customersPaged: { items: [], totalCount: 0 } } }, maxUsageCount: 5 },
        {
          request: {
            query: AdminCreateCartDocument,
            variables: (vars: Record<string, unknown>) => {
              onCreate(vars)
              return true
            },
          },
          result: { data: { adminCreateCart: { __typename: 'Cart', id: 'cart-9' } } },
        },
        { request: { query: CartsDocument, variables: () => true }, result: { data: { carts: [] } }, maxUsageCount: 3 },
      ],
    })
    fireEvent.mouseDown(screen.getByRole('tab', { name: 'Guest' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Guest' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Create cart' }))
    expect(await screen.findByText('cart page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({ channelId: 'c1', customerId: null })
  })
})


describe('NewCartDialog - offers', () => {
  it('creates a guest offer and opens the Offers detail page', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/offers',
      routes: {
        '/offers': <NewCartDialog open offer onOpenChange={() => {}} channelId="c1" />,
        '/offers/:id': <p>offer page</p>,
      },
      mocks: [
        { request: { query: CustomersPagedDocument, variables: () => true }, result: { data: { customersPaged: { items: [], totalCount: 0 } } }, maxUsageCount: 5 },
        {
          request: {
            query: AdminCreateCartOfferDocument,
            variables: (vars: Record<string, unknown>) => { onCreate(vars); return true },
          },
          result: { data: { adminCreateCartOffer: { __typename: 'AdminCart', id: 'offer-9' } } },
        },
      ],
    })
    fireEvent.mouseDown(screen.getByRole('tab', { name: 'Guest' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Guest' }))
    fireEvent.change(await screen.findByLabelText('Send to'), { target: { value: 'guest@example.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create offer' }))
    expect(await screen.findByText('offer page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({ input: { channelId: 'c1', customerId: null, email: 'guest@example.com' } })
  })
})
