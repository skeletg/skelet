import { useMutation, useQuery } from '@apollo/client/react'
import { useMemo, useState, type FormEvent } from 'react'
import { type ColumnDef } from '@/components/data-table/tanstack'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
import { formatCents } from '@/lib/format'

import { AdjustStoreCreditDocument, StoreCreditDocument } from './Customers.gql'
import { useCustomerAccess } from './useCustomerAccess'

// Anything this size or bigger is more likely a fat finger than a real goodwill credit, so it
// needs a typed acknowledgement on top of the dialog
const CONFIRM_ABOVE_CENTS = 100_000

// majorToCents converts a user-entered amount in major units (e.g. "50.00")
// to integer cents, rejecting unparseable input
function majorToCents(v: string): number | null {
  const n = Number.parseFloat(v)
  if (Number.isNaN(n)) return null
  return Math.round(n * 100)
}

type LedgerEntry = {
  id: string
  amountCents: number
  currencyCode: string
  reason: string
  createdAt: unknown
}

export function StoreCreditSection({ customerId }: { customerId: string }) {
  const { canManage } = useCustomerAccess()
  const { data, loading, refetch } = useQuery(StoreCreditDocument, {
    variables: { customerId },
  })
  const [adjusting, setAdjusting] = useState(false)

  const credit = data?.storeCredit
  const balanceCurrency = credit?.currencyCode ?? ''

  const columns = useMemo<ColumnDef<LedgerEntry>[]>(
    () => [
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
        id: 'amount',
        accessorFn: (row) => row.amountCents,
        header: () => <div className="text-right">Amount</div>,
        size: 180,
        cell: ({ row }) => {
          const e = row.original
          return (
            <div className="flex items-center justify-end gap-1.5">
              {/* A mixed-currency history would otherwise read as if it summed into the balance */}
              {e.currencyCode !== balanceCurrency && (
                <Badge variant="outline" className="font-mono text-[10px]">
                  {e.currencyCode}
                </Badge>
              )}
              <span
                className={`tabular-nums ${e.amountCents < 0 ? 'text-destructive' : 'text-emerald-600 dark:text-emerald-400'}`}
              >
                {e.amountCents > 0 ? '+' : ''}
                {formatCents(e.amountCents, e.currencyCode)}
              </span>
            </div>
          )
        },
      },
    ],
    [balanceCurrency],
  )

  if (loading && !credit) {
    return <p className="text-sm text-muted-foreground">Loading…</p>
  }
  if (!credit) return null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-2xl font-semibold tabular-nums">
            {formatCents(credit.balanceCents, credit.currencyCode)}
          </p>
          <p className="text-xs text-muted-foreground">
            Current balance in {credit.currencyCode}, summed over the full history
          </p>
        </div>
        {canManage && (
          <Button variant="outline" onClick={() => setAdjusting(true)}>
            Adjust balance
          </Button>
        )}
      </div>

      {credit.entries.length > 0 && (
        <div className="grid gap-1">
          <DataTable
            columns={columns}
            data={credit.entries as LedgerEntry[]}
            getRowId={(row) => row.id}
            pageSize={10}
            emptyState="No credit movements yet."
          />
          <p className="text-xs text-muted-foreground">
            Showing the latest {credit.entries.length} movements (the API caps this at 50) - the
            balance above covers the full history.
          </p>
        </div>
      )}

      <AdjustStoreCreditDialog
        customerId={customerId}
        currencyCode={credit.currencyCode}
        open={adjusting}
        onClose={() => setAdjusting(false)}
        onAdjusted={() => void refetch()}
      />
    </div>
  )
}

function AdjustStoreCreditDialog({
  customerId,
  currencyCode,
  open,
  onClose,
  onAdjusted,
}: {
  customerId: string
  currencyCode: string
  open: boolean
  onClose: () => void
  onAdjusted: () => void
}) {
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [typed, setTyped] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [adjust, { loading }] = useMutation(AdjustStoreCreditDocument)

  const cents = majorToCents(amount)
  const needsAck = cents != null && Math.abs(cents) >= CONFIRM_ABOVE_CENTS

  function reset() {
    setAmount('')
    setReason('')
    setTyped('')
    setSubmitError(null)
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    if (cents == null || cents === 0) {
      setSubmitError('Enter a non-zero amount (negative to deduct).')
      return
    }
    if (!reason.trim()) {
      setSubmitError('A reason is required - it lands in the ledger.')
      return
    }
    if (needsAck && typed !== 'adjust') {
      setSubmitError('Type "adjust" to confirm an amount this large.')
      return
    }
    try {
      await adjust({
        variables: { customerId, amountCents: cents, reason: reason.trim() },
      })
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
          <DialogTitle>Adjust store credit</DialogTitle>
          <DialogDescription>
            Positive grants credit, negative deducts (deductions cannot exceed
            the balance). Recorded in the customer&apos;s ledger.
          </DialogDescription>
        </DialogHeader>
        <form id="adjust-store-credit" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="sc-adjust-amount">Amount (+/−)</Label>
            <Input
              id="sc-adjust-amount"
              required
              type="number"
              step="0.01"
              placeholder="25.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="sc-adjust-reason">Reason</Label>
            <Input
              id="sc-adjust-reason"
              required
              placeholder="Goodwill, refund to credit, …"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
          {needsAck && cents != null && (
            <div className="grid gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <p className="text-sm">
                This {cents > 0 ? 'grants' : 'deducts'}{' '}
                <strong className="tabular-nums">{formatCents(Math.abs(cents), currencyCode)}</strong>.
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
            form="adjust-store-credit"
            disabled={loading || !reason.trim() || (needsAck && typed !== 'adjust')}
          >
            {loading ? 'Saving…' : 'Apply adjustment'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
