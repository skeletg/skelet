import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon } from 'lucide-react'
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
import { SignupFormsDocument } from './Forms.gql'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  active: 'default',
}

const TYPE_LABEL: Record<string, string> = {
  popup: 'Popup',
  inline: 'Inline',
  flyout: 'Flyout',
}

export function FormsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { data, loading, error } = useQuery(SignupFormsDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const forms = data?.signupForms ?? []

  type FormRow = (typeof forms)[number]

  const columns = useMemo<ColumnDef<FormRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/forms/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'type',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Type" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {TYPE_LABEL[row.original.type] ?? row.original.type}
          </span>
        ),
      },
      {
        accessorKey: 'status',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        cell: ({ row }) => (
          <Badge variant={STATUS_VARIANT[row.original.status] ?? 'outline'} className="capitalize">
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'submissionCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Signups" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground tabular-nums">
            {row.original.submissionCount.toLocaleString()} signups
          </span>
        ),
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Signup forms"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Popups, flyouts and embedded forms that grow your list. Capture an email, then hand off
            to a welcome flow.
          </span>
        }
        actions={
          isChannel ? (
            <Button asChild size="sm">
              <Link to="/forms/new">
                <PlusIcon className="size-4" /> New form
              </Link>
            </Button>
          ) : undefined
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>Signup forms are per channel - select one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection>
            <DataTable
              urlState={urlState}
              columns={columns}
              data={forms}
              loading={loading}
              error={error ? { message: error.message } : null}
              getRowId={(r) => r.id}
              emptyState="No signup forms yet."
              searchPlaceholder="Search forms…"
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
