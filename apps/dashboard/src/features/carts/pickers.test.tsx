import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'

import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { CustomerSearch } from './pickers'

const ada = {
  __typename: 'Customer',
  id: 'c1',
  email: 'ada@example.test',
  firstName: 'Ada',
  lastName: 'Lovelace',
  phone: '',
  isActive: true,
  isEmailVerified: true,
  createdAt: new Date().toISOString(),
  group: null,
}

describe('CustomerSearch', () => {
  it('asks the server for the typed search, a page at a time, instead of loading every customer', async () => {
    const onPick = vi.fn()
    render(
      <MockedProvider
        mocks={[
          {
            request: { query: CustomersPagedDocument, variables: { search: 'ada', limit: 8, offset: 0 } },
            result: { data: { customersPaged: { __typename: 'CustomerPage', totalCount: 1, items: [ada] } } },
          },
        ]}
      >
        <CustomerSearch onPick={onPick} />
      </MockedProvider>,
    )
    fireEvent.change(screen.getByPlaceholderText(/search customers/i), { target: { value: 'ada' } })
    fireEvent.click(await screen.findByText('ada@example.test', {}, { timeout: 2000 }))
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: 'c1' }))
  })
})
