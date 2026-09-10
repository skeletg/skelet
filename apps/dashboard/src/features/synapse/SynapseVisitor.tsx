import { useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { ActivityProfile } from './ActivityProfile'
import { VisitorRecordingsSection } from '@/features/lens/SessionList'
import { VisitorProfileDocument } from './Synapse.signals.gql'

// SynapseVisitor is the not-yet-identified counterpart of SynapseCustomer: the behavioural profile of one anonymous
// visitor (the lens_aid cookie), reached from the Synapse overview's "most active visitors" list
export function SynapseVisitor() {
  const { anonId = '' } = useParams()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { data, loading, error } = useQuery(VisitorProfileDocument, {
    variables: { channelId, anonId, sinceDays: 90 },
    skip: !anonId || !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const p = data?.visitorProfile
  const short = anonId.length > 12 ? anonId.slice(0, 8) : anonId

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={`Anonymous visitor ${short}`}
        badges={<Badge variant="secondary">Not identified</Badge>}
        meta={
          <span className="text-xs text-muted-foreground">
            A visitor who hasn't signed in yet - tracked by their browser id. Their activity stitches into a customer
            profile automatically once they log in. Last 90 days.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load visitor profile</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Visitor profiles are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : !p ? (
          <DetailPageSection title="Activity">
            <p className="text-sm text-muted-foreground">{loading ? 'Loading…' : 'No activity yet.'}</p>
          </DetailPageSection>
        ) : (
          <ActivityProfile profile={p} emptyText="No activity recorded for this visitor in this window." />
        )}
        <VisitorRecordingsSection anonId={anonId} />
      </DetailPageBody>
    </DetailPageLayout>
  )
}
