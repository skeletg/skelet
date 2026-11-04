import { ScratchReport } from './ScratchReport'

const description = 'Whether people come back - as one return rate, or period by period for each starting cohort.'

export function SynapseRetention() {
  return <ScratchReport kinds={['retention', 'stickiness']} title="Retention" what="Retention reports" description={description} defaultDays={90} />
}

// The old Engagement page is Retention's return-rate view now; its links keep working
export function SynapseStickiness() {
  return <ScratchReport kinds={['stickiness', 'retention']} title="Retention" what="Retention reports" description={description} />
}
