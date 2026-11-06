// A saved report - which Synapse engine and its settings, as a board stores it (Signals' board.Definition, version 1).
// Signals refuses fields it doesn't know, so this type and that struct move together
export type ReportKind = 'insights' | 'funnel' | 'retention' | 'stickiness' | 'flow'

export type ReportDefinition = {
  kind: ReportKind
  sinceDays?: number // unset = the board's period
  event?: string // insights, stickiness, the flow's focus
  measure?: 'total' | 'unique'
  unit?: 'day' | 'week'
  breakdown?: string
  steps?: string[]
  filters?: { step: number; key: string; value: string }[]
  windowHours?: number
  startEvent?: string
  returnEvent?: string
  periods?: number
  depth?: number
}

export type BoardReport = { title: string; definition: ReportDefinition }

export const REPORT_KIND_LABEL: Record<ReportKind, string> = {
  insights: 'Insights',
  funnel: 'Funnel',
  retention: 'Retention',
  stickiness: 'Engagement',
  flow: 'Flow',
}

// Drops unset keys, so a definition built from page state compares and saves the same as one read back
export function cleanDefinition(d: ReportDefinition): ReportDefinition {
  return Object.fromEntries(
    Object.entries(d).filter(([, v]) => v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0)),
  ) as ReportDefinition
}

// One line saying what a report counts, for the save dialog and a tile's subtitle
export function describeReport(d: ReportDefinition): string {
  switch (d.kind) {
    case 'insights':
      return `${d.measure === 'unique' ? 'People doing' : 'Count of'} ${d.event || 'any event'}${d.breakdown ? ` by ${d.breakdown}` : ''}`
    case 'funnel':
      return (d.steps ?? []).join(' → ') + (d.windowHours ? ` within ${d.windowHours} h` : '')
    case 'retention':
      return `${d.startEvent} then ${d.returnEvent}, by ${d.unit ?? 'day'}`
    case 'stickiness':
      return `Returning on ${d.event || 'any activity'}`
    case 'flow':
      return `After ${d.event}`
  }
}

// The board-wide period every report without its own uses
export const PERIODS = [7, 30, 90, 180] as const
