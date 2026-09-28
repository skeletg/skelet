import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput } from '@/test/page-harness'
import { CustomerGroupFormDialog } from './CustomerGroupFormDialog'
import { CreateCustomerGroupDocument, CustomerGroupsDocument } from './CustomerGroups.gql'

describe('CustomerGroupFormDialog', () => {
  it('needs a name, then creates with an empty description cleared to null', async () => {
    const onCreate = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <MockedProvider
        mocks={[
          {
            request: { query: CreateCustomerGroupDocument, variables: captureInput(onCreate) },
            result: { data: { createCustomerGroup: { __typename: 'CustomerGroup', id: 'g1', slug: 'wholesale', name: 'Wholesale' } } },
          },
          { request: { query: CustomerGroupsDocument }, result: { data: { customerGroups: [] } }, maxUsageCount: 3 },
        ]}
      >
        <CustomerGroupFormDialog open onOpenChange={onOpenChange} />
      </MockedProvider>,
    )

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'wholesale' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create group' }))
    expect(await screen.findByText('Name is required')).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Wholesale' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create group' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onCreate).toHaveBeenCalledWith({ slug: 'wholesale', name: 'Wholesale', description: null })
  })
})
