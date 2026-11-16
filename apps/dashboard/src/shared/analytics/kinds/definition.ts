import type { ReactNode } from 'react'
import type { BoardReportDefinitionInput, BoardReportKind, PropertyFilterOp, SynapseFunnelFilterInput, SynapsePropertyFilterInput } from '@/graphql/schema.types'

// A report - which Synapse engine and its settings. A report page keeps one in its URL and a board stores one per tile
export type ReportKind = BoardReportKind
export type ReportDefinition = BoardReportDefinitionInput

// What every kind's controls and result take - a board's edit panel, its tiles and the report pages hand them the same
export type ControlsProps = { channelId: string; value: ReportDefinition; onChange: (next: ReportDefinition) => void }
export type ResultProps = {
  channelId: string
  d: ReportDefinition
  sinceDays: number
  compact?: boolean
  // Where a result can put a report it leads to - "see where the people who dropped off went" - on the board or a page
  onAddReport?: (title: string, d: ReportDefinition) => void
}

export type ConditionOp = PropertyFilterOp
// Holds a report's events to those whose property compares to the values - the same shape every engine takes
export type Condition = SynapsePropertyFilterInput

export const CONDITION_OP_LABEL: Record<ConditionOp, string> = { EQ: '=', NEQ: '≠', IN: 'is one of' }

// Net sales: what every line sold, edited and refunded comes to, in the base currency - refunds and removals are negative
export const NET_SALES = { events: ['order_line', 'order_line_adjusted', 'refund_line'], measure: 'sum', sumKey: 'net_base_cents' } as const
export const isNetSales = (d: ReportDefinition) => d.measure === 'sum' && d.sumKey === NET_SALES.sumKey && (d.events ?? []).join() === NET_SALES.events.join()

export const describeConditions = (cs: readonly Condition[] | null | undefined) =>
  (cs ?? []).map((c) => `${c.key} ${CONDITION_OP_LABEL[c.op]} ${c.values.join(', ')}`).join(' and ')

// A condition with no value yet holds nothing back, so it neither runs nor saves until it has one - and one read back
// loses its __typename, which an input refuses
export const readyConditions = (cs: readonly Condition[] | null | undefined): Condition[] =>
  (cs ?? [])
    .filter((c) => c.key.trim() && c.values.some((v) => v.trim()))
    .map((c) => ({ key: c.key.trim(), op: c.op, values: c.values.map((v) => v.trim()).filter(Boolean) }))

type ReadFilter = Omit<SynapseFunnelFilterInput, 'op' | 'values' | 'value'> & { op?: ConditionOp | null; values?: string[] | null; value?: string | null }

// A funnel's step filters as an operator and its values - one saved before operators held a lone value, which is EQ.
// The one place that older shape is read
export const funnelFilters = (fs: readonly ReadFilter[] | null | undefined) =>
  (fs ?? []).map(({ step, key, op, values, value }) => ({ step, key, op: op ?? 'EQ', values: values?.length ? values : [value ?? ''] }))

// One report kind, whole: what it's called, what it counts, what a new one starts as, and how it's built and shown.
// A kind is either on "+ Add report" itself (menu) or a view inside another kind's builder (Return rate is Retention's)
export type Kind = {
  label: string
  page: string // its report page, where a report one page leads to opens
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
  const { filters, where, startWhere, returnWhere, ...rest } = d
  const fields = {
    ...rest,
    filters: funnelFilters(filters),
    where: readyConditions(where),
    startWhere: readyConditions(startWhere),
    returnWhere: readyConditions(returnWhere),
  }
  return Object.fromEntries(
    Object.entries(fields).filter(
      ([k, v]) => k !== '__typename' && v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0),
    ),
  ) as ReportDefinition
}

// Whether two definitions run the same report - an unset field and the order the fields were set in don't count
const canonical = (d: ReportDefinition) => JSON.stringify(Object.entries(cleanDefinition(d)).sort(([a], [b]) => a.localeCompare(b)))
export const sameDefinition = (a: ReportDefinition, b: ReportDefinition) => canonical(a) === canonical(b)
