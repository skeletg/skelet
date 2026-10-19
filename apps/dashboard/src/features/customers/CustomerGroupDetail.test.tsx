import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CustomerGroupDetailPage } from './CustomerGroupDetail'
import { CustomerGroupDocument, UpdateCustomerGroupDocument } from './CustomerGroups.gql'
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

function renderGroup(me = meMock(), onUpdate = vi.fn()) {
  renderPage({
    path: '/customers/groups/g1',
    adminId: 'admin-1',
    routes: { '/customers/groups/:id': <CustomerGroupDetailPage /> },
    mocks: [
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

  it('opens the same review on ⌘S', async () => {
    renderGroup()
    await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Trade' } })
    fireEvent.keyDown(window, { key: 's', metaKey: true })
    expect(within(await screen.findByRole('alertdialog', {}, SLOW)).getByRole('button', { name: /Apply 1 change/ })).toBeDefined()
  })

  it('is read-only without a global customers.manage grant', async () => {
    renderGroup(meMock({ superuser: false, permissions: ['customers.view'] }))
    // Disabled through the page's fieldset, not the input's own attribute
    expect((await screen.findByLabelText('Name')).matches(':disabled')).toBe(true)
    expect(screen.queryByRole('button', { name: 'Save changes' })).toBeNull()
  })
})
