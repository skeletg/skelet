import type { BoardReportDefinitionInput, BoardReportKind, BoardReportSize } from '@/graphql/schema.types'
import type { BoardReportFieldsFragment } from './Boards.gql'

// A saved report - which Synapse engine and its settings, as a board stores it
export type ReportKind = BoardReportKind
export type ReportSize = BoardReportSize
export type ReportDefinition = BoardReportDefinitionInput
export type BoardReport = { title: string; definition: ReportDefinition }

export const REPORT_SIZE_LABEL: Record<ReportSize, string> = { SMALL: 'Small', MEDIUM: 'Medium', FULL: 'Full row' }

// A definition as the input takes it: unset keys dropped, and no __typename, which Apollo adds to every object a query
// reads and an input refuses. A definition built from page state and one read back then compare and save the same
export function cleanDefinition(d: ReportDefinition | BoardReportFieldsFragment['definition']): ReportDefinition {
  const { filters, ...rest } = d
  const fields = { ...rest, filters: filters?.map(({ step, key, value }) => ({ step, key, value })) }
  return Object.fromEntries(
    Object.entries(fields).filter(
      ([k, v]) => k !== '__typename' && v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0),
    ),
  ) as ReportDefinition
}

export const cleanReport = (r: BoardReportFieldsFragment): BoardReport => ({ title: r.title, definition: cleanDefinition(r.definition) })

// Each report kind once: its name, the one line saying what it counts, and what a new one starts as - on a board's
// "+ Add report" and on a report page opened bare. An unset setting takes that kind's Synapse query's own default
export const REPORTS: Record<ReportKind, { label: string; describe: (d: ReportDefinition) => string; start: ReportDefinition }> = {
  INSIGHTS: {
    label: 'Insights',
    describe: (d) => `${d.measure === 'unique' ? 'People doing' : 'Count of'} ${d.event || 'any event'}${d.breakdown ? ` by ${d.breakdown}` : ''}`,
    start: { kind: 'INSIGHTS', event: 'page_view', measure: 'total', unit: 'day' },
  },
  FUNNEL: {
    label: 'Funnel',
    describe: (d) => (d.steps ?? []).join(' → ') + (d.windowHours ? ` within ${d.windowHours} h` : ''),
    start: { kind: 'FUNNEL', steps: ['page_view', 'view_item', 'cart_add', 'begin_checkout', 'order_placed'] },
  },
  RETENTION: {
    label: 'Retention by cohort',
    describe: (d) => `${d.startEvent} then ${d.returnEvent}, by ${d.unit ?? 'day'}`,
    start: { kind: 'RETENTION', startEvent: 'order_placed', returnEvent: 'order_placed', unit: 'week', periods: 8 },
  },
  STICKINESS: { label: 'Return rate', describe: (d) => `Returning on ${d.event || 'any activity'}`, start: { kind: 'STICKINESS' } },
  FLOW: { label: 'Flow', describe: (d) => `After ${d.event}`, start: { kind: 'FLOW', event: 'page_view', depth: 4 } },
}

// A kind this build doesn't know - saved by a newer one - has no entry
export const reportOf = (d: ReportDefinition) => REPORTS[d.kind] as (typeof REPORTS)[ReportKind] | undefined
export const describeReport = (d: ReportDefinition) => reportOf(d)?.describe(d) ?? d.kind
