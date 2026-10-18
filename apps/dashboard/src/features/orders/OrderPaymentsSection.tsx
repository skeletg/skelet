import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { formatCents, fromMinor, toMinor } from '@/lib/format'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  OfflinePaymentMethodsDocument,
  CapturePaymentDocument,
  RefundPaymentDocument,
  VoidPaymentDocument,
  RecordOfflinePaymentDocument,
  OrderDocument,
  type OrderQuery,
  OrdersPagedDocument,
} from './Orders.gql'
import { paymentRemaining } from './payment-remaining'

type OrderData = OrderQuery['order']

// OrderPaymentsSection shows the gateway payment ledger and the capture / refund
// / void actions for an order - These are gateway-level money movements (distinct
// from RMA-driven refunds in the Returns section) - All gated on orders.update +
// a present gateway authorization; the backend re-checks + guards amounts
export function OrderPaymentsSection({ order }: { order: OrderData }) {
  const permissions = usePermissions()
  const canManage = permissions.canAccessChannelScope('orders.update', order.channelId)
  const refetch = [{ query: OrderDocument, variables: { id: order.id } }, OrdersPagedDocument]

  const [capture, captureState] = useMutation(CapturePaymentDocument, { refetchQueries: refetch })
  const [refund, refundState] = useMutation(RefundPaymentDocument, { refetchQueries: refetch })
  const [voidPayment, voidState] = useMutation(VoidPaymentDocument, { refetchQueries: refetch })
  const [recordOffline, recordState] = useMutation(RecordOfflinePaymentDocument, { refetchQueries: refetch })
  // The offline providers registered for this channel - the same list the storefront offers at checkout. A role without
  // payments.view gets nothing back, so the dialog falls back to a free-text field rather than breaking
  const { data: methodData } = useQuery(OfflinePaymentMethodsDocument, {
    variables: { channelId: order.channelId },
    errorPolicy: 'all',
  })
  const offlineMethods = (methodData?.paymentMethods ?? []).filter((m) => m.isOffline)
  // Default to how the customer said they would pay, when that is an offline method
  const offlineDefault =
    offlineMethods.find((m) => m.providerSlug === order.paymentProvider)?.providerSlug ??
    offlineMethods[0]?.providerSlug ??
    ''

  const [dialog, setDialog] = useState<'capture' | 'refund' | 'record' | null>(null)
  const [confirmVoid, setConfirmVoid] = useState(false)
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [reference, setReference] = useState('')
  // Offline payments carry more than a reference: a part amount, how it arrived, and
  // when it actually landed, which is rarely the day someone gets round to recording it
  const [method, setMethod] = useState('')
  const [receivedAt, setReceivedAt] = useState('')
  const [payNote, setPayNote] = useState('')
  const [recordAmount, setRecordAmount] = useState('')
  const [error, setError] = useState<string | null>(null)

  const ps = order.paymentStatus as string
  const hasGateway = order.paymentProvider !== '' && order.paymentReference !== ''
  const canCapture = hasGateway && (ps === 'AUTHORIZED' || ps === 'PARTIALLY_CAPTURED')
  const canRefund = hasGateway && (ps === 'PAID' || ps === 'PARTIALLY_CAPTURED' || ps === 'PARTIALLY_REFUNDED')
  const canVoid = hasGateway && ps === 'AUTHORIZED'
  // Offline / manual reconciliation: any unpaid order (cash on delivery, bank
  // transfer, manual/draft order) can be marked paid without a gateway
  const canRecordOffline = canManage && ps === 'UNPAID'
  const busy = captureState.loading || refundState.loading || voidState.loading || recordState.loading

  // Typed in major units like every other money field - "12.50" is 1250 cents, never truncated to 12
  const amountCents = toMinor(amount, order.currencyCode)
  const amountValid = amount.trim() === '' || (amountCents !== null && amountCents > 0)

  function openDialog(kind: 'capture' | 'refund') {
    setError(null)
    setAmount('')
    setReason('')
    setDialog(kind)
  }

  async function onSubmit() {
    if (!amountValid) {
      setError('Enter a positive amount, or leave it blank for the full remaining.')
      return
    }
    setError(null)
    try {
      if (dialog === 'capture') {
        await capture({ variables: { orderId: order.id, amount: amountCents } })
      } else {
        await refund({ variables: { orderId: order.id, amount: amountCents, reason: reason.trim() || null } })
      }
      setDialog(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Action failed.')
    }
  }

  async function onVoid() {
    setError(null)
    try {
      await voidPayment({ variables: { orderId: order.id } })
      setConfirmVoid(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Void failed.')
    }
  }

  async function onRecord() {
    setError(null)
    try {
      const cents = recordAmount.trim() === '' ? null : Math.round(parseFloat(recordAmount) * 100)
      if (cents !== null && (!Number.isFinite(cents) || cents <= 0)) {
        setError('Enter a positive amount, or leave it blank for the full balance.')
        return
      }
      await recordOffline({
        variables: {
          input: {
            orderId: order.id,
            amount: cents,
            method: method || offlineDefault || 'other',
            reference: reference.trim() || null,
            receivedAt: receivedAt ? new Date(receivedAt).toISOString() : null,
            note: payNote.trim() || null,
          },
        },
      })
      setDialog(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not record payment.')
    }
  }

  const txns = order.paymentTransactions
  const amountDue = order.total - order.giftCardAmount

  return (
    <DetailPageSection
      title="Payments"
      description="Every movement of money on this order, and the actions that create one."
      trailing={
        canManage && (hasGateway || canRecordOffline) ? (
          <div className="flex flex-wrap items-center gap-2">
            {canRecordOffline && (
              <Button
                size="sm"
                disabled={busy}
                onClick={() => {
                  setError(null)
                  setReference('')
                  setMethod(offlineDefault)
                  setReceivedAt('')
                  setPayNote('')
                  setRecordAmount('')
                  setDialog('record')
                }}
              >
                Record payment
              </Button>
            )}
            {canCapture && (
              <Button size="sm" disabled={busy} onClick={() => openDialog('capture')}>
                Capture
              </Button>
            )}
            {canRefund && (
              <Button size="sm" variant="outline" disabled={busy} onClick={() => openDialog('refund')}>
                Refund
              </Button>
            )}
            {canVoid && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => { setError(null); setConfirmVoid(true) }}
              >
                Void
              </Button>
            )}
          </div>
        ) : undefined
      }
    >
      {error && !dialog && !confirmVoid && (
        <Alert variant="destructive">
          <AlertTitle>Payment action failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {!hasGateway ? (
        <p className="text-sm text-muted-foreground">
          {ps === 'UNPAID'
            ? 'Awaiting an offline payment - cash on delivery, bank transfer, pay on pickup. Record it here once the funds land.'
            : 'No gateway payment on this order - it was covered by gift card or store credit.'}
        </p>
      ) : txns.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No payment movements yet.
        </p>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kind</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Amount</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead className="text-right">When</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {[...txns]
                .sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime())
                .map((t) => (
                  <TableRow key={t.id}>
                    <TableCell className="font-medium capitalize">{t.kind}</TableCell>
                    <TableCell className="text-xs uppercase tracking-wide text-muted-foreground">
                      {t.status}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatCents(t.amount, order.currencyCode)}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{t.reason || '-'}</TableCell>
                    <TableCell className="text-right text-xs text-muted-foreground">
                      {new Date(t.createdAt).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={dialog !== null} onOpenChange={(v) => { if (!v) setDialog(null) }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {dialog === 'capture'
                ? 'Capture payment'
                : dialog === 'refund'
                  ? 'Refund payment'
                  : 'Record offline payment'}
            </DialogTitle>
            <DialogDescription>
              {dialog === 'record'
                ? `Money taken outside a gateway - cash on delivery, bank transfer, cheque, pay on pickup. Record part of the balance and the order stays partially paid until the rest lands.`
                : `Leave the amount blank to ${dialog === 'capture' ? 'capture the full authorized remaining' : 'refund the full captured remaining'}. Amount due: ${formatCents(amountDue, order.currencyCode)}.`}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            {dialog === 'record' ? (
              <>
                <div className="grid grid-cols-2 gap-3 items-start">
                  <div className="grid gap-2">
                    <Label htmlFor="pay-record-amount">Amount</Label>
                    <Input
                      id="pay-record-amount"
                      inputMode="decimal"
                      value={recordAmount}
                      onChange={(e) => setRecordAmount(e.target.value)}
                      placeholder={(amountDue / 100).toFixed(2)}
                    />
                    <p className="text-xs text-muted-foreground">
                      Blank records the full {formatCents(amountDue, order.currencyCode)} owed
                    </p>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="pay-method">How it arrived</Label>
                    {offlineMethods.length > 0 ? (
                      <select
                        id="pay-method"
                        value={method || offlineDefault}
                        onChange={(e) => setMethod(e.target.value)}
                        className="h-9 rounded-md border bg-transparent px-3 text-sm"
                      >
                        {offlineMethods.map((m) => (
                          <option key={m.providerSlug} value={m.providerSlug}>
                            {m.displayName}
                          </option>
                        ))}
                        <option value="other">Other</option>
                      </select>
                    ) : (
                      <Input
                        id="pay-method"
                        value={method}
                        onChange={(e) => setMethod(e.target.value)}
                        placeholder="bank-transfer, cod…"
                      />
                    )}
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="grid gap-2">
                    <Label htmlFor="pay-reference">Reference</Label>
                    <Input
                      id="pay-reference"
                      value={reference}
                      onChange={(e) => setReference(e.target.value)}
                      placeholder="wire #, receipt no.…"
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="pay-received">Received on</Label>
                    <Input
                      id="pay-received"
                      type="date"
                      value={receivedAt}
                      onChange={(e) => setReceivedAt(e.target.value)}
                    />
                  </div>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="pay-note">Note (optional)</Label>
                  <Input
                    id="pay-note"
                    value={payNote}
                    onChange={(e) => setPayNote(e.target.value)}
                    placeholder="anything the next person should know"
                  />
                </div>
              </>
            ) : (
              <>
                <div className="grid gap-2">
                  <Label htmlFor="pay-amount">Amount</Label>
                  <Input
                    id="pay-amount"
                    inputMode="decimal"
                    value={amount}
                    onChange={(e) => setAmount(e.target.value)}
                    placeholder={fromMinor(paymentRemaining(dialog === 'refund' ? 'refund' : 'capture', txns), order.currencyCode)}
                  />
                  {amount.trim() !== '' && amountValid && (
                    <p className="text-xs text-muted-foreground">
                      = {formatCents(amountCents as number, order.currencyCode)}
                    </p>
                  )}
                </div>
                {dialog === 'refund' && (
                  <div className="grid gap-2">
                    <Label htmlFor="pay-reason">Reason (optional)</Label>
                    <Input
                      id="pay-reason"
                      value={reason}
                      onChange={(e) => setReason(e.target.value)}
                      placeholder="customer request, damaged…"
                    />
                  </div>
                )}
              </>
            )}
            {error && (
              <Alert variant="destructive">
                <AlertTitle>Failed</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDialog(null)} disabled={busy}>
              Cancel
            </Button>
            {dialog === 'record' ? (
              <Button type="button" onClick={onRecord} disabled={busy}>
                {busy ? 'Working…' : 'Record payment'}
              </Button>
            ) : (
              <Button type="button" onClick={onSubmit} disabled={busy || !amountValid}>
                {busy ? 'Working…' : dialog === 'capture' ? 'Capture' : 'Refund'}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={confirmVoid} onOpenChange={(v) => { if (!v) setConfirmVoid(false) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Void authorization?</AlertDialogTitle>
            <AlertDialogDescription>
              This releases the uncaptured authorization with the gateway. It can't be undone - the
              customer would need to be re-charged.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {error && (
            <Alert variant="destructive">
              <AlertTitle>Void failed</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={voidState.loading}>Keep authorization</AlertDialogCancel>
            <AlertDialogAction disabled={voidState.loading} onClick={(e) => { e.preventDefault(); onVoid() }}>
              {voidState.loading ? 'Voiding…' : 'Void'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageSection>
  )
}
