import { REPORT_SIZE_LABEL, describeReport, type BoardReport, type ReportDefinition } from './report'
import { added, changed, removed } from '@/shared/saving/diff'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'

// What "Add to board" hands this page - a report staged on arrival, saved like any other edit
export type AddToBoardState = { add?: BoardReport }

// A report on the board being edited. key is `s<i>` for the board's i-th saved report and `n<…>` for one added since,
// so an edit, a resize or a move reads as exactly that rather than as a remove and an add
export type DraftReport = BoardReport & { key: string }

export const keyed = (reports: BoardReport[]): DraftReport[] => reports.map((r, i) => ({ ...r, key: `s${i}` }))
export const unkeyed = (draft: DraftReport[]): BoardReport[] => draft.map((d) => ({ title: d.title, definition: d.definition }))

const settings = (d: ReportDefinition) => JSON.stringify({ ...d, size: undefined }) // stringify drops the undefined size
const sizeLabel = (d: ReportDefinition) => REPORT_SIZE_LABEL[d.size ?? 'MEDIUM']

// boardDiff is the staged change list the header Save reviews: the name, each report added, removed, retitled,
// rebuilt or resized, and the order when the reports that stayed moved
export function boardDiff(name: string, reports: BoardReport[], draftName: string, draft: DraftReport[]): SectionDiffEntry[] {
  const out: SectionDiffEntry[] = []
  const n = changed('Name', name, draftName.trim())
  if (n) out.push(n)
  reports.forEach((r, i) => {
    const d = draft.find((x) => x.key === `s${i}`)
    if (!d) {
      out.push(removed(`Report · ${r.title}`, describeReport(r.definition)))
      return
    }
    const t = changed(`Report · ${r.title} · title`, r.title, d.title.trim())
    if (t) out.push(t)
    if (settings(r.definition) !== settings(d.definition)) {
      // A filter or window can change without changing the one-line description, and that's still a change to show
      const label = `Report · ${r.title} · settings`
      out.push(changed(label, describeReport(r.definition), describeReport(d.definition)) ?? changed(label, 'Earlier settings', describeReport(d.definition))!)
    }
    const s = changed(`Report · ${r.title} · size`, sizeLabel(r.definition), sizeLabel(d.definition))
    if (s) out.push(s)
  })
  for (const d of draft) if (!d.key.startsWith('s')) out.push(added(`Report · ${d.title.trim()}`, describeReport(d.definition)))
  const kept = draft.filter((d) => d.key.startsWith('s'))
  const before = kept.map((d) => d.key).sort((a, b) => Number(a.slice(1)) - Number(b.slice(1)))
  if (kept.some((d, i) => d.key !== before[i])) {
    const titles = (keys: string[]) => keys.map((k) => reports[Number(k.slice(1))].title).join(', ')
    const o = changed('Report order', titles(before), titles(kept.map((d) => d.key)))
    if (o) out.push(o)
  }
  return out
}
