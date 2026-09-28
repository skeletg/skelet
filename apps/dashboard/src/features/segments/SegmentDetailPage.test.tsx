import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { SegmentDetailPage } from './SegmentDetailPage'
import { SegmentDetailDocument, SetSegmentRulesDocument, UpdateSegmentDocument } from './Segments.gql'

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
