import type { BoardReportDefinitionInput, BoardReportKind } from '@/graphql/schema.types'
import type { ReactNode } from 'react'
import type { BoardReportFieldsFragment } from './Boards.gql'
import { FlowTile, FunnelTile, InsightsTile, RetentionTile, StickinessTile, type TileProps } from './tiles'

// A saved report - which Synapse engine and its settings, as a board stores it
export type ReportKind = BoardReportKind
export type ReportDefinition = BoardReportDefinitionInput
export type BoardReport = { title: string; definition: ReportDefinition }

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

// Each report kind once: its name, the one line saying what it counts, and the tile that runs it through the Synapse
// query of its kind - the same read the report page makes. An unset setting takes that query's own default
export const REPORTS: Record<ReportKind, { label: string; describe: (d: ReportDefinition) => string; Tile: (p: TileProps) => ReactNode }> = {
  INSIGHTS: {
    label: 'Insights',
    describe: (d) => `${d.measure === 'unique' ? 'People doing' : 'Count of'} ${d.event || 'any event'}${d.breakdown ? ` by ${d.breakdown}` : ''}`,
    Tile: InsightsTile,
  },
  FUNNEL: {
    label: 'Funnel',
    describe: (d) => (d.steps ?? []).join(' → ') + (d.windowHours ? ` within ${d.windowHours} h` : ''),
    Tile: FunnelTile,
  },
  RETENTION: { label: 'Retention', describe: (d) => `${d.startEvent} then ${d.returnEvent}, by ${d.unit ?? 'day'}`, Tile: RetentionTile },
  STICKINESS: { label: 'Engagement', describe: (d) => `Returning on ${d.event || 'any activity'}`, Tile: StickinessTile },
  FLOW: { label: 'Flow', describe: (d) => `After ${d.event}`, Tile: FlowTile },
}

// A kind this build doesn't know - saved by a newer one - has no entry
export const reportOf = (d: ReportDefinition) => REPORTS[d.kind] as (typeof REPORTS)[ReportKind] | undefined
