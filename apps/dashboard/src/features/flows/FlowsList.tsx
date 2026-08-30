import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon, WorkflowIcon } from 'lucide-react'
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
import { DeleteFlowDocument, FlowsDocument, RestoreFlowDocument } from './Flows.gql'
import { TRIGGER_LABELS } from './flowTypes'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  active: 'default',
  paused: 'secondary',
  archived: 'outline',
}

type FlowRow = {
  id: string
  name: string
  triggerType: string
  status: string
  deletedAt?: string | null
}

export function FlowsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('flows.manage')
  const [showDeleted, setShowDeleted] = useState(false)
  const { data, loading, error, refetch } = useQuery(FlowsDocument, {
    variables: { channelId: effectiveChannelId ?? '', includeDeleted: showDeleted },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const [deleteFlow] = useMutation(DeleteFlowDocument)
  const [restoreFlow] = useMutation(RestoreFlowDocument)
  const flows = (data?.flows ?? []) as FlowRow[]

  const columns = useMemo<ColumnDef<FlowRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/flows/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'triggerType',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Trigger" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {TRIGGER_LABELS[row.original.triggerType] ?? row.original.triggerType}
          </span>
        ),
      },
      {
        accessorKey: 'status',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Badge variant={STATUS_VARIANT[row.original.status] ?? 'outline'} className="capitalize">
              {row.original.status}
            </Badge>
            <DeletedBadge deletedAt={row.original.deletedAt} />
          </div>
        ),
      },
      {
        id: 'actions',
        enableSorting: false,
        header: () => null,
        cell: ({ row }) =>
          canManage ? (
            <div className="flex justify-end">
              <SoftDeleteRowAction
                deletedAt={row.original.deletedAt}
                entityLabel="flow"
                name={row.original.name}
                consequence="The flow stops enrolling anyone new. Its runs and their history are kept, and you can restore it later."
                onDelete={async () => {
                  await deleteFlow({ variables: { id: row.original.id } })
                  await refetch()
                }}
                onRestore={async () => {
                  await restoreFlow({ variables: { id: row.original.id } })
                  await refetch()
                }}
              />
            </div>
          ) : null,
      },
    ],
    [canManage, deleteFlow, restoreFlow, refetch],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Flows"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Automated, event-triggered email sequences (welcome series, post-purchase, …). A customer
            enters when the trigger fires and moves through the steps over time.
          </span>
        }
        actions={
          isChannel ? (
            <div className="flex items-center gap-4">
              <ShowDeletedToggle
                id="flows-show-deleted"
                checked={showDeleted}
                onCheckedChange={setShowDeleted}
              />
              <Button asChild size="sm">
                <Link to="/flows/new">
                  <PlusIcon className="size-4" /> New flow
                </Link>
              </Button>
            </div>
          ) : undefined
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>Flows are per channel - select one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection>
            <DataTable
              urlState={urlState}
              columns={columns}
              data={flows}
              loading={loading}
              error={error ? { message: error.message } : null}
              getRowId={(r) => r.id}
              searchPlaceholder="Search flows…"
              emptyState={
                <div className="py-8 text-center">
                  <WorkflowIcon className="mx-auto mb-2 size-6 text-muted-foreground" />
                  <p className="text-sm text-muted-foreground">No flows yet.</p>
                  <Button asChild variant="outline" size="sm" className="mt-3">
                    <Link to="/flows/new">Build your first flow</Link>
                  </Button>
                </div>
              }
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
