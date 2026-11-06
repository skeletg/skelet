import { describeReport } from '@/shared/analytics/kinds'
import { sameDefinition } from '@/shared/analytics/kinds/definition'
import { added, changed, removed } from '@/shared/saving/diff'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { REPORT_SIZE_LABEL, type BoardReport } from './report'

// What "Add to board" hands this page - a report staged on arrival, saved like any other edit
export type AddToBoardState = { add?: BoardReport }

// A report on the board being edited: key identifies it on screen, from is the position of the saved report it is - null
// for one added since - so an edit, a resize or a move reads as exactly that rather than as a remove and an add
export type DraftReport = BoardReport & { key: string; from: number | null }

export const keyed = (reports: BoardReport[]): DraftReport[] => reports.map((r, i) => ({ ...r, key: `s${i}`, from: i }))
export const fresh = (r: BoardReport): DraftReport => ({ ...r, key: crypto.randomUUID(), from: null })
export const unkeyed = (draft: DraftReport[]): BoardReport[] => draft.map(({ title, size, definition }) => ({ title, size, definition }))

// boardDiff is the staged change list the header Save reviews: the name, each report added, removed, retitled,
// rebuilt or resized, and the order when the reports that stayed moved
export function boardDiff(name: string, reports: BoardReport[], draftName: string, draft: DraftReport[]): SectionDiffEntry[] {
  const out: (SectionDiffEntry | null)[] = [changed('Name', name, draftName.trim())]
  reports.forEach((r, i) => {
    const label = `Report · ${r.title}`
    const d = draft.find((x) => x.from === i)
    if (!d) {
      out.push(removed(label, describeReport(r.definition)))
      return
    }
    out.push(changed(`${label} · title`, r.title, d.title.trim()))
    if (!sameDefinition(r.definition, d.definition)) {
      // A filter or window can change without changing the one-line description, and that's still a change to show
      const [before, after] = [describeReport(r.definition), describeReport(d.definition)]
      out.push(changed(`${label} · settings`, before === after ? 'Earlier settings' : before, after))
    }
    out.push(changed(`${label} · size`, REPORT_SIZE_LABEL[r.size], REPORT_SIZE_LABEL[d.size]))
  })
  for (const d of draft) if (d.from === null) out.push(added(`Report · ${d.title.trim()}`, describeReport(d.definition)))
  const order = draft.flatMap((d) => (d.from === null ? [] : [d.from]))
  if (order.some((f, i) => i > 0 && f < order[i - 1])) {
    // Numbered, so swapping two reports of the same title still reads as a change
    const titles = (positions: number[]) => positions.map((p) => `${p + 1}. ${reports[p].title}`).join(', ')
    out.push(changed('Report order', titles([...order].sort((a, b) => a - b)), titles(order)))
  }
  return out.filter((e) => e !== null)
}
