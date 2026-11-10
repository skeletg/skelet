import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { BoardShareTargetsDocument, SetBoardSharesDocument, TransferBoardDocument, type BoardFullFragment } from './Boards.gql'
import { ShareDialog, shareWarning } from './ShareDialog'

const board = {
  __typename: 'Board',
  id: 'b1',
  name: 'Product X',
  updatedAt: '2026-11-04T10:00:00Z',
  access: 'OWN',
  favourite: false,
  owner: { __typename: 'BoardOwner', id: 'me', name: 'Ada', removed: false },
  reports: [],
  shares: [],
  draft: null,
  accessRequests: [],
} as unknown as BoardFullFragment

const target = (kind: string, id: string, name: string, seesBoards = true, seesData = true) => ({ __typename: 'BoardShareTarget', kind, id, name, seesBoards, seesData })

describe('shareWarning', () => {
  it('warns about someone who would see the layout without the numbers, or nothing at all', () => {
    expect(shareWarning(target('role', 'r', 'Marketing', true, false), 'Main')).toMatch(/layout shows, its numbers don't/)
    expect(shareWarning(target('admin', 'a', 'Bo', false, false), 'Main')).toMatch(/won't see it/)
    expect(shareWarning(target('admin', 'a', 'Bo'), 'Main')).toBeNull()
  })
})

describe('ShareDialog', () => {
  it('shares read-only with a role and sends exactly that', async () => {
    let sent: Record<string, unknown> | null = null
    renderDialog(<ShareDialog channelId="c1" shop="Main" board={board} canManage={false} onClose={() => {}} />, [
      {
        request: { query: BoardShareTargetsDocument, variables: { channelId: 'c1' } },
        result: { data: { boardShareTargets: [target('admin', 'me', 'Ada'), target('role', 'mkt', 'Marketing', true, false)] } },
      },
      {
        request: {
          query: SetBoardSharesDocument,
          variables: (v: Record<string, unknown>) => {
            sent = v
            return true
          },
        },
        result: { data: { setBoardShares: board } },
      },
    ])
    const pick = await screen.findByRole('combobox', { name: 'Share with' }, SLOW)
    await waitFor(() => expect(within(pick).queryByRole('option', { name: 'Marketing' })).toBeTruthy())
    expect(within(pick).queryByRole('option', { name: 'Ada' })).toBeNull() // the owner isn't offered their own board
    fireEvent.change(pick, { target: { value: 'mkt' } })
    expect(screen.getByText(/layout shows, its numbers don't/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save sharing' }))
    await waitFor(() => expect(sent).not.toBeNull(), SLOW)
    expect(sent).toMatchObject({ id: 'b1', shares: [{ adminId: null, roleId: 'mkt', level: 'VIEW' }] })
  })
  it("starts from the board's shares, so adding someone keeps everyone it's already shared with", async () => {
    let sent: Record<string, unknown> | null = null
    const shared = { ...board, shares: [{ __typename: 'BoardShare', adminId: 'bo', roleId: null, name: 'Bo', level: 'EDIT' as const }] }
    renderDialog(<ShareDialog channelId="c1" shop="Main" board={shared} canManage={false} onClose={() => {}} />, [
      {
        request: { query: BoardShareTargetsDocument, variables: { channelId: 'c1' } },
        result: { data: { boardShareTargets: [target('admin', 'bo', 'Bo'), target('role', 'mkt', 'Marketing')] } },
      },
      {
        request: {
          query: SetBoardSharesDocument,
          variables: (v: Record<string, unknown>) => {
            sent = v
            return true
          },
        },
        result: { data: { setBoardShares: shared } },
      },
    ])
    expect(await screen.findByRole('combobox', { name: 'What Bo can do' }, SLOW)).toBeTruthy()
    const pick = screen.getByRole('combobox', { name: 'Share with' })
    await waitFor(() => expect(within(pick).queryByRole('option', { name: 'Marketing' })).toBeTruthy())
    fireEvent.change(pick, { target: { value: 'mkt' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save sharing' }))
    await waitFor(() => expect(sent).not.toBeNull(), SLOW)
    expect(sent).toMatchObject({ shares: [{ adminId: 'bo', roleId: null, level: 'EDIT' }, { adminId: null, roleId: 'mkt', level: 'VIEW' }] })
  })

  it('makes a board a shop board, for someone who manages boards, and back with an owner picked first', async () => {
    const sent: Record<string, unknown>[] = []
    const transferMock = {
      request: {
        query: TransferBoardDocument,
        variables: (v: Record<string, unknown>) => {
          sent.push(v)
          return true
        },
      },
      result: { data: { transferBoard: true } },
      maxUsageCount: 2,
    }
    const targets = { request: { query: BoardShareTargetsDocument, variables: { channelId: 'c1' } }, result: { data: { boardShareTargets: [target('admin', 'bo', 'Bo')] } }, maxUsageCount: 2 }
    const view = renderDialog(<ShareDialog channelId="c1" shop="Main" board={board} canManage onClose={() => {}} />, [targets, transferMock])
    fireEvent.change(await screen.findByRole('combobox', { name: 'General access' }, SLOW), { target: { value: 'shop' } })
    expect(screen.queryByRole('combobox', { name: 'Share with' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Save sharing' }))
    await waitFor(() => expect(sent).toEqual([{ channelId: 'c1', id: 'b1', ownerId: null }]), SLOW)
    view.unmount()

    const shopBoard = { ...board, owner: null } as unknown as BoardFullFragment
    renderDialog(<ShareDialog channelId="c1" shop="Main" board={shopBoard} canManage onClose={() => {}} />, [targets, transferMock])
    fireEvent.change(await screen.findByRole('combobox', { name: 'General access' }, SLOW), { target: { value: 'people' } })
    expect(screen.getByRole('button', { name: 'Save sharing' })).toHaveProperty('disabled', true) // nobody owns it yet
    const owner = screen.getByRole('combobox', { name: 'Owner' })
    await waitFor(() => expect(within(owner).queryByRole('option', { name: 'Bo' })).toBeTruthy())
    fireEvent.change(owner, { target: { value: 'bo' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save sharing' }))
    await waitFor(() => expect(sent[1]).toEqual({ channelId: 'c1', id: 'b1', ownerId: 'bo' }), SLOW)
  })
})
