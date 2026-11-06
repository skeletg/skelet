import type { DocumentNode } from 'graphql'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { SynapseInsightsDocument, SynapseOverviewDocument, SynapseStickinessDocument } from '@/features/synapse/Synapse.gql'
import { BoardDocument, DiscardBoardDraftDocument, SaveBoardDocument, SaveBoardDraftDocument } from './Boards.gql'
import { BoardPage } from './BoardPage'
import { cleanReport } from './report'

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
    kind: 'STICKINESS' as const,
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
    windowDays: null,
  },
  size: null,
})
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
      path: '/synapse/boards/b1',
      routes: { '/synapse/boards/:id': <BoardPage /> },
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
    })
    // Sent back as the input takes it - no nulls, no __typename
    expect((saved as unknown as { reports: unknown[] }).reports).toEqual([{ title: 'Buyers', size: 'MEDIUM', definition: { kind: 'STICKINESS', event: 'order_placed' } }])
  })

  it('adds a report from the header, builds and sizes it in the panel beside the tile, and saves it with the board', async () => {
    let saved: Record<string, unknown> | null = null
    renderPage({
      path: '/synapse/boards/b1',
      routes: { '/synapse/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks((v) => (saved = v))],
    })
    fireEvent.pointerDown((await screen.findAllByRole('button', { name: 'Add report' }, SLOW))[0], { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /^Insights/ }))

    const panel = await screen.findByRole('complementary', { name: 'Edit Page view' }, SLOW)
    expect(screen.getByRole('region', { name: 'Page view' })).toBeTruthy() // the tile is on the grid while it's built
    fireEvent.click(within(panel).getByRole('button', { name: 'Unique' }))
    fireEvent.change(within(panel).getByRole('combobox', { name: 'Size' }), { target: { value: 'FULL' } })
    fireEvent.change(within(panel).getByRole('textbox', { name: 'Title' }), { target: { value: 'Visitors' } })

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const dialog = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(dialog).getByText('Report · Visitors')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply 1 change' }))
    await waitFor(() => expect(saved).not.toBeNull(), SLOW)
    expect((saved as unknown as { reports: unknown[] }).reports).toEqual([
      ...board.reports.map(cleanReport),
      { title: 'Visitors', size: 'FULL', definition: { kind: 'INSIGHTS', event: 'page_view', measure: 'unique', unit: 'day' } },
    ])
  })

  it("stages a saved report's resize as just that, and Esc closes the panel", async () => {
    renderPage({
      path: '/synapse/boards/b1',
      routes: { '/synapse/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks(() => {})],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Buyers' }, SLOW))
    const panel = await screen.findByRole('complementary', { name: 'Edit Buyers' })
    fireEvent.change(within(panel).getByRole('combobox', { name: 'Size' }), { target: { value: 'SMALL' } })
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
        payload: { name: 'Weekly, kept', reports: [{ ...cleanReport(board.reports[1]), key: 's1', from: 1 }] },
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

  it('opens the saved board when the kept draft is in a shape this build no longer writes', async () => {
    const oldDraft = {
      ...board,
      draft: { __typename: 'BoardDraft', payload: { name: 'Old shape', reports: [{ ...board.reports[1], key: 's1' }] }, baseUpdatedAt: board.updatedAt, updatedAt: board.updatedAt },
    }
    renderPage({
      path: '/boards/b1',
      routes: { '/boards/:id': <BoardPage /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock({ superuser: true }), ...boardMocks(() => {}, oldDraft)],
    })
    expect(await screen.findByRole('region', { name: 'Returning' }, SLOW)).toBeTruthy()
    expect((screen.getByRole('textbox', { name: 'Board name' }) as HTMLInputElement).value).toBe(board.name)
    expect(screen.queryByText(/These are your unsaved changes from/)).toBeNull()
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
