import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { BoardShareTargetsDocument, SetBoardSharesDocument, type BoardFullFragment } from './Boards.gql'
import { ShareDialog, shareWarning } from './ShareDialog'

const board = {
  __typename: 'Board',
  id: 'b1',
  name: 'Product X',
  updatedAt: '2026-11-04T10:00:00Z',
  status: 'published',
  access: 'own',
  favourite: false,
  owner: { __typename: 'BoardOwner', id: 'me', name: 'Ada', removed: false },
  reports: [],
  shares: [],
  draft: null,
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
    renderDialog(<ShareDialog channelId="c1" shop="Main" board={board} open onOpenChange={() => {}} />, [
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
    expect(sent).toMatchObject({ id: 'b1', shares: [{ adminId: null, roleId: 'mkt', level: 'view' }] })
  })
})
