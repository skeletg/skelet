import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { AuditLogDocument } from './Audit.gql'

// Read-only admin audit trail: every admin mutation, with redacted
// variables - Server-side pagination; newest first

type Row = {
  id: string
  adminEmail: string
  action: string
  detail: unknown
  ip: string
  success: boolean
  durationMs: number
  createdAt: string
}

const PAGE = 50

export function AuditLogPage() {
  const [actionFilter, setActionFilter] = useState('')
  const [applied, setApplied] = useState('')
  const [page, setPage] = useState(0)
  const { data, loading, error } = useQuery(AuditLogDocument, {
    variables: { action: applied || null, limit: PAGE, offset: page * PAGE },
    fetchPolicy: 'cache-and-network',
  })

  const rows = useMemo<Row[]>(
    () =>
      (data?.auditLog.entries ?? []).map((e) => ({
        id: e.id,
        adminEmail: e.adminEmail || '(unauthenticated)',
        action: e.action,
        detail: e.detail,
        ip: e.ip,
        success: e.success,
        durationMs: e.durationMs,
        createdAt: e.createdAt as string,
      })),
    [data],
  )
  const total = data?.auditLog.total ?? 0

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="When" />,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-sm text-muted-foreground">
            {new Date(row.original.createdAt).toLocaleString()}
          </span>
        ),
      },
      {
        accessorKey: 'adminEmail',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Admin" />,
        cell: ({ row }) => <span className="text-sm">{row.original.adminEmail}</span>,
      },
      {
        accessorKey: 'action',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Action" />,
        cell: ({ row }) => <code className="font-mono text-xs">{row.original.action}</code>,
      },
      {
        accessorKey: 'success',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Result" />,
        cell: ({ row }) =>
          row.original.success ? (
            <Badge variant="outline" className="text-emerald-700 dark:text-emerald-300">ok</Badge>
          ) : (
            <Badge variant="outline" className="text-red-700 dark:text-red-300">failed</Badge>
          ),
      },
      {
        accessorKey: 'ip',
        header: ({ column }) => <DataTableColumnHeader column={column} title="IP" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.ip}</span>,
      },
      {
        id: 'detail',
        header: 'Variables',
        cell: ({ row }) =>
          row.original.detail ? (
            <details className="max-w-[380px]">
              <summary className="cursor-pointer text-xs text-muted-foreground">view</summary>
              <pre className="mt-1 max-h-48 overflow-auto rounded bg-muted/50 p-2 text-[11px]">
                {JSON.stringify(row.original.detail, null, 2)}
              </pre>
            </details>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Audit log"
        meta={<span>{total} recorded mutation(s) - variables stored with secrets redacted.</span>}
      />
      <DetailPageBody>
        <DetailPageSection>
          {error ? (
            <Alert variant="destructive">
              <AlertTitle>Couldn't load the audit log</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          ) : (
            <div className="grid gap-3">
              <form
                className="flex gap-2"
                onSubmit={(e) => {
                  e.preventDefault()
                  setPage(0)
                  setApplied(actionFilter.trim())
                }}
              >
                <Input
                  className="max-w-xs"
                  placeholder="Filter by action (exact, e.g. UpdateProduct)"
                  value={actionFilter}
                  onChange={(e) => setActionFilter(e.target.value)}
                />
                <Button type="submit" variant="outline" size="sm" className="h-9">
                  Filter
                </Button>
              </form>
              <DataTable columns={columns} data={rows} loading={loading} getRowId={(r) => r.id} emptyState="No audit entries yet." />
              <div className="flex items-center justify-between text-sm text-muted-foreground">
                <span>
                  Page {page + 1} of {Math.max(1, Math.ceil(total / PAGE))}
                </span>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={(page + 1) * PAGE >= total}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Next
                  </Button>
                </div>
              </div>
            </div>
          )}
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}
