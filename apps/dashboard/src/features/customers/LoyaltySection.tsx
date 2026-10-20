import { useMutation, useQuery } from '@apollo/client/react'
import { useState, type FormEvent } from 'react'
import { type ColumnDef } from '@/components/data-table/tanstack'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { AdjustLoyaltyPointsDocument, LoyaltyDocument } from './Customers.gql'
import { useCustomerAccess } from './useCustomerAccess'

// Past this a typed acknowledgement is required - an extra keystroke beats granting 500k points
const CONFIRM_ABOVE_POINTS = 100_000

type PointsEntry = {
  id: string
  points: number
  reason: string
  createdAt: unknown
}

const columns: ColumnDef<PointsEntry>[] = [
  {
    accessorKey: 'createdAt',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Date" />,
    size: 190,
    cell: ({ row }) => (
      <span className="whitespace-nowrap text-muted-foreground">
        {new Date(row.original.createdAt as string).toLocaleString()}
      </span>
    ),
  },
  {
    accessorKey: 'reason',
    header: 'Reason',
    cell: ({ row }) => row.original.reason || '-',
  },
  {
    id: 'points',
    accessorFn: (row) => row.points,
    header: () => <div className="text-right">Points</div>,
    size: 140,
    cell: ({ row }) => (
      <div
        className={`text-right tabular-nums ${row.original.points < 0 ? 'text-destructive' : 'text-success'}`}
      >
        {row.original.points > 0 ? '+' : ''}
        {row.original.points.toLocaleString()}
      </div>
    ),
  },
]

export function LoyaltySection({ customerId }: { customerId: string }) {
  const { canManage } = useCustomerAccess()
  const { data, loading, error, refetch } = useQuery(LoyaltyDocument, {
    variables: { customerId },
  })
  const [adjusting, setAdjusting] = useState(false)

  const loyalty = data?.loyalty
  if (loading && !loyalty) {
    return <p className="text-sm text-muted-foreground">Loading…</p>
  }
  // A failed load used to return null too, so the section vanished as if the customer had none
  if (error && !loyalty) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn&apos;t load loyalty</AlertTitle>
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }
  if (!loyalty) return null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-2xl font-semibold tabular-nums">
            {loyalty.balancePoints.toLocaleString()}{' '}
            <span className="text-base font-normal text-muted-foreground">
              points
            </span>
          </p>
          <p className="text-xs text-muted-foreground">Current balance</p>
        </div>
        {canManage && (
          <Button variant="outline" onClick={() => setAdjusting(true)}>
            Adjust points
          </Button>
        )}
      </div>

      {loyalty.entries.length > 0 && (
        <div className="grid gap-1">
          <DataTable
            columns={columns}
            data={loyalty.entries as PointsEntry[]}
            getRowId={(row) => row.id}
            pageSize={10}
            emptyState="No points movements yet."
          />
          <p className="text-xs text-muted-foreground">
            Showing the latest {loyalty.entries.length} movements (the API caps this at 50) - the
            balance above covers the full history.
          </p>
        </div>
      )}

      <AdjustLoyaltyDialog
        customerId={customerId}
        open={adjusting}
        onClose={() => setAdjusting(false)}
        onAdjusted={() => void refetch()}
      />
    </div>
  )
}

function AdjustLoyaltyDialog({
  customerId,
  open,
  onClose,
  onAdjusted,
}: {
  customerId: string
  open: boolean
  onClose: () => void
  onAdjusted: () => void
}) {
  const [points, setPoints] = useState('')
  const [reason, setReason] = useState('')
  const [typed, setTyped] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [adjust, { loading }] = useMutation(AdjustLoyaltyPointsDocument)

  const parsed = Number.parseInt(points, 10)
  const n = Number.isNaN(parsed) ? null : parsed
  const needsAck = n != null && Math.abs(n) >= CONFIRM_ABOVE_POINTS

  function reset() {
    setPoints('')
    setReason('')
    setTyped('')
    setSubmitError(null)
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    if (n == null || n === 0) {
      setSubmitError('Enter a non-zero whole number (negative to deduct).')
      return
    }
    if (!reason.trim()) {
      setSubmitError('A reason is required - it lands in the points ledger.')
      return
    }
    if (needsAck && typed !== 'adjust') {
      setSubmitError('Type "adjust" to confirm an amount this large.')
      return
    }
    try {
      await adjust({ variables: { customerId, points: n, reason: reason.trim() } })
      reset()
      onAdjusted()
      onClose()
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Adjustment failed.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          reset()
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Adjust loyalty points</DialogTitle>
          <DialogDescription>
            Positive grants points, negative deducts (deductions cannot exceed
            the balance). Recorded in the customer&apos;s points ledger.
          </DialogDescription>
        </DialogHeader>
        <form id="adjust-loyalty" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="lp-adjust-points">Points (+/−)</Label>
            <Input
              id="lp-adjust-points"
              required
              type="number"
              step="1"
              placeholder="500"
              value={points}
              onChange={(e) => setPoints(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="lp-adjust-reason">Reason</Label>
            <Input
              id="lp-adjust-reason"
              required
              placeholder="Promotion bonus, correction, …"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
          {needsAck && n != null && (
            <div className="grid gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <p className="text-sm">
                This {n > 0 ? 'grants' : 'deducts'}{' '}
                <strong className="tabular-nums">{Math.abs(n).toLocaleString()}</strong> points.
                Type <strong>adjust</strong> to confirm.
              </p>
              <Input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="adjust" />
            </div>
          )}
          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Couldn&apos;t adjust</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            form="adjust-loyalty"
            disabled={loading || !reason.trim() || (needsAck && typed !== 'adjust')}
          >
            {loading ? 'Saving…' : 'Apply adjustment'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
