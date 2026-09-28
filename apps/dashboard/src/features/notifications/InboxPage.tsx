import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import {
  MarkAllNotificationsReadDocument,
  MarkNotificationReadDocument,
  NotificationUnreadCountDocument,
  NotificationsDocument,
  type NotificationsQuery,
} from './Notifications.gql'
import { CATEGORY_LABEL, NOTIFICATION_CATEGORIES } from './preferences'

type Row = NotificationsQuery['notifications'][number]

const SEVERITY: Record<string, { label: string; cls: string }> = {
  critical: { label: 'Critical', cls: 'bg-destructive-soft text-destructive' },
  warning: { label: 'Warning', cls: 'bg-warning-soft text-warning' },
  info: { label: 'Info', cls: 'bg-info-soft text-info' },
}

function fmtWhen(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

// InboxPage is the full admin notification center - a catalog-style table of
// everything the operator may see, filterable by severity and category
export function InboxPage() {
  const navigate = useNavigate()
  const { data, loading, refetch } = useQuery(NotificationsDocument, {
    variables: { limit: 200 },
    fetchPolicy: 'cache-and-network',
  })
  const [markRead] = useMutation(MarkNotificationReadDocument)
  const [markAll] = useMutation(MarkAllNotificationsReadDocument)
  const rows = data?.notifications ?? []

  const open = async (n: Row) => {
    if (!n.read) await markRead({ variables: { id: n.id }, refetchQueries: [NotificationUnreadCountDocument] }).catch(() => {})
    if (n.actionUrl) navigate(n.actionUrl)
    else void refetch()
  }
  const onMarkAll = async () => {
    await markAll({ refetchQueries: [NotificationUnreadCountDocument] }).catch(() => {})
    void refetch()
  }

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'severity',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Severity" />,
        filterFn: (row, _id, value: string[]) => !value?.length || value.includes(row.original.severity),
        cell: ({ row }) => {
          const s = SEVERITY[row.original.severity] ?? { label: row.original.severity, cls: 'bg-muted' }
          return <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${s.cls}`}>{s.label}</span>
        },
      },
      {
        id: 'title',
        accessorFn: (r) => r.title,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Notification" />,
        cell: ({ row }) => {
          const n = row.original
          return (
            <div className="min-w-0 max-w-[520px]">
              <p className={`truncate ${n.read ? 'text-muted-foreground' : 'font-medium'}`}>
                {!n.read && <span className="mr-1.5 inline-block size-1.5 rounded-full bg-primary align-middle" />}
                {n.title}
              </p>
              {n.body && <p className="truncate text-xs text-muted-foreground">{n.body}</p>}
            </div>
          )
        },
      },
      {
        accessorKey: 'category',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Category" />,
        filterFn: (row, _id, value: string[]) => !value?.length || value.includes(row.original.category),
        cell: ({ row }) => <Badge variant="secondary">{CATEGORY_LABEL[row.original.category] ?? row.original.category}</Badge>,
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="When" />,
        cell: ({ row }) => <span className="whitespace-nowrap text-sm text-muted-foreground">{fmtWhen(row.original.createdAt)}</span>,
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) => (
          <button onClick={() => void open(row.original)} className="text-xs font-medium text-primary hover:underline">
            Open →
          </button>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Inbox"
        meta={<span className="text-xs text-muted-foreground">Store-ops notifications - things needing your attention</span>}
        actions={
          <Button variant="outline" size="sm" onClick={onMarkAll}>
            Mark all read
          </Button>
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable
            columns={columns}
            data={rows}
            loading={loading}
            getRowId={(n) => n.id}
            searchPlaceholder="Search notifications…"
            filters={[
              {
                column: 'severity',
                title: 'Severity',
                options: [
                  { label: 'Critical', value: 'critical' },
                  { label: 'Warning', value: 'warning' },
                  { label: 'Info', value: 'info' },
                ],
              },
              {
                column: 'category',
                title: 'Category',
                // the categories anything actually posts, same list Preferences offers to mute
                options: NOTIFICATION_CATEGORIES.map((c) => ({ label: c.label, value: c.id })),
              },
            ]}
            initialSorting={[{ id: 'createdAt', desc: true }]}
            emptyState={<p className="py-8 text-center text-sm text-muted-foreground">You&apos;re all caught up 🎉</p>}
          />
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}
