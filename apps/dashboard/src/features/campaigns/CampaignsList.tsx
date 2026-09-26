import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon, MailIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeletedBadge,
  ShowDeletedToggle,
  SoftDeleteRowAction,
} from '@/components/SoftDeleteActions'
import {
  DeleteEmailCampaignDocument,
  EmailCampaignsDocument,
  RestoreEmailCampaignDocument,
} from './Campaigns.gql'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  scheduled: 'secondary',
  sending: 'default',
  sent: 'default',
  cancelled: 'outline',
}

const AUDIENCE_LABEL: Record<string, string> = {
  all: 'All customers',
  group: 'Customer group',
  segment: 'Segment',
}

function fmtDate(iso?: string | null): string {
  if (!iso) return '-'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString()
}

type CampaignRow = {
  id: string
  name: string
  status: string
  audienceKind: string
  sentCount: number
  skippedCount: number
  totalRecipients: number
  sentAt?: string | null
  scheduledFor?: string | null
  deletedAt?: string | null
}

export function CampaignsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('campaigns.manage')
  const [showDeleted, setShowDeleted] = useState(false)
  const { data, loading, error, refetch } = useQuery(EmailCampaignsDocument, {
    variables: { channelId: effectiveChannelId ?? '', includeDeleted: showDeleted },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const [deleteCampaign] = useMutation(DeleteEmailCampaignDocument)
  const [restoreCampaign] = useMutation(RestoreEmailCampaignDocument)
  const campaigns = (data?.emailCampaigns ?? []) as CampaignRow[]

  const columns = useMemo<ColumnDef<CampaignRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/campaigns/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'status',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        cell: ({ row }) => (
          <Badge
            variant={STATUS_VARIANT[row.original.status] ?? 'outline'}
            className="capitalize"
          >
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'audienceKind',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Audience" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {AUDIENCE_LABEL[row.original.audienceKind] ?? row.original.audienceKind}
          </span>
        ),
      },
      {
        id: 'sent',
        header: 'Sent',
        enableSorting: false,
        cell: ({ row }) => {
          const c = row.original
          return (
            <span className="text-sm tabular-nums">
              {c.status === 'draft' || c.status === 'scheduled'
                ? '-'
                : `${c.sentCount}/${c.totalRecipients}${c.skippedCount ? ` · ${c.skippedCount} skipped` : ''}`}
            </span>
          )
        },
      },
      {
        id: 'scheduledOrSent',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title="Scheduled / sent" />
        ),
        accessorFn: (c) => c.sentAt ?? c.scheduledFor ?? '',
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {fmtDate(row.original.sentAt ?? row.original.scheduledFor)}
          </span>
        ),
      },
      {
        id: 'actions',
        enableSorting: false,
        header: () => null,
        cell: ({ row }) =>
          canManage ? (
            <div className="flex items-center justify-end gap-2">
              <DeletedBadge deletedAt={row.original.deletedAt} />
              <SoftDeleteRowAction
                deletedAt={row.original.deletedAt}
                entityLabel="campaign"
                name={row.original.name}
                consequence="Recipients, opens and clicks are kept, and a scheduled campaign stops going out. You can restore it later."
                onDelete={async () => {
                  await deleteCampaign({ variables: { id: row.original.id } })
                  await refetch()
                }}
                onRestore={async () => {
                  await restoreCampaign({ variables: { id: row.original.id } })
                  await refetch()
                }}
              />
            </div>
          ) : null,
      },
    ],
    [canManage, deleteCampaign, restoreCampaign, refetch],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Campaigns"
        badges={
          isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined
        }
        meta={
          <span className="text-xs text-muted-foreground">
            One-off and scheduled email broadcasts to your customers. Only customers opted in to
            marketing email receive them.
          </span>
        }
        actions={
          isChannel ? (
            <div className="flex items-center gap-4">
              <ShowDeletedToggle
                id="campaigns-show-deleted"
                checked={showDeleted}
                onCheckedChange={setShowDeleted}
              />
              {canManage && (
                <Button asChild size="sm">
                  <Link to="/campaigns/new">
                    <PlusIcon className="size-4" /> New campaign
                  </Link>
                </Button>
              )}
            </div>
          ) : undefined
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>
                Campaigns are sent per channel. Use the header scope picker to select a channel, then
                create a campaign.
              </AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection>
            <DataTable
              urlState={urlState}
              columns={columns}
              data={campaigns}
              loading={loading}
              error={error ? { message: error.message } : null}
              getRowId={(row) => row.id}
              searchPlaceholder="Search campaigns…"
              emptyState={
                <div className="py-8 text-center">
                  <MailIcon className="mx-auto mb-2 size-6 text-muted-foreground" />
                  <p className="text-sm text-muted-foreground">No campaigns yet.</p>
                  {canManage && (
                    <Button asChild variant="outline" size="sm" className="mt-3">
                      <Link to="/campaigns/new">Create your first campaign</Link>
                    </Button>
                  )}
                </div>
              }
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
