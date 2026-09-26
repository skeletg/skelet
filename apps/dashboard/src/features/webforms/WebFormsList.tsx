import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
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
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeletedBadge,
  ShowDeletedToggle,
  SoftDeleteRowAction,
} from '@/components/SoftDeleteActions'
import {
  DeleteWebFormDocument,
  RestoreWebFormDocument,
  WebFormsDocument,
} from './WebForms.gql'
import type { Field } from './types'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  active: 'default',
}

function fieldCount(fields: unknown): number {
  if (!Array.isArray(fields)) return 0
  return (fields as Field[]).filter((f) => f && f.type && !['heading', 'paragraph', 'divider', 'section'].includes(f.type)).length
}

export function WebFormsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { has } = usePermissions()
  const canManage = has('webforms.manage')
  const [showDeleted, setShowDeleted] = useState(false)
  const { data, loading, error, refetch } = useQuery(WebFormsDocument, {
    variables: { channelId: effectiveChannelId ?? '', includeDeleted: showDeleted },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const [deleteForm] = useMutation(DeleteWebFormDocument)
  const [restoreForm] = useMutation(RestoreWebFormDocument)
  const forms = data?.webForms ?? []

  type FormRow = (typeof forms)[number]

  const columns = useMemo<ColumnDef<FormRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/cms/forms/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <code className="font-mono text-xs text-muted-foreground">{row.original.slug}</code>,
      },
      {
        id: 'fields',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Fields" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground tabular-nums">{fieldCount(row.original.fields)}</span>
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
        header: ({ column }) => <DataTableColumnHeader column={column} title="Submissions" />,
        cell: ({ row }) => (
          <Link
            to={`/cms/forms/${row.original.id}/submissions`}
            className="text-sm text-muted-foreground tabular-nums hover:underline"
          >
            {row.original.submissionCount.toLocaleString()}
          </Link>
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
                entityLabel="form"
                name={row.original.name}
                consequence="The storefront stops serving the form, but every submission it already collected stays readable. You can restore it later."
                onDelete={async () => {
                  await deleteForm({ variables: { id: row.original.id } })
                  await refetch()
                }}
                onRestore={async () => {
                  await restoreForm({ variables: { id: row.original.id } })
                  await refetch()
                }}
              />
            </div>
          ) : null,
      },
    ],
    [canManage, deleteForm, restoreForm, refetch],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Forms"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Custom forms with conditional fields, placed on any page with the Form block. Submissions
            land here.
          </span>
        }
        actions={
          isChannel ? (
            <div className="flex items-center gap-4">
              <ShowDeletedToggle
                id="forms-show-deleted"
                checked={showDeleted}
                onCheckedChange={setShowDeleted}
              />
              {canManage && (
                <Button asChild size="sm">
                  <Link to="/cms/forms/new">
                    <PlusIcon className="size-4" /> New form
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
              <AlertDescription>Forms are per channel - select one in the header scope picker.</AlertDescription>
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
              emptyState="No forms yet."
              searchPlaceholder="Search forms…"
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
