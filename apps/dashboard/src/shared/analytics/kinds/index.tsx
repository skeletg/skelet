import { eventLabel } from '@/features/synapse/eventLabel'
import type { ReportDefinition, ReportKind } from '../report'
import { FlowControls, FlowResult } from './flow'
import { FunnelControls, FunnelResult } from './funnel'
import { InsightsControls, InsightsResult } from './insights'
import { RetentionControls, RetentionResult, StickinessResult } from './retention'
import { Note, type ControlsProps, type ResultProps } from './ui'

// Each kind's controls and result - a report page, a board's sheet and a board's tile all render through these
const KINDS: Record<ReportKind, { Controls: (p: ControlsProps) => React.ReactNode; Result: (p: ResultProps) => React.ReactNode }> = {
  insights: { Controls: InsightsControls, Result: InsightsResult },
  funnel: { Controls: FunnelControls, Result: FunnelResult },
  flow: { Controls: FlowControls, Result: FlowResult },
  retention: { Controls: RetentionControls, Result: RetentionResult },
  stickiness: { Controls: RetentionControls, Result: StickinessResult }, // one Retention builder, two views
}

// The builders "+ Add report" offers; Retention opens on its by-cohort view and switches to return rate inside
export const BUILDERS: { kind: ReportKind; label: string; hint: string }[] = [
  { kind: 'insights', label: 'Insights', hint: 'Count any event over time' },
  { kind: 'funnel', label: 'Funnel', hint: 'Conversion through steps, in order' },
  { kind: 'flow', label: 'Flow', hint: 'What people did next' },
  { kind: 'retention', label: 'Retention', hint: 'Whether people come back' },
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
    case 'insights':
      return d.event ? eventLabel(d.event) : 'All events'
    case 'funnel':
      return (d.steps ?? []).map(eventLabel).join(' → ')
    case 'flow':
      return `After ${eventLabel(d.event ?? '')}`
    case 'retention':
      return `${eventLabel(d.startEvent ?? '')} retention`
    case 'stickiness':
      return d.event ? `Returning on ${eventLabel(d.event)}` : 'Returning shoppers'
  }
}
