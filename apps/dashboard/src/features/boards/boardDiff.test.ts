import { describe, expect, it } from 'vitest'
import type { BoardReport } from './report'
import { boardDiff } from './boardDiff'

const views: BoardReport = { title: 'Views', definition: { kind: 'INSIGHTS', event: 'page_view' } }
const funnel: BoardReport = { title: 'Funnel', definition: { kind: 'FUNNEL', steps: ['page_view', 'order_placed'] } }

describe('boardDiff', () => {
  it('is empty for an untouched board', () => {
    expect(boardDiff('Mine', [views, funnel], 'Mine', [views, funnel])).toEqual([])
  })
  it('lists a rename, an added and a removed report', () => {
    const d = boardDiff('Mine', [views], ' Ours ', [funnel])
    expect(d.map((e) => [e.kind, e.label])).toEqual([
      ['change', 'Name'],
      ['remove', 'Report · Views'],
      ['add', 'Report · Funnel'],
    ])
  })
  it('shows a retitled report as one change, not a remove and an add', () => {
    const d = boardDiff('Mine', [views], 'Mine', [{ ...views, title: 'Page views' }])
    expect(d).toHaveLength(1)
    expect(d[0]).toMatchObject({ label: 'Report · Views · title', before: 'Views', after: 'Page views' })
  })
  it('names the new order when only the order moved', () => {
    const d = boardDiff('Mine', [views, funnel], 'Mine', [funnel, views])
    expect(d).toEqual([expect.objectContaining({ label: 'Report order', before: 'Views, Funnel', after: 'Funnel, Views' })])
  })
})
