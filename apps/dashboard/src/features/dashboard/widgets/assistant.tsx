import { useQuery } from '@apollo/client/react'
import { useConfigScope } from '@/lib/config-scope'
import { AssistantReportDocument } from '@/features/dashboard/Assistant.gql'
import { AssistantTiles } from '@/features/agentic/AssistantTiles'
import { SALES_DAYS } from './sales'
import { NeedsChannel, WidgetSection } from './primitives'

export function AssistantWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(AssistantReportDocument, {
    variables: { channelId: effectiveChannelId ?? '', days: SALES_DAYS },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const r = data?.assistantReport

  return (
    <WidgetSection
      title="Shopping assistant"
      description={`What the storefront assistant cost and sold over the last ${SALES_DAYS} days.`}
      to="/agentic"
      toLabel="Overview"
      loading={!!effectiveChannelId && loading && !r}
      error={error ?? null}
    >
      {!effectiveChannelId || !r ? (
        <NeedsChannel what="the assistant's numbers" />
      ) : (
        <AssistantTiles report={r} />
      )}
    </WidgetSection>
  )
}
