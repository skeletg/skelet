import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { CheckIcon, CopyIcon, ExternalLinkIcon } from 'lucide-react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { DataTable } from '@/components/data-table'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import { PURPOSE_LABELS, TRACKING_DOCS_URL, eventLabel } from './platformGuides'
import { destinationStatus, installCommand, platformStatus, type Destination, type Platform } from './status'
import { TrackingOverviewDocument } from './Tracking.gql'

// Every platform shop events can go to, with where each stands on this channel, and the destinations set up
export function TrackingPage() {
  const navigate = useNavigate()
  const { effectiveChannelId, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('tracking.manage')
  const { data, loading, error, refetch } = useQuery(TrackingOverviewDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
  })
  const platforms = data?.trackingPlatforms ?? []
  const destinations = useMemo(() => data?.trackingDestinations ?? [], [data])
  const byKind = useMemo(() => {
    const m = new Map<string, Destination[]>()
    for (const d of destinations) m.set(d.kind, [...(m.get(d.kind) ?? []), d])
    return m
  }, [destinations])
  const labelOf = (kind: string) => platforms.find((p) => p.kind === kind)?.label ?? kind

  const platformColumns = useMemo<ColumnDef<Platform>[]>(
    () => [
      {
        id: 'platform',
        header: 'Platform',
        cell: ({ row }) => (
          <div className="grid">
            <span className={row.original.installed ? 'font-medium' : 'text-muted-foreground'}>{row.original.label}</span>
            {row.original.defaultEvents.length > 0 && (
              <span className="text-xs text-muted-foreground">{row.original.defaultEvents.map(eventLabel).join(', ')}</span>
            )}
          </div>
        ),
      },
      {
        id: 'consent',
        header: 'Needs consent',
        cell: ({ row }) => (
          <div className="flex flex-wrap gap-1">
            {row.original.purposes.map((p) => (
              <Badge key={p} variant="outline">{PURPOSE_LABELS[p] ?? p}</Badge>
            ))}
          </div>
        ),
      },
      {
        id: 'status',
        header: 'Status',
        cell: ({ row }) => {
          const s = platformStatus(row.original, byKind.get(row.original.kind) ?? [])
          return <Badge variant={s.variant}>{s.label}</Badge>
        },
      },
      {
        id: 'action',
        header: '',
        cell: ({ row }) => <PlatformAction platform={row.original} count={byKind.get(row.original.kind)?.length ?? 0} canManage={canManage} />,
      },
    ],
    [byKind, canManage],
  )

  const destinationColumns = useMemo<ColumnDef<Destination>[]>(
    () => [
      { accessorKey: 'name', header: 'Name' },
      { id: 'platform', header: 'Platform', cell: ({ row }) => labelOf(row.original.kind) },
      {
        id: 'status',
        header: 'Status',
        cell: ({ row }) => {
          const s = destinationStatus(row.original)
          return <Badge variant={s.variant}>{s.label}</Badge>
        },
      },
      {
        id: 'sent',
        header: 'Last sent',
        cell: ({ row }) => (row.original.health.lastSentAt ? new Date(row.original.health.lastSentAt).toLocaleString() : 'Never'),
      },
      {
        id: 'failed',
        header: 'Failed (24h)',
        cell: ({ row }) => (row.original.health.dead24h > 0 ? <span className="text-destructive">{row.original.health.dead24h}</span> : '0'),
      },
    ],
    [platforms], // eslint-disable-line react-hooks/exhaustive-deps
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Tracking"
        meta="Shop events go from the backend straight to each platform - no tracking script slows the storefront, and each visitor's cookie choice is respected."
        actions={
          <Button asChild variant="outline" size="sm">
            <a href={TRACKING_DOCS_URL} target="_blank" rel="noreferrer">
              Guide <ExternalLinkIcon className="size-3" />
            </a>
          </Button>
        }
      />
      <DetailPageBody>
        {!effectiveChannelId ? (
          <p className="p-6 text-sm text-muted-foreground">Pick a channel in the header - every destination belongs to one.</p>
        ) : error ? (
          <LoadError title="Could not load tracking" message={error.message} onRetry={() => void refetch()} retrying={loading} />
        ) : (
          <>
            <DetailPageSection title="Your destinations" description={selectedChannel ? `Set up on ${selectedChannel.name}.` : undefined}>
              <DataTable<Destination>
                compact
                columns={destinationColumns}
                data={destinations}
                loading={loading && !data}
                getRowId={(d) => d.id}
                onRowClick={(d) => navigate(`/tracking/${d.id}`)}
                emptyState="Nothing set up yet - pick a platform below."
              />
            </DetailPageSection>
            <DetailPageSection
              title="Platforms"
              description="Each one is a plugin. Ones not in this build are listed with the command that adds them; the backend needs a rebuild after."
            >
              <DataTable<Platform> compact columns={platformColumns} data={platforms} loading={loading && !data} getRowId={(p) => p.kind} />
            </DetailPageSection>
            <DetailPageSection title="Consent">
              <p className="max-w-2xl text-sm text-muted-foreground">
                The cookie banner and consent mode are under{' '}
                <Link to="/privacy" className="underline underline-offset-4">Settings → Privacy</Link>, with which destination each choice feeds.
              </p>
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function PlatformAction({ platform, count, canManage }: { platform: Platform; count: number; canManage: boolean }) {
  const [copied, setCopied] = useState(false)
  if (!platform.installed) {
    const command = installCommand(platform.pluginSlug)
    return (
      <Button
        variant="ghost"
        size="sm"
        className="h-7 gap-1.5 font-mono text-xs"
        title="Install the plugin, then rebuild the backend"
        onClick={async (e) => {
          e.stopPropagation()
          try {
            await navigator.clipboard.writeText(command)
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
          } catch {
            // the command stays visible to copy by hand
          }
        }}
      >
        {copied ? <CheckIcon className="size-3" /> : <CopyIcon className="size-3" />}
        {command}
      </Button>
    )
  }
  if (!canManage || !platform.availableOnChannel) return null
  return (
    <Button asChild variant={count === 0 ? 'default' : 'outline'} size="sm" className="h-7 text-xs">
      <Link to={`/tracking/new/${platform.kind}`}>{count === 0 ? 'Set up' : 'Add another'}</Link>
    </Button>
  )
}
