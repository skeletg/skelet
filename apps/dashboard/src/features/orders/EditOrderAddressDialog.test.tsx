import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { EditOrderAddressDialog } from './EditOrderAddressDialog'
import { EditOrderAddressDocument, OrderDocument } from './Orders.gql'

const address = (firstName: string) => ({
  firstName,
  lastName: 'Doe',
  company: null,
  street1: '1 Main St',
  street2: null,
  city: 'Berlin',
  state: null,
  postalCode: '10115',
  countryCode: 'DE',
  phone: null,
})

// A stored shipping snapshot with a hole in it must not block a billing-only edit
const order = {
  id: 'o1',
  currencyCode: 'EUR',
  taxAmount: 100,
  shippingAmount: 500,
  total: 5600,
  shippingAddress: { ...address('Jane'), street1: '' },
  billingAddress: address('Jane'),
} as never

describe('EditOrderAddressDialog', () => {
  it('validates and sends only the side that changed', async () => {
    const onEdit = vi.fn()
    const onSaved = vi.fn()
    renderDialog(<EditOrderAddressDialog order={order} open onOpenChange={() => {}} onSaved={onSaved} />, [
      { request: { query: CountriesPickerDocument }, result: { data: { countries: [] } }, maxUsageCount: 3 },
      {
        request: { query: EditOrderAddressDocument, variables: captureInput(onEdit) },
        result: { data: { editOrderAddress: { __typename: 'Order', id: 'o1', taxAmount: 100, shippingAmount: 500, total: 5600 } } },
      },
      { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
    ])

    fireEvent.change(screen.getByLabelText('First name *', { selector: '#billing-firstName' }), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save addresses' }))
    expect(await screen.findByText('First name is required')).toBeDefined()
    expect(screen.queryByText('Street line 1 is required')).toBeNull()
    expect(onEdit).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('First name *', { selector: '#billing-firstName' }), { target: { value: 'Janet' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save addresses' }))
    await vi.waitFor(() => expect(onSaved).toHaveBeenCalledWith('Billing address updated - totals unchanged.'), SLOW)
    expect(onEdit).toHaveBeenCalledWith({
      orderId: 'o1',
      shippingAddress: null,
      billingAddress: expect.objectContaining({ firstName: 'Janet', company: null, countryCode: 'DE' }),
    })
  })
})
