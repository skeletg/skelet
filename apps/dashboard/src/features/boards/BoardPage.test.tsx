import type { DocumentNode } from 'graphql'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { SynapseFlowDocument, SynapseFunnelDocument, SynapseInsightsDocument, SynapseOverviewDocument, SynapseStickinessDocument } from '@/features/synapse/Synapse.gql'
import { BoardDocument, DiscardBoardDraftDocument, SaveBoardDocument, SaveBoardDraftDocument } from './Boards.gql'
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
  status: 'published',
  access: 'own',
  favourite: false,
  owner: { __typename: 'BoardOwner', id: 'admin-1', name: 'Ada', removed: false },
  shares: [],
  draft: null,
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
        ...draftWrites(() => {}),
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

  it('keeps unsaved edits about a second after the last one, against the board as loaded', async () => {
    const kept: Record<string, unknown>[] = []
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks(() => {}, board, (v) => kept.push(v))],
    })
    fireEvent.change(await screen.findByRole('textbox', { name: 'Board name' }, SLOW), { target: { value: 'Weekly review' } })
    expect(kept).toEqual([]) // nothing is written mid-typing
    await waitFor(() => expect(kept).toHaveLength(1), { timeout: 3000 })
    expect(kept[0]).toMatchObject({ channelId: 'c1', id: 'b1', baseUpdatedAt: board.updatedAt, payload: { name: 'Weekly review' } })
  })

  it('opens on the unsaved edits kept from last time, and saves them against the board they started from', async () => {
    let saved: Record<string, unknown> | null = null
    const withDraft = {
      ...board,
      updatedAt: '2026-11-05T09:00:00Z',
      draft: {
        __typename: 'BoardDraft',
        payload: { name: 'Weekly, kept', reports: [{ ...board.reports[1], key: 's1' }] },
        baseUpdatedAt: '2026-11-04T10:00:00.123456Z',
        updatedAt: '2026-11-04T18:00:00Z',
      },
    }
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks((v) => (saved = v), withDraft)],
    })
    expect(await screen.findByText(/These are your unsaved changes from/, {}, SLOW)).toBeTruthy()
    expect((screen.getByRole('textbox', { name: 'Board name' }) as HTMLInputElement).value).toBe('Weekly, kept')
    expect(screen.queryByRole('region', { name: 'Returning' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const dialog = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(dialog).getByText('Report · Returning')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: /^Apply/ }))
    await waitFor(() => expect(saved).not.toBeNull(), SLOW)
    expect(saved).toMatchObject({ name: 'Weekly, kept', readAt: '2026-11-04T10:00:00.123456Z' }) // a newer save in between is refused, not overwritten
  })

  it('shows someone with view access the board without anything to change', async () => {
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-2',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks(() => {}, { ...board, access: 'view' })],
    })
    expect(await screen.findByRole('region', { name: 'Returning' }, SLOW)).toBeTruthy()
    expect(screen.getByText("Ada's board. You can view it.")).toBeTruthy()
    for (const name of ['Save', 'Add report', 'Delete', 'Share', 'Edit Returning']) expect(screen.queryByRole('button', { name })).toBeNull()
  })

  it("tells someone without customer analytics what's missing instead of running the reports", async () => {
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-2',
      scope: channelScope,
      mocks: [meMock({ superuser: false, permissions: ['boards.view'] }), ...boardMocks(() => {}, { ...board, access: 'view' })],
    })
    expect(await screen.findAllByText("You need customer analytics on Main to see this report's numbers.", {}, SLOW)).toHaveLength(2)
  })

  it('says a new board is a draft, and its first Save publishes it', async () => {
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks(() => {}, { ...board, status: 'draft' })],
    })
    expect(await screen.findByText('A draft only you can see - Save to keep it.', {}, SLOW)).toBeTruthy()
    const save = screen.getByRole('button', { name: 'Save' })
    await waitFor(() => expect(save).toHaveProperty('disabled', false))
    expect(screen.queryByRole('button', { name: 'Share' })).toBeNull() // a draft can't be shared yet
    fireEvent.click(save)
    expect(within(await screen.findByRole('alertdialog', {}, SLOW)).getByText('Publish')).toBeTruthy()
  })

  it("puts the people who dropped off a funnel step on the board as a flow, beside the funnel", async () => {
    const funnel = { __typename: 'BoardReport', title: 'Product to cart', definition: { kind: 'funnel', steps: ['view_item', 'cart_add'], size: 'full' } }
    const counts = { __typename: 'SynapseFunnel', breakdown: [], steps: [{ __typename: 'SynapseFunnelStep', eventName: 'view_item', count: 200 }, { __typename: 'SynapseFunnelStep', eventName: 'cart_add', count: 118 }] }
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [
        meMock({ superuser: true }),
        ...boardMocks(() => {}, { ...board, reports: [funnel] }),
        { request: { query: SynapseFunnelDocument, variables: () => true }, result: { data: { synapseFunnel: counts } }, maxUsageCount: 10 },
        { request: { query: SynapseFlowDocument, variables: () => true }, error: new Error('not in this test'), maxUsageCount: 10 },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'See where the 82 went' }, SLOW))
    expect(await screen.findByRole('region', { name: 'After View item, not Added to cart' })).toBeTruthy()
    expect(screen.getByRole('complementary', { name: 'Edit After View item, not Added to cart' })).toBeTruthy()
  })
})

function draftWrites(onKeep: (v: Record<string, unknown>) => void) {
  return [
    {
      request: {
        query: SaveBoardDraftDocument,
        variables: (v: Record<string, unknown>) => {
          onKeep(v)
          return true
        },
      },
      result: { data: { saveBoardDraft: '2026-11-04T11:00:00Z' } },
      maxUsageCount: 20,
    },
    { request: { query: DiscardBoardDraftDocument, variables: () => true }, result: { data: { discardBoardDraft: true } }, maxUsageCount: 20 },
  ]
}

function boardMocks(onSave: (v: Record<string, unknown>) => void, b: Record<string, unknown> = board, onKeep: (v: Record<string, unknown>) => void = () => {}) {
  const none = (query: DocumentNode) => ({ request: { query, variables: () => true }, error: new Error('not in this test'), maxUsageCount: 20 })
  return [
    { request: { query: BoardDocument, variables: { channelId: 'c1', id: 'b1' } }, result: { data: { board: b } }, maxUsageCount: 5 },
    ...draftWrites(onKeep),
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
