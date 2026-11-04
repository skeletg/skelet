import { describe, expect, it } from 'vitest'
import type { BoardsQuery } from './Boards.gql'
import { inView } from './BoardsPage'

type Row = BoardsQuery['boards'][number]
const row = (over: Partial<Row>): Row => ({ __typename: 'Board', id: 'b', name: 'B', updatedAt: '', reportCount: 0, status: 'published', access: 'view', favourite: false, draftUpdatedAt: null, owner: null, ...over }) as Row
const owner = (id: string) => ({ __typename: 'BoardOwner' as const, id, name: id, removed: false })

describe('the boards list views', () => {
  const mine = row({ owner: owner('me'), access: 'own' })
  const theirs = row({ owner: owner('bo'), favourite: true })
  const shop = row({})
  const draft = row({ owner: owner('me'), status: 'draft' })
  const unsaved = row({ owner: owner('bo'), draftUpdatedAt: '2026-11-04T10:00:00Z' })
  const all = [mine, theirs, shop, draft, unsaved]
  const pick = (v: Parameters<typeof inView>[0]) => all.filter((b) => inView(v, 'me', b))
  it('split what the admin can open by who it belongs to', () => {
    expect(pick('mine')).toEqual([mine, draft])
    expect(pick('shared')).toEqual([theirs, unsaved])
    expect(pick('shop')).toEqual([shop])
  })
  it('find the starred boards and the ones with unsaved work', () => {
    expect(pick('favourites')).toEqual([theirs])
    expect(pick('drafts')).toEqual([draft, unsaved])
    expect(pick('all')).toEqual(all)
  })
})
