import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, renderPage } from '@/test/page-harness'
import { draftKey, writeDraft } from '@/lib/form-drafts'
import { toAddressValues } from '@/shared/schemas/address'
import { ProductPickerDocument } from '@/features/catalog/Catalog.gql'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { DraftOrderCreate } from './DraftOrderCreate'
import { CreateDraftOrderDocument, PlaceDraftOrderDocument } from './Orders.gql'

// What these pin: nothing is sent without lines, an email and an address, and a refreshed page gets
// its work back from the stored draft - then "Save & place" creates the draft and places it

const channelScope = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'c1', locale: '' }, effectiveChannelId: 'c1', isChannel: true, isGlobal: false }

function renderDraft(writes = { create: vi.fn(), place: vi.fn() }) {
  renderPage({
    path: '/orders/new',
    adminId: 'admin-1',
    scope: channelScope,
    routes: { '/orders/new': <DraftOrderCreate />, '/orders/:id': <p>order page</p> },
    mocks: [
      { request: { query: ProductPickerDocument, variables: () => true }, result: { data: { productsPaged: { items: [] } } }, maxUsageCount: 10 },
      { request: { query: CustomersPagedDocument, variables: () => true }, result: { data: { customersPaged: { items: [], totalCount: 0 } } }, maxUsageCount: 10 },
      { request: { query: CountriesPickerDocument }, result: { data: { countries: [] } }, maxUsageCount: 5 },
      {
        request: { query: CreateDraftOrderDocument, variables: captureInput(writes.create) },
        result: { data: { createDraftOrder: { __typename: 'Order', id: 'o9' } } },
      },
      {
        request: {
          query: PlaceDraftOrderDocument,
          variables: (vars: { id: string }) => {
            writes.place(vars.id)
            return true
          },
        },
        result: { data: { placeDraftOrder: { __typename: 'Order', id: 'o9' } } },
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('DraftOrderCreate', () => {
  it('says what is missing and sends nothing', async () => {
    const writes = renderDraft()
    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }))
    expect(await screen.findByText('Add at least one product')).toBeDefined()
    expect(screen.getByText('Enter the order email')).toBeDefined()
    expect(screen.getByText('Street line 1 is required')).toBeDefined()
    expect(writes.create).not.toHaveBeenCalled()
  })

  it('restores a stored draft after a refresh, then saves and places it', async () => {
    const billing = { ...toAddressValues(), street1: '1 Main St', city: 'Berlin', postalCode: '10115', countryCode: 'DE' }
    writeDraft(draftKey('admin-1', null, 'draft-order:new:'), {
      email: { base: '', value: 'jane@example.com' },
      lines: { base: [], value: [{ sku: 'TEE-1', name: 'Tee', unitPrice: 1500, currency: 'EUR', quantity: 2 }] },
      billing: { base: toAddressValues(), value: billing },
    })
    const writes = renderDraft()

    // Offered, not applied: the form keeps its empty values until the admin restores
    const reviewButton = await screen.findByRole('button', { name: 'Review' })
    expect((screen.getByLabelText('Email') as HTMLInputElement).value).toBe('')
    fireEvent.click(reviewButton)
    const review = await screen.findByRole('alertdialog')
    expect(within(review).getByText('jane@example.com')).toBeDefined()
    fireEvent.click(within(review).getByRole('button', { name: 'Restore' }))
    await vi.waitFor(() => expect((screen.getByLabelText('Email') as HTMLInputElement).value).toBe('jane@example.com'))
    fireEvent.click(screen.getByRole('button', { name: 'Save & place' }))

    expect(await screen.findByText('order page', {}, SLOW)).toBeDefined()
    expect(writes.create).toHaveBeenCalledWith(
      expect.objectContaining({
        channelId: 'c1',
        email: 'jane@example.com',
        lines: [{ sku: 'TEE-1', quantity: 2 }],
        billingAddress: expect.objectContaining({ street1: '1 Main St', countryCode: 'DE' }),
        shippingAddress: expect.objectContaining({ street1: '1 Main St' }),
      }),
    )
    expect(writes.place).toHaveBeenCalledWith('o9')
  })
})
