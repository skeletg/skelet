import type { DocumentNode } from 'graphql'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { SynapseInsightsDocument, SynapseOverviewDocument, SynapseStickinessDocument } from '@/features/synapse/Synapse.gql'
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

const report = (title: string, event: string) => ({ __typename: 'BoardReport', title, definition: { kind: 'stickiness', event } })
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
      reports: [{ title: 'Buyers', definition: { kind: 'stickiness', event: 'order_placed' } }],
    })
  })

  it('adds a report from the header, builds and sizes it in the panel beside the tile, and saves it with the board', async () => {
    let saved: Record<string, unknown> | null = null
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks((v) => (saved = v))],
    })
    fireEvent.pointerDown((await screen.findAllByRole('button', { name: 'Add report' }, SLOW))[0], { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /^Insights/ }))

    const panel = await screen.findByRole('complementary', { name: 'Edit Page view' }, SLOW)
    expect(screen.getByRole('region', { name: 'Page view' })).toBeTruthy() // the tile is on the grid while it's built
    fireEvent.click(within(panel).getByRole('button', { name: 'Unique' }))
    fireEvent.change(within(panel).getByRole('combobox', { name: 'Size' }), { target: { value: 'full' } })
    fireEvent.change(within(panel).getByRole('textbox', { name: 'Title' }), { target: { value: 'Visitors' } })

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const dialog = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(dialog).getByText('Report · Visitors')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply 1 change' }))
    await waitFor(() => expect(saved).not.toBeNull(), SLOW)
    expect((saved as unknown as { reports: unknown[] }).reports).toEqual([
      ...board.reports.map(({ title, definition }) => ({ title, definition })),
      { title: 'Visitors', definition: { kind: 'insights', event: 'page_view', measure: 'unique', unit: 'day', size: 'full' } },
    ])
  })

  it("stages a saved report's resize as just that, and Esc closes the panel", async () => {
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks(() => {})],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Buyers' }, SLOW))
    const panel = await screen.findByRole('complementary', { name: 'Edit Buyers' })
    fireEvent.change(within(panel).getByRole('combobox', { name: 'Size' }), { target: { value: 'small' } })
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('complementary')).toBeNull())

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const dialog = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(dialog).getByText('Report · Buyers · size')).toBeTruthy()
    expect(within(dialog).queryByText(/settings/)).toBeNull()
  })
})

function boardMocks(onSave: (v: Record<string, unknown>) => void) {
  const none = (query: DocumentNode) => ({ request: { query, variables: () => true }, error: new Error('not in this test'), maxUsageCount: 20 })
  return [
    { request: { query: BoardDocument, variables: { channelId: 'c1', id: 'b1' } }, result: { data: { board } }, maxUsageCount: 5 },
    none(SynapseOverviewDocument),
    none(SynapseInsightsDocument),
    none(SynapseStickinessDocument),
    {
      request: {
        query: SaveBoardDocument,
        variables: (v: Record<string, unknown>) => {
          onSave(v)
          return true
        },
      },
      result: { data: { saveBoard: board } },
    },
  ]
}
