import { eventLabel } from '@/features/synapse/eventLabel'
import type { ReportDefinition, ReportKind } from '@/features/boards/report'
import { FlowControls, FlowResult } from './flow'
import { FunnelControls, FunnelResult } from './funnel'
import { InsightsControls, InsightsResult } from './insights'
import { RetentionControls, RetentionResult, StickinessResult } from './retention'
import { Note, type ControlsProps, type ResultProps } from './ui'

// Each kind's controls and result - a report page, a board's sheet and a board's tile all render through these
const KINDS: Record<ReportKind, { Controls: (p: ControlsProps) => React.ReactNode; Result: (p: ResultProps) => React.ReactNode }> = {
  INSIGHTS: { Controls: InsightsControls, Result: InsightsResult },
  FUNNEL: { Controls: FunnelControls, Result: FunnelResult },
  FLOW: { Controls: FlowControls, Result: FlowResult },
  RETENTION: { Controls: RetentionControls, Result: RetentionResult },
  STICKINESS: { Controls: RetentionControls, Result: StickinessResult }, // one Retention builder, two views
}

// The builders "+ Add report" offers; Retention opens on its by-cohort view and switches to return rate inside
export const BUILDERS: { kind: ReportKind; label: string; hint: string }[] = [
  { kind: 'INSIGHTS', label: 'Insights', hint: 'Count any event over time' },
  { kind: 'FUNNEL', label: 'Funnel', hint: 'Conversion through steps, in order' },
  { kind: 'FLOW', label: 'Flow', hint: 'What people did next' },
  { kind: 'RETENTION', label: 'Retention', hint: 'Whether people come back' },
]

export function ReportControls(p: ControlsProps) {
  const k = KINDS[p.value.kind]
  return k ? <k.Controls {...p} /> : null
}

export function ReportResult(p: ResultProps) {
  const k = KINDS[p.d.kind]
  if (!k) return <Note>This report was saved by a newer version and can't be shown here.</Note>
  return <k.Result {...p} />
}

// A title for a report nobody has named yet
export function suggestTitle(d: ReportDefinition): string {
  switch (d.kind) {
    case 'INSIGHTS':
      return d.event ? eventLabel(d.event) : 'All events'
    case 'FUNNEL':
      return (d.steps ?? []).map(eventLabel).join(' → ')
    case 'FLOW':
      return `After ${eventLabel(d.event ?? '')}`
    case 'RETENTION':
      return `${eventLabel(d.startEvent ?? '')} retention`
    case 'STICKINESS':
      return d.event ? `Returning on ${eventLabel(d.event)}` : 'Returning shoppers'
  }
}
