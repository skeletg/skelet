import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import { toComponent } from '@/features/blocks/model'
import type { BlockComponent } from '@/features/email/blocks'
import { EmailBlocksDocument } from '@/features/email/Email.gql'

// The "Blocks" tab: the reusable email-block library (plain snippets and parameterized blocks together), rendered with the shared DataTable
// - Detail editing stays at /emails/blocks/:slug; the "New block" action lives in the EmailsHub page header
export function BlocksTab({ channelId, isChannel }: { channelId: string; isChannel: boolean }) {
  const { has } = usePermissions()
  const canManage = has('emails.manage')
  const { data, loading, error } = useQuery(EmailBlocksDocument, {
    variables: { channelId },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const blocks = useMemo(() => (data?.emailBlocks ?? []).map(toComponent), [data])

  const columns = useMemo<ColumnDef<BlockComponent>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/emails/blocks/${row.original.slug}`} className="font-medium hover:underline">
            {row.original.icon ? `${row.original.icon} ` : ''}
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
        id: 'inputs',
        accessorFn: (b) => b.fields.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Inputs" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{row.original.fields.length || '-'}</span>
        ),
      },
      {
        id: 'body',
        enableSorting: false,
        header: () => <span>Body</span>,
        cell: ({ row }) =>
          row.original.htmlTemplate ? (
            <Badge variant="outline">HTML</Badge>
          ) : (
            <span className="text-sm text-muted-foreground">{row.original.blocks.length} blocks</span>
          ),
      },
    ],
    [],
  )

  if (!isChannel) {
    return (
      <DetailPageSection>
        <Alert>
          <AlertTitle>Pick a channel</AlertTitle>
          <AlertDescription>Blocks are per channel - select one in the header scope picker.</AlertDescription>
        </Alert>
      </DetailPageSection>
    )
  }

  return (
    <DetailPageSection>
      <DataTable
        columns={columns}
        data={blocks}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(b) => b.slug}
        searchPlaceholder="Search name or slug…"
        emptyState={
          <div className="text-center">
            <p className="text-sm text-muted-foreground">No blocks yet.</p>
            {canManage && (
              <Button asChild variant="outline" size="sm" className="mt-3">
                <Link to="/emails/blocks/new">Create your first block</Link>
              </Button>
            )}
          </div>
        }
      />
    </DetailPageSection>
  )
}
