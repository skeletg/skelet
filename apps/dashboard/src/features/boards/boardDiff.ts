import { added, changed, removed, sameJson } from '@/shared/saving/diff'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { reportOf, type BoardReport, type ReportDefinition } from './report'

// What "Add to board" hands this page - a report staged on arrival, saved like any other edit
export type AddToBoardState = { add?: BoardReport }

const same = (a: BoardReport, b: BoardReport) => a.title === b.title && sameJson(a.definition, b.definition)
const describe = (d: ReportDefinition) => reportOf(d)?.describe(d) ?? d.kind

// boardDiff is the staged change list the header Save reviews: the name, each report added or taken off, a title
// changed in place, and the order when only that moved
export function boardDiff(name: string, reports: BoardReport[], draftName: string, draft: BoardReport[]): SectionDiffEntry[] {
  const out: SectionDiffEntry[] = []
  const n = changed('Name', name, draftName.trim())
  if (n) out.push(n)
  const left = [...draft]
  const gone: BoardReport[] = []
  for (const r of reports) {
    const i = left.findIndex((d) => same(d, r))
    if (i >= 0) left.splice(i, 1)
    else gone.push(r)
  }
  for (const g of [...gone]) {
    const i = left.findIndex((d) => sameJson(d.definition, g.definition))
    if (i >= 0) {
      const t = changed(`Report · ${g.title} · title`, g.title, left[i].title.trim())
      if (t) out.push(t)
      left.splice(i, 1)
      gone.splice(gone.indexOf(g), 1)
    }
  }
  for (const g of gone) out.push(removed(`Report · ${g.title}`, describe(g.definition)))
  for (const a of left) out.push(added(`Report · ${a.title}`, describe(a.definition)))
  if (out.length === 0) {
    const order = (rs: BoardReport[]) => rs.map((r) => r.title).join(', ')
    const o = changed('Report order', order(reports), order(draft))
    if (o) out.push(o)
  }
  return out
}
