import type { ControlsProps, Kind, ReportDefinition, ReportKind, ResultProps } from './definition'
import { flow } from './flow'
import { funnel } from './funnel'
import { insights } from './insights'
import { retention, stickiness } from './retention'
import { Note } from './ui'

// Every report kind, in the order "+ Add report" lists them
export const KINDS: Record<ReportKind, Kind> = { INSIGHTS: insights, FUNNEL: funnel, FLOW: flow, RETENTION: retention, STICKINESS: stickiness }

// A kind this build doesn't know - saved by a newer one - has none
export const kindOf = (d: ReportDefinition) => KINDS[d.kind] as Kind | undefined

export const describeReport = (d: ReportDefinition) => kindOf(d)?.describe(d) ?? d.kind

// The builders "+ Add report" offers
export const BUILDERS = (Object.keys(KINDS) as ReportKind[]).flatMap((kind) => {
  const k = KINDS[kind]
  return 'menu' in k ? [{ kind, ...k.menu }] : []
})

// The builder a kind is edited in - its own, or the one it's a view of
export const builderOf = (kind: ReportKind): ReportKind => {
  const k = KINDS[kind] as Kind | undefined
  return k && 'builder' in k ? k.builder : kind
}

export function ReportControls(p: ControlsProps) {
  const k = kindOf(p.value)
  return k ? <k.Controls {...p} /> : null
}

export function ReportResult(p: ResultProps) {
  const k = kindOf(p.d)
  if (!k) return <Note>This report was saved by a newer version and can't be shown here.</Note>
  return <k.Result {...p} />
}
