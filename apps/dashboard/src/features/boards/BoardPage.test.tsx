import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { BoardDocument, SaveBoardDocument } from './Boards.gql'
import { BoardPage } from './BoardPage'

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

// As the query reads it: every field of the definition, unset ones null, each object with its __typename
const report = (title: string, event: string) => ({
  __typename: 'BoardReport',
  title,
  definition: {
    __typename: 'BoardReportDefinition',
    kind: 'STICKINESS',
    sinceDays: null,
    event,
    measure: null,
    unit: null,
    breakdown: null,
    steps: null,
    filters: null,
    windowHours: null,
    startEvent: null,
    returnEvent: null,
    periods: null,
    depth: null,
  },
})
const board = {
  __typename: 'Board',
  id: 'b1',
  name: 'Weekly',
  updatedAt: '2026-11-04T10:00:00.123456Z',
  reports: [report('Returning', 'page_view'), report('Buyers', 'order_placed')],
}

describe('BoardPage', () => {
  it('stages a rename and a removed report, and saves them with the board as read', async () => {
    let saved: Record<string, unknown> | null = null
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [
        meMock({ superuser: true }),
        { request: { query: BoardDocument, variables: { channelId: 'c1', id: 'b1' } }, result: { data: { board } }, maxUsageCount: 5 },
        {
          request: {
            query: SaveBoardDocument,
            variables: (v: Record<string, unknown>) => {
              saved = v
              return true
            },
          },
          result: { data: { saveBoard: { ...board, name: 'Weekly review', reports: [board.reports[1]] } } },
        },
      ],
    })
    const save = await screen.findByRole('button', { name: 'Save' }, SLOW)
    expect(save).toHaveProperty('disabled', true)

    fireEvent.change(screen.getByRole('textbox', { name: 'Board name' }), { target: { value: 'Weekly review' } })
    fireEvent.click(screen.getByRole('button', { name: 'Remove Returning' }))
    await waitFor(() => expect(save).toHaveProperty('disabled', false))
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(dialog).getByText('Name')).toBeTruthy()
    expect(within(dialog).getByText('Report · Returning')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply 2 changes' }))

    await waitFor(() => expect(saved).not.toBeNull(), SLOW)
    expect(saved).toMatchObject({
      channelId: 'c1',
      id: 'b1',
      name: 'Weekly review',
      readAt: '2026-11-04T10:00:00.123456Z',
    })
    // Sent back as the input takes it - no nulls, no __typename
    expect((saved as unknown as { reports: unknown[] }).reports).toEqual([{ title: 'Buyers', definition: { kind: 'STICKINESS', event: 'order_placed' } }])
  })
})
