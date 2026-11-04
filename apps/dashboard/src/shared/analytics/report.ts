// A saved report - which Synapse engine and its settings, as a board stores it (Signals' board.Definition, version 1).
// Signals refuses fields it doesn't know, so this type and that struct move together
export type ReportKind = 'insights' | 'funnel' | 'retention' | 'stickiness' | 'flow'

export type ReportDefinition = {
  kind: ReportKind
  sinceDays?: number // unset = the board's period
  event?: string // insights, stickiness, the flow's focus
  measure?: 'total' | 'unique' | 'sum'
  unit?: 'day' | 'week'
  breakdown?: string
  steps?: string[]
  filters?: { step: number; key: string; value: string; op?: ConditionOp; values?: string[] }[]
  windowHours?: number
  startEvent?: string
  returnEvent?: string
  periods?: number
  depth?: number
  windowDays?: number // stickiness: the return window, unset = the shop's default
  size?: ReportSize // the tile's width on a board, unset = medium
  events?: string[] // insights: several events counted together, in place of event
  sumKey?: string // insights with measure sum: the integer property added up
  where?: Condition[] // what the events must carry: insights, stickiness, the flow's focus
  startWhere?: Condition[] // retention's start
  returnWhere?: Condition[] // retention's return
  notThen?: string // flow: only the people who didn't do this after the focus
  notThenWithinHours?: number
}

export type ConditionOp = 'eq' | 'neq' | 'in'
// Holds a report's events to those whose property compares to the values - the same shape every engine takes
export type Condition = { key: string; op: ConditionOp; values: string[] }

export const CONDITION_OP_LABEL: Record<ConditionOp, string> = { eq: '=', neq: '≠', in: 'is one of' }

// Net sales: what every line sold, edited and refunded comes to, in the base currency - refunds and removals are negative
export const NET_SALES = { events: ['order_line', 'order_line_adjusted', 'refund_line'], measure: 'sum', sumKey: 'net_base_cents' } as const
export const isNetSales = (d: ReportDefinition) => d.measure === 'sum' && d.sumKey === NET_SALES.sumKey && (d.events ?? []).join() === NET_SALES.events.join()

export const describeConditions = (cs: Condition[] | undefined) =>
  (cs ?? []).map((c) => `${c.key} ${CONDITION_OP_LABEL[c.op]} ${c.values.join(', ')}`).join(' and ')

// A condition with no value yet holds nothing back, so it neither runs nor saves until it has one
export const readyConditions = (cs: Condition[] | undefined): Condition[] =>
  (cs ?? []).filter((c) => c.key.trim() && c.values.some((v) => v.trim())).map((c) => ({ ...c, key: c.key.trim(), values: c.values.map((v) => v.trim()).filter(Boolean) }))

// What the Synapse queries take for a definition's conditions
export const conditionsIn = (cs: Condition[] | undefined) => readyConditions(cs).map((c) => ({ key: c.key, op: c.op, values: c.values }))

export type ReportSize = 'small' | 'medium' | 'full'
export const REPORT_SIZE_LABEL: Record<ReportSize, string> = { small: 'Small', medium: 'Medium', full: 'Full row' }

export type BoardReport = { title: string; definition: ReportDefinition }

export const REPORT_KIND_LABEL: Record<ReportKind, string> = {
  insights: 'Insights',
  funnel: 'Funnel',
  retention: 'Retention by cohort',
  stickiness: 'Return rate',
  flow: 'Flow',
}

// What a new report of each kind starts as - on a board's "+ Add report" and on a report page opened bare
export const DEFAULT_DEFINITIONS: Record<ReportKind, ReportDefinition> = {
  insights: { kind: 'insights', event: 'page_view', measure: 'total', unit: 'day' },
  funnel: { kind: 'funnel', steps: ['page_view', 'view_item', 'cart_add', 'begin_checkout', 'order_placed'] },
  retention: { kind: 'retention', startEvent: 'order_placed', returnEvent: 'order_placed', unit: 'week', periods: 8 },
  stickiness: { kind: 'stickiness' },
  flow: { kind: 'flow', event: 'page_view', depth: 4 },
}

// Drops unset keys, so a definition built from page state compares and saves the same as one read back
export function cleanDefinition(d: ReportDefinition): ReportDefinition {
  d = { ...d, where: readyConditions(d.where), startWhere: readyConditions(d.startWhere), returnWhere: readyConditions(d.returnWhere) }
  return Object.fromEntries(
    Object.entries(d).filter(([, v]) => v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0)),
  ) as ReportDefinition
}

// One line saying what a report counts, for the save dialog and a tile's subtitle
export function describeReport(d: ReportDefinition): string {
  switch (d.kind) {
    case 'insights': {
      const what = isNetSales(d)
        ? 'Net sales'
        : d.measure === 'sum'
          ? `Sum of ${d.sumKey} on ${(d.events ?? [d.event]).filter(Boolean).join(', ') || 'any event'}`
          : `${d.measure === 'unique' ? 'People doing' : 'Count of'} ${(d.events ?? [d.event]).filter(Boolean).join(', ') || 'any event'}`
      const where = describeConditions(d.where)
      return `${what}${where ? ` where ${where}` : ''}${d.breakdown ? ` by ${d.breakdown}` : ''}`
    }
    case 'funnel':
      return (d.steps ?? []).join(' → ') + (d.windowHours ? ` within ${d.windowHours} h` : '')
    case 'retention':
      return `${d.startEvent}${d.startWhere?.length ? ` where ${describeConditions(d.startWhere)}` : ''} then ${d.returnEvent}${d.returnWhere?.length ? ` where ${describeConditions(d.returnWhere)}` : ''}, by ${d.unit ?? 'day'}`
    case 'stickiness':
      return `Returning on ${d.event || 'any activity'}${d.where?.length ? ` where ${describeConditions(d.where)}` : ''}`
    case 'flow':
      return `After ${d.event}${d.where?.length ? ` where ${describeConditions(d.where)}` : ''}${d.notThen ? `, not then ${d.notThen}` : ''}`
  }
}

// The board-wide period every report without its own uses
export const PERIODS = [7, 30, 90, 180] as const
