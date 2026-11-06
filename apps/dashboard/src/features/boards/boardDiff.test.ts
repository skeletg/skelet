import { describe, expect, it } from 'vitest'
import type { BoardReport } from './report'
import { boardDiff, fresh, keyed } from './boardDiff'

const views: BoardReport = { title: 'Views', size: 'MEDIUM', definition: { kind: 'INSIGHTS', event: 'page_view' } }
const funnel: BoardReport = { title: 'Funnel', size: 'MEDIUM', definition: { kind: 'FUNNEL', steps: ['page_view', 'order_placed'] } }
const saved = [views, funnel]

describe('boardDiff', () => {
  it('is empty for an untouched board', () => {
    expect(boardDiff('Mine', saved, 'Mine', keyed(saved))).toEqual([])
  })
  it('lists a rename, an added and a removed report', () => {
    const d = boardDiff('Mine', [views], ' Ours ', [fresh(funnel)])
    expect(d.map((e) => [e.kind, e.label])).toEqual([
      ['change', 'Name'],
      ['remove', 'Report · Views'],
      ['add', 'Report · Funnel'],
    ])
  })
  it('shows a retitled report as one change, not a remove and an add', () => {
    const d = boardDiff('Mine', [views], 'Mine', [{ ...keyed([views])[0], title: 'Page views' }])
    expect(d).toEqual([expect.objectContaining({ label: 'Report · Views · title', before: 'Views', after: 'Page views' })])
  })
  it('shows a report rebuilt in the sheet as a change to its settings', () => {
    const [v] = keyed([views])
    const d = boardDiff('Mine', [views], 'Mine', [{ ...v, definition: { ...v.definition, event: 'cart_add' } }])
    expect(d).toEqual([expect.objectContaining({ label: 'Report · Views · settings', before: 'Count of page_view', after: 'Count of cart_add' })])
  })
  it('still shows a settings change the one-line description hides', () => {
    const [f] = keyed([funnel])
    const d = boardDiff('Mine', [funnel], 'Mine', [{ ...f, definition: { ...f.definition, filters: [{ step: 0, key: 'device', value: 'mobile' }] } }])
    expect(d).toEqual([expect.objectContaining({ label: 'Report · Funnel · settings', before: 'Earlier settings' })])
  })
  it('shows a resize on its own, not as a settings change', () => {
    const [v] = keyed([views])
    const d = boardDiff('Mine', [views], 'Mine', [{ ...v, size: 'FULL' }])
    expect(d).toEqual([expect.objectContaining({ label: 'Report · Views · size', before: 'Medium', after: 'Full row' })])
  })
  it('names the new order when the reports that stayed moved', () => {
    const [v, f] = keyed(saved)
    const d = boardDiff('Mine', saved, 'Mine', [f, v, fresh(views)])
    expect(d).toContainEqual(expect.objectContaining({ label: 'Report order', before: '1. Views, 2. Funnel', after: '2. Funnel, 1. Views' }))
  })
  it("doesn't count a setting cleared back to unset, or set in another order, as a change", () => {
    const [v] = keyed([views])
    expect(boardDiff('Mine', [views], 'Mine', [{ ...v, definition: { breakdown: '', event: 'page_view', kind: 'INSIGHTS' } }])).toEqual([])
  })
  it('shows a duplicated report as added, not as the one it was copied from', () => {
    const [v] = keyed([views])
    expect(boardDiff('Mine', [views], 'Mine', [v, fresh({ ...v, title: 'Views (copy)' })])).toEqual([expect.objectContaining({ kind: 'add', label: 'Report · Views (copy)' })])
  })
  it('shows two reports of the same title swapping places', () => {
    const [a, b] = keyed([views, views])
    expect(boardDiff('Mine', [views, views], 'Mine', [b, a])).toEqual([expect.objectContaining({ label: 'Report order', before: '1. Views, 2. Views', after: '2. Views, 1. Views' })])
  })
})
