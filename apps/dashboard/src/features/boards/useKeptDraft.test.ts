import { describe, expect, it } from 'vitest'
import type { BoardFullFragment } from './Boards.gql'
import { cleanReport } from './report'
import { staging } from './useKeptDraft'

const report = (title: string) => ({
  __typename: 'BoardReport' as const,
  title,
  size: null,
  definition: { __typename: 'BoardReportDefinition' as const, kind: 'INSIGHTS' as const, event: 'page_view', sinceDays: null, measure: null, unit: null, breakdown: null, steps: null, filters: null, windowHours: null, startEvent: null, returnEvent: null, periods: null, depth: null, windowDays: null, events: null, sumKey: null, where: null, startWhere: null, returnWhere: null, notThen: null, notThenWithinHours: null },
})
const board = { id: 'b1', name: 'Weekly', updatedAt: 't1', draft: null, reports: [report('A'), report('B')] } as unknown as BoardFullFragment
const saved = board.reports.map(cleanReport)
const withDraft = (reports: { from: number | null; report: ReturnType<typeof report> }[]) =>
  ({ ...board, draft: { name: 'Kept', reports, baseUpdatedAt: 't0', updatedAt: 't0' } }) as unknown as BoardFullFragment

describe('staging', () => {
  it('opens on the board as saved when nothing was kept', () => {
    expect(staging(board, saved, true)).toMatchObject({ name: 'Weekly', reports: [{ title: 'A', from: 0 }, { title: 'B', from: 1 }] })
  })
  it('opens on the kept edits, each still the saved report it came from', () => {
    const s = staging(withDraft([{ from: 1, report: report('B') }, { from: null, report: report('New') }]), saved, true)
    expect(s.name).toBe('Kept')
    expect(s.reports.map((r) => [r.title, r.from])).toEqual([['B', 1], ['New', null]])
  })
  it("reads a kept position the board no longer has as an added report, so the diff can't look it up", () => {
    expect(staging(withDraft([{ from: 2, report: report('Gone') }]), saved, true).reports[0].from).toBeNull()
  })
  it("ignores kept edits for someone who can't edit the board", () => {
    expect(staging(withDraft([{ from: 0, report: report('A') }]), saved, false).name).toBe('Weekly')
  })
})
