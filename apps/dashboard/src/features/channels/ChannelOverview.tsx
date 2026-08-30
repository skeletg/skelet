import { useMutation, useQuery } from '@apollo/client/react'
import { StarIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  ChannelDocument,
  ChannelsDocument,
  SetChannelAsDefaultDocument,
} from '@/features/channels/Channels.gql'
import { ChannelWithLocationsDocument } from '@/features/inventory/Inventory.gql'

// ChannelOverview renders the read-only informational view shown on the channel detail page (/channels/:id) - Purely a summary - no edit
// buttons, no links out - Operators who want to change anything go to /settings on their own and pick this channel in the header scope

export function ChannelOverview({ channelId }: { channelId: string }) {
  const { data, loading, error, refetch } = useQuery(ChannelDocument, {
    variables: { id: channelId },
    notifyOnNetworkStatusChange: true,
  })
  const channel = data?.channel

  const invQuery = useQuery(ChannelWithLocationsDocument, {
    variables: { id: channelId },
  })
  const inventoryLocations = invQuery.data?.channel.inventoryLocations ?? []

  const { has } = usePermissions()
  const canManage = has('channels.manage')
  const [setAsDefault, { loading: settingDefault }] = useMutation(SetChannelAsDefaultDocument, {
    // Promoting a channel demotes the previous default, so the whole list
    // has to come back, not just this channel
    refetchQueries: [
      { query: ChannelDocument, variables: { id: channelId } },
      { query: ChannelsDocument },
    ],
  })

  if (loading && !channel) {
    return (
      <DetailPageSection title="Overview" description="Loading channel…">
        <div />
      </DetailPageSection>
    )
  }

  if (error || !channel) {
    return (
      <DetailPageSection title="Overview">
        <LoadError
          title="Couldn't load channel"
          message={error?.message ?? 'Channel not found.'}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </DetailPageSection>
    )
  }

  return (
    <>
      <DetailPageSection title="Identity" description="Name, slug, currency, default locale, URL, status.">
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
          <Row label="Name" value={channel.name} />
          <Row label="Slug" value={<code className="font-mono text-xs">{channel.slug}</code>} />
          <Row label="Currency" value={channel.currencyCode} />
          <Row label="Default locale" value={channel.defaultLocale} />
          <Row
            label="Public URL"
            value={
              channel.publicUrl ? (
                <a
                  href={channel.publicUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="text-primary hover:underline"
                >
                  {channel.publicUrl}
                </a>
              ) : (
                <span className="text-muted-foreground">-</span>
              )
            }
          />
          <Row
            label="Status"
            value={
              <div className="flex items-center gap-2">
                {channel.isActive ? (
                  <Badge variant="secondary">Active</Badge>
                ) : (
                  <Badge variant="outline">Inactive</Badge>
                )}
                {channel.isDefault ? (
                  <Badge variant="default" className="gap-1">
                    <StarIcon className="size-3" />
                    Default
                  </Badge>
                ) : canManage ? (
                  // Exactly one channel is the default; promoting this one
                  // demotes whichever held it, so no "unset" action exists
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-7"
                    disabled={settingDefault}
                    onClick={() =>
                      void setAsDefault({ variables: { channelId: channel.id } })
                    }
                  >
                    <StarIcon className="size-3" />
                    {settingDefault ? 'Setting…' : 'Make default'}
                  </Button>
                ) : null}
              </div>
            }
          />
        </dl>
      </DetailPageSection>

      <DetailPageSection
        title="Locales"
        description={`${channel.locales.length} supported · default ${channel.defaultLocale}`}
      >
        {channel.locales.length === 0 ? (
          <p className="text-sm text-muted-foreground">No locales assigned.</p>
        ) : (
          <div className="flex flex-wrap gap-1">
            {channel.locales.map((l) => (
              <Badge
                key={l.code}
                variant={l.code === channel.defaultLocale ? 'default' : 'secondary'}
                className="font-mono font-normal"
                title={l.name}
              >
                {l.code}
                {l.code === channel.defaultLocale ? ' (default)' : ''}
              </Badge>
            ))}
          </div>
        )}
      </DetailPageSection>

      <DetailPageSection
        title="Inventory locations"
        description={`${inventoryLocations.length} assigned · ordered by priority`}
      >
        {invQuery.loading && inventoryLocations.length === 0 ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : inventoryLocations.length === 0 ? (
          <p className="text-sm text-muted-foreground">No locations assigned.</p>
        ) : (
          <ol className="grid gap-1 text-sm">
            {inventoryLocations
              .slice()
              .sort((a, b) => a.priority - b.priority)
              .map((entry, idx) => (
                <li key={entry.location.id} className="flex items-center gap-2">
                  <span className="w-6 text-right text-xs text-muted-foreground">
                    {idx + 1}.
                  </span>
                  <span>{entry.location.name}</span>
                  {!entry.location.isActive ? (
                    <Badge variant="outline" className="text-xs">
                      Inactive
                    </Badge>
                  ) : null}
                </li>
              ))}
          </ol>
        )}
      </DetailPageSection>

      <DetailPageSection
        title="Metadata"
        description={`${channel.metadata.length} entr${channel.metadata.length === 1 ? 'y' : 'ies'} · free-form key/value`}
      >
        {channel.metadata.length === 0 ? (
          <p className="text-sm text-muted-foreground">No metadata entries.</p>
        ) : (
          <dl className="grid grid-cols-1 gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
            {channel.metadata.map((e) => (
              <Row
                key={e.key}
                label={<code className="font-mono text-xs">{e.key}</code>}
                value={<span className="truncate">{e.value}</span>}
              />
            ))}
          </dl>
        )}
      </DetailPageSection>
    </>
  )
}

function Row({ label, value }: { label: React.ReactNode; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[max-content_1fr] items-baseline gap-3">
      <dt className="text-xs uppercase tracking-wider text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-all">{value}</dd>
    </div>
  )
}
