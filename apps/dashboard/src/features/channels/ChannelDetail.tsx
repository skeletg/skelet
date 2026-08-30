import { useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { StarIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { PluginSections } from '@/plugins/detail-sections'
import { LoadError } from '@/shared/errors/LoadError'
import { ChannelDocument } from '@/features/channels/Channels.gql'
import { ChannelOverview } from '@/features/channels/ChannelOverview'

// ChannelDetail is a standalone PDP-style page reached from /channels/:id Header carries the "Back to channels"
// affordance + active/default badges; body holds the read-only ChannelOverview
export function ChannelDetail() {
  const params = useParams<{ id?: string }>()
  const id = params.id ?? ''

  const { data, loading, error, refetch } = useQuery(ChannelDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })

  if (loading && !data) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading channel…</p>
  }
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load channel"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.channel) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Channel not found.</p>
  }
  const channel = data.channel

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/channels"
        backLabel="Back to channels"
        title={channel.name}
        badges={
          <>
            <Badge variant={channel.isActive ? 'secondary' : 'outline'}>
              {channel.isActive ? 'Active' : 'Inactive'}
            </Badge>
            {channel.isDefault && (
              <Badge variant="default" className="gap-1">
                <StarIcon className="size-3" />
                Default
              </Badge>
            )}
          </>
        }
        meta={
          <>
            <code className="font-mono">{channel.slug}</code>
            <span>·</span>
            <span>{channel.defaultLocale}</span>
            <span>·</span>
            <code className="font-mono">{channel.currencyCode}</code>
          </>
        }
      />

      <DetailPageBody>
        <ChannelOverview channelId={channel.id} />
        <PluginSections target="channel" entity={channel} mode="edit" />
      </DetailPageBody>
    </DetailPageLayout>
  )
}
