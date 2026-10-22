import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { ShieldOffIcon, PlusIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  EmailSuppressionsDocument,
  SuppressEmailDocument,
  UnsuppressEmailDocument,
} from './Deliverability.gql'
import { formatDate } from '@/lib/format'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

// Suppression is account-wide (not channel-scoped): a hard bounce or complaint
// blocks an address everywhere, so this page has no channel picker
const REASON_LABELS: Record<string, string> = {
  hard_bounce: 'Hard bounce',
  soft_bounce: 'Soft bounce',
  complaint: 'Spam complaint',
  manual: 'Manual',
  unsubscribe: 'Unsubscribe',
}

const REASON_TONE: Record<string, string> = {
  hard_bounce: 'bg-destructive-soft text-destructive',
  complaint: 'bg-destructive-soft text-destructive',
  soft_bounce: 'bg-warning-soft text-warning',
  manual: 'bg-muted text-muted-foreground',
  unsubscribe: 'bg-info-soft text-info',
}

type SuppressionRow = {
  id: string
  email: string
  reason: string
  detail?: string | null
  softBounces: number
  createdAt?: string | null
}

export function SuppressionList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const [reason, setReason] = useState('')
  const [newEmail, setNewEmail] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [confirmingRemove, setConfirmingRemove] = useState<string | null>(null)

  const { data, loading, error, refetch } = useQuery(EmailSuppressionsDocument, {
    variables: { reason: reason || null, limit: 100, offset: 0 },
    fetchPolicy: 'cache-and-network',
  })
  const [suppress, { loading: adding }] = useMutation(SuppressEmailDocument)
  const [unsuppress] = useMutation(UnsuppressEmailDocument)

  const page = data?.emailSuppressions
  const items = (page?.items ?? []) as SuppressionRow[]

  const add = async () => {
    const email = newEmail.trim()
    if (!email) return
    setErr(null)
    try {
      await suppress({ variables: { email, note: 'added by operator' } })
      setNewEmail('')
      await refetch()
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to add')
    }
  }

  // Removing an entry lets marketing mail reach an address that bounced, complained or opted out - so it asks first
  async function onConfirmRemove() {
    if (!confirmingRemove) return
    await unsuppress({ variables: { email: confirmingRemove } })
    await refetch()
  }

  const columns = useMemo<ColumnDef<SuppressionRow>[]>(
    () => [
      {
        accessorKey: 'email',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Address" />,
        cell: ({ row }) => <span className="font-medium">{row.original.email}</span>,
      },
      {
        accessorKey: 'reason',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Reason" />,
        cell: ({ row }) => {
          const s = row.original
          return (
            <span
              className={`rounded-full px-2 py-0.5 text-xs font-medium ${REASON_TONE[s.reason] ?? 'bg-muted text-muted-foreground'}`}
            >
              {REASON_LABELS[s.reason] ?? s.reason}
              {s.reason === 'soft_bounce' && s.softBounces > 0 ? ` ×${s.softBounces}` : ''}
            </span>
          )
        },
      },
      {
        accessorKey: 'detail',
        header: 'Detail',
        enableSorting: false,
        cell: ({ row }) => (
          <span className="block max-w-xs truncate text-xs text-muted-foreground">
            {row.original.detail || '-'}
          </span>
        ),
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Since" />,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{formatDate(row.original.createdAt)}</span>
        ),
      },
      {
        id: 'actions',
        size: 80,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <div className="text-right">
            <Button variant="ghost" size="sm" onClick={() => setConfirmingRemove(row.original.email)}>
              Remove
            </Button>
          </div>
        ),
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Suppression list"
        badges={page ? <Badge variant="secondary">{page.totalCount} suppressed</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Addresses that never receive marketing email - hard bounces and spam complaints are added
            automatically; you can add or remove entries by hand. Transactional mail (order receipts,
            password reset) is never suppressed.
          </span>
        }
      />
      <DetailPageBody>
        {(error || err) && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't complete that</AlertTitle>
            <AlertDescription>{err ?? error?.message}</AlertDescription>
          </Alert>
        )}
        <DetailPageSection>
          <div className="flex flex-wrap items-end gap-2">
            <div className="flex items-end gap-2">
              <Input
                type="email"
                placeholder="block an address…"
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && void add()}
                className="w-64"
              />
              <Button className="h-10" size="sm" onClick={() => void add()} disabled={adding || !newEmail.trim()}>
                <PlusIcon className="size-4" /> Suppress
              </Button>
            </div>
            <div className="ml-auto">
              <Select value={reason || 'all'} onValueChange={(v) => setReason(v === 'all' ? '' : v)}>
                <SelectTrigger className="w-44">
                  <SelectValue placeholder="All reasons" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All reasons</SelectItem>
                  {Object.entries(REASON_LABELS).map(([k, v]) => (
                    <SelectItem key={k} value={k}>
                      {v}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection>
          <DataTable
            urlState={urlState}
            columns={columns}
            data={items}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(r) => r.id}
            searchPlaceholder="Search address…"
            emptyState={
              <div className="rounded-lg border border-dashed px-4 py-12 text-center">
                <ShieldOffIcon className="mx-auto mb-2 size-6 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">
                  No suppressed addresses. Your list is clean.
                </p>
              </div>
            }
          />
        </DetailPageSection>
        <ConfirmDialog
          open={!!confirmingRemove}
          onOpenChange={(open) => !open && setConfirmingRemove(null)}
          title={`Remove ${confirmingRemove ?? 'this address'} from the suppression list?`}
          description="Marketing email can reach this address again. If it bounced, complained or unsubscribed, sending to it can hurt your sender reputation or break consent rules."
          confirmLabel="Remove from list"
          destructive
          onConfirm={onConfirmRemove}
        />
      </DetailPageBody>
    </DetailPageLayout>
  )
}
