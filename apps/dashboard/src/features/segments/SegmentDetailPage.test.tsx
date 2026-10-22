import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'

const { toastSuccess } = vi.hoisted(() => ({ toastSuccess: vi.fn() }))
vi.mock('sonner', () => ({ toast: { success: toastSuccess, error: vi.fn() } }))
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { SegmentDetailPage } from './SegmentDetailPage'
import { ClearSegmentMembershipDocument, SegmentDetailDocument, SetSegmentRulesDocument, UpdateSegmentDocument } from './Segments.gql'

// What these pin: settings and rules are reviewed together, each mutation goes only when its part
// changed, and switching dynamic membership off clears the rules

const segment = {
  __typename: 'CustomerSegment' as const,
  id: 's1',
  slug: 'vip',
  name: 'VIP',
  description: '',
  isActive: true,
  memberCount: 0,
  rules: { logic: 'and', conditions: [{ field: 'orders_count', op: 'gte', value: 5 }] },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  members: [],
}

function renderSegment(writes = { update: vi.fn(), rules: vi.fn() }) {
  renderPage({
    path: '/segments/s1',
    adminId: 'admin-1',
    routes: { '/segments/:id': <SegmentDetailPage /> },
    mocks: [
      { request: { query: SegmentDetailDocument, variables: { id: 's1' } }, result: { data: { customerSegment: segment } }, maxUsageCount: 5 },
      {
        request: { query: UpdateSegmentDocument, variables: captureInput(writes.update) },
        result: { data: { updateCustomerSegment: { __typename: 'CustomerSegment', id: 's1' } } },
      },
      {
        request: {
          query: SetSegmentRulesDocument,
          variables: (vars: Record<string, unknown>) => {
            writes.rules(vars)
            return true
          },
        },
        result: { data: { setSegmentRules: { __typename: 'CustomerSegment', id: 's1' } } },
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('SegmentDetailPage', () => {
  it('reviews a rename and the switch to manual membership, then writes both', async () => {
    const writes = renderSegment()
    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Big spenders' } })
    fireEvent.click(screen.getByRole('switch', { name: /Dynamic membership/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.rules).toHaveBeenCalledWith({ segmentId: 's1', rules: null }), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ id: 's1', name: 'Big spenders', slug: 'vip', description: '', isActive: true })
  })

  it('leaves the rules alone when only the settings changed', async () => {
    const writes = renderSegment()
    fireEvent.click(await screen.findByRole('switch', { name: /^Active/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.update).toHaveBeenCalledWith(expect.objectContaining({ isActive: false })), SLOW)
    expect(writes.rules).not.toHaveBeenCalled()
  })
})

// Clearing the membership confirms through the shared ConfirmDialog: a refused clear keeps it open with the server's
// message (it used to be a toast), a successful one closes it and still toasts
describe('SegmentDetailPage clear membership', () => {
  async function confirmClear(mock: MockLink.MockedResponse) {
    renderPage({
      path: '/segments/s1',
      adminId: 'admin-1',
      routes: { '/segments/:id': <SegmentDetailPage /> },
      mocks: [{ request: { query: SegmentDetailDocument, variables: { id: 's1' } }, result: { data: { customerSegment: { ...segment, memberCount: 2 } } }, maxUsageCount: 5 }, mock],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Clear all' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear membership' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused clear', async () => {
    const dialog = await confirmClear({ request: { query: ClearSegmentMembershipDocument, variables: { segmentId: 's1' } }, result: { errors: [{ message: 'segment is locked' }] } })
    expect(await within(dialog).findByText('segment is locked')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes and toasts once the clear goes through', async () => {
    await confirmClear({ request: { query: ClearSegmentMembershipDocument, variables: { segmentId: 's1' } }, result: { data: { clearSegmentMembership: 2 } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(toastSuccess).toHaveBeenCalledWith('Membership cleared')
  })
})
