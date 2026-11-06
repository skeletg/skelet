import { ScratchReport } from './ScratchReport'

export function SynapseInsights() {
  return <ScratchReport kinds={['INSIGHTS']} title="Insights" what="Insights" description="Count any event over time, split by a property." />
}
