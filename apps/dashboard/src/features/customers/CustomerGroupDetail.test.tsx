import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CustomerGroupDetailPage } from './CustomerGroupDetail'
import { CustomerGroupDocument, DeleteCustomerGroupDocument, UpdateCustomerGroupDocument } from './CustomerGroups.gql'
import { CustomersPagedDocument } from './Customers.gql'

// What these pin: a profile edit is reviewed and saved with an emptied description cleared, and an
// admin without a global customers.manage grant gets the page read-only

const group = {
  __typename: 'CustomerGroup' as const,
  id: 'g1',
  slug: 'wholesale',
  name: 'Wholesale',
  description: 'Trade buyers',
  isDefault: false,
  memberCount: 0,
  createdAt: '2026-01-01T00:00:00Z',
}

function renderGroup(me = meMock(), onUpdate = vi.fn(), extra: MockLink.MockedResponse[] = []) {
  renderPage({
    path: '/customers/groups/g1',
    adminId: 'admin-1',
    routes: { '/customers/groups/:id': <CustomerGroupDetailPage />, '/customers/groups': <p>Groups list</p> },
    mocks: [
      ...extra,
      me,
      { request: { query: CustomerGroupDocument, variables: { id: 'g1' } }, result: { data: { customerGroup: group } }, maxUsageCount: 3 },
      {
        request: { query: CustomersPagedDocument, variables: () => true },
        result: { data: { customersPaged: { __typename: 'CustomerPage', items: [], totalCount: 0 } } },
        maxUsageCount: 3,
      },
      {
        request: { query: UpdateCustomerGroupDocument, variables: captureInput(onUpdate) },
        result: { data: { updateCustomerGroup: { ...group, name: 'Trade', description: '' } } },
      },
    ],
  })
  return onUpdate
}

afterEach(() => localStorage.clear())

describe('CustomerGroupDetailPage', () => {
  it('reviews a profile edit and clears an emptied description', async () => {
    const onUpdate = renderGroup()
    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Trade' } })
    fireEvent.change(screen.getByLabelText('Description'), { target: { value: '  ' } })
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ name: 'Trade', description: null }), SLOW)
  })

  it('is read-only without a global customers.manage grant', async () => {
    renderGroup(meMock({ superuser: false, permissions: ['customers.view'] }))
    // Disabled through the page's fieldset, not the input's own attribute
    expect((await screen.findByLabelText('Name')).matches(':disabled')).toBe(true)
    expect(screen.queryByRole('button', { name: 'Save changes' })).toBeNull()
  })
})

// The delete confirm runs through the shared ConfirmDialog: a refused delete keeps it open with the server's
// message, a successful one goes back to the list
describe('CustomerGroupDetailPage delete', () => {
  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderGroup(meMock(), vi.fn(), [mock])
    // The button renders disabled until the grants load
    const trigger = await screen.findByRole('button', { name: 'Delete group' }, SLOW)
    await waitFor(() => expect((trigger as HTMLButtonElement).disabled).toBe(false), SLOW)
    fireEvent.click(trigger)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete group' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteCustomerGroupDocument, variables: { id: 'g1' } }, result: { errors: [{ message: 'group is the default' }] } })
    expect(await within(dialog).findByText('group is the default')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteCustomerGroupDocument, variables: { id: 'g1' } }, result: { data: { deleteCustomerGroup: true } } })
    expect(await screen.findByText('Groups list', {}, SLOW)).toBeDefined()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
