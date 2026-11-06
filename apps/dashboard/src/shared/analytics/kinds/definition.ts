import type { ReactNode } from 'react'
import type { BoardReportDefinitionInput, BoardReportKind } from '@/graphql/schema.types'

// A report - which Synapse engine and its settings. A report page keeps one in its URL and a board stores one per tile
export type ReportKind = BoardReportKind
export type ReportDefinition = BoardReportDefinitionInput

// What every kind's controls and result take - a board's edit panel, its tiles and the report pages hand them the same
export type ControlsProps = { channelId: string; value: ReportDefinition; onChange: (next: ReportDefinition) => void }
export type ResultProps = { channelId: string; d: ReportDefinition; sinceDays: number; compact?: boolean }

// One report kind, whole: what it's called, what it counts, what a new one starts as, and how it's built and shown.
// A kind is either on "+ Add report" itself (menu) or a view inside another kind's builder (Return rate is Retention's)
export type Kind = {
  label: string
  describe: (d: ReportDefinition) => string // one line saying what it counts
  title: (d: ReportDefinition) => string // a title for a report nobody has named yet
  start: ReportDefinition
  Controls: (p: ControlsProps) => ReactNode
  Result: (p: ResultProps) => ReactNode
} & ({ menu: { label: string; hint: string } } | { builder: ReportKind })

// A definition as read back from the API: unset fields null, and Apollo's __typename on every object
type Read = { [K in keyof ReportDefinition]?: ReportDefinition[K] | null } & Pick<ReportDefinition, 'kind'>

// A definition as the input takes it: unset keys dropped, and no __typename, which an input refuses. A definition built
// from page state and one read back then save the same
export function cleanDefinition(d: Read): ReportDefinition {
  const { filters, ...rest } = d
  const fields = { ...rest, filters: filters?.map(({ step, key, value }) => ({ step, key, value })) }
  return Object.fromEntries(
    Object.entries(fields).filter(
      ([k, v]) => k !== '__typename' && v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0),
    ),
  ) as ReportDefinition
}

// Whether two definitions run the same report - an unset field and the order the fields were set in don't count
const canonical = (d: ReportDefinition) => JSON.stringify(Object.entries(cleanDefinition(d)).sort(([a], [b]) => a.localeCompare(b)))
export const sameDefinition = (a: ReportDefinition, b: ReportDefinition) => canonical(a) === canonical(b)
