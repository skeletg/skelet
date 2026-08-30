import { useState } from 'react'
import { useLazyQuery, useMutation, useQuery } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DetailPageSection } from '@/components/layout/detail-page'
import { SectionNote } from './SectionNote'
import { formatCents } from '@/lib/format'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CreditMemosDocument,
  CreditableQuantitiesDocument,
  IssueCreditMemoDocument,
  OrderDocumentPdfDocument,
  OrderDocument,
  type OrderQuery,
} from './Orders.gql'

type OrderData = OrderQuery['order']

type LineDraft = { checked: boolean; quantity: string }

// OrderCreditMemosSection lists the refunds raised against an order and issues
// new ones. A memo here is NOT tied to an RMA - that path lives in
// OrderReturnsSection and writes its own memo when the return is refunded. This
// is the "refund without a return" case: goodwill, a pricing correction, a
// shipping credit
export function OrderCreditMemosSection({ order }: { order: OrderData }) {
  // Channel-scoped, matching the backend gate - a global has() check would offer the button on channels the admin can't refund on
  const canRefund = usePermissions().canAccessChannelScope('orders.refund', order.channelId)
  const [open, setOpen] = useState(false)
  const [lines, setLines] = useState<Record<string, LineDraft>>({})
  const [shippingRefund, setShippingRefund] = useState('')
  const [adjustmentRefund, setAdjustmentRefund] = useState('')
  const [adjustmentFee, setAdjustmentFee] = useState('')
  const [refundTarget, setRefundTarget] = useState('original')
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')

  const { data } = useQuery(CreditMemosDocument, { variables: { orderId: order.id } })
  const { data: creditable } = useQuery(CreditableQuantitiesDocument, {
    variables: { orderId: order.id },
    skip: !canRefund,
  })
  const [issue, { loading: issuing }] = useMutation(IssueCreditMemoDocument, {
    refetchQueries: [
      { query: CreditMemosDocument, variables: { orderId: order.id } },
      { query: CreditableQuantitiesDocument, variables: { orderId: order.id } },
      { query: OrderDocument, variables: { id: order.id } },
    ],
  })
  const [fetchPdf, { loading: downloading }] = useLazyQuery(OrderDocumentPdfDocument, {
    fetchPolicy: 'network-only',
  })

  const memos = data?.creditMemos ?? []
  // Nothing has been collected, so there is nothing refundable and the section would be a placeholder
  const collected =
    order.paymentTransactions.some((t) => t.kind === 'CAPTURE' && t.status === 'SUCCEEDED') ||
    order.giftCardAmount > 0 ||
    order.storeCreditAmount > 0
  const rmaNumberFor = (returnId: string) =>
    order.returns.find((r) => r.id === returnId)?.rmaNumber
  const remaining = new Map((creditable?.creditableQuantities ?? []).map((q) => [q.orderLineId, q.quantity]))
  const anyCreditable = [...remaining.values()].some((q) => q > 0)

  async function downloadMemo(memoId: string, number: string) {
    const res = await fetchPdf({
      variables: { orderId: order.id, kind: 'CREDIT_MEMO', creditMemoId: memoId },
    })
    if (!res.data) return
    const bytes = Uint8Array.from(atob(res.data.orderDocumentPdf), (c) => c.charCodeAt(0))
    const url = URL.createObjectURL(new Blob([bytes], { type: 'application/pdf' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${number}.pdf`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  function openDialog() {
    setLines(
      Object.fromEntries(order.lines.map((l) => [l.id, { checked: false, quantity: '' }])),
    )
    setShippingRefund('')
    setAdjustmentRefund('')
    setAdjustmentFee('')
    setRefundTarget('original')
    setReason('')
    setError('')
    setOpen(true)
  }

  const cents = (v: string) => Math.round(parseFloat(v || '0') * 100) || 0

  async function submit() {
    setError('')
    const picked = Object.entries(lines)
      .filter(([, d]) => d.checked && parseFloat(d.quantity) > 0)
      .map(([orderLineId, d]) => ({ orderLineId, quantity: parseFloat(d.quantity) }))
    const shipping = cents(shippingRefund)
    const adjustment = cents(adjustmentRefund)
    const fee = cents(adjustmentFee)
    if (picked.length === 0 && shipping === 0 && adjustment === 0) {
      setError('Pick at least one line, or enter a shipping or adjustment amount.')
      return
    }
    try {
      await issue({
        variables: {
          input: {
            orderId: order.id,
            lines: picked,
            shippingRefund: shipping,
            adjustmentRefund: adjustment,
            adjustmentFee: fee,
            refundTarget,
            reason,
          },
        },
      })
      setOpen(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not issue the credit memo.')
    }
  }

  return (
    <DetailPageSection
      title="Credit memos"
      description="Every refund raised against this order. Memos marked with an RMA are the return above, not a second refund."
      trailing={
        canRefund ? (
          <Button size="sm" variant="outline" disabled={!collected} onClick={openDialog}>
            Issue credit memo
          </Button>
        ) : undefined
      }
    >
      {memos.length === 0 && !collected && (
        <SectionNote>
          Nothing has been collected on this order yet, so there is nothing to refund - record the
          payment first.
        </SectionNote>
      )}
      {memos.length === 0 ? (
        collected ? (
          <p className="text-sm text-muted-foreground">No refunds have been issued on this order.</p>
        ) : null
      ) : (
        <ul className="divide-y rounded-md border">
          {memos.map((m) => (
            <li key={m.id} className="flex items-start justify-between gap-4 p-3">
              <div className="min-w-0 space-y-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium tabular-nums">{m.number}</span>
                  {m.returnId && (
                    <Badge variant="secondary">
                      {rmaNumberFor(m.returnId) ? `Settles ${rmaNumberFor(m.returnId)}` : 'Return'}
                    </Badge>
                  )}
                  {m.offline && <Badge variant="outline">Offline</Badge>}
                  {m.refundTarget === 'gift_card' && <Badge variant="outline">Gift card</Badge>}
                </div>
                <div className="text-sm text-muted-foreground">
                  {new Date(m.createdAt).toLocaleDateString()}
                  {m.lines.length > 0 && ` - ${m.lines.length} line${m.lines.length === 1 ? '' : 's'}`}
                  {m.reason && ` - ${m.reason}`}
                </div>
                {m.lines.length > 0 && (
                  <ul className="text-sm text-muted-foreground">
                    {m.lines.map((l) => (
                      <li key={l.orderLineId} className="tabular-nums">
                        {l.quantity} x {l.productName}
                        <span className="text-xs"> ({l.productSku})</span>
                        {' - '}
                        {formatCents(l.totalAmount, order.currencyCode)}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-3">
                <span className="font-semibold tabular-nums">
                  {formatCents(m.total, order.currencyCode)}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={downloading}
                  onClick={() => downloadMemo(m.id, m.number)}
                >
                  PDF
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Issue a credit memo</DialogTitle>
            <DialogDescription>
              Refunds to the order's payment method, or as a gift card. Orders paid outside a
              gateway (cash on delivery, bank transfer) are recorded here without a gateway call.
            </DialogDescription>
          </DialogHeader>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Could not issue</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-4">
            {anyCreditable ? (
              <div className="divide-y rounded-md border">
                {order.lines.map((l) => {
                  const left = remaining.get(l.id) ?? 0
                  if (left <= 0) return null
                  const draft = lines[l.id] ?? { checked: false, quantity: '' }
                  return (
                    <div key={l.id} className="flex items-center gap-3 p-2">
                      <Checkbox
                        checked={draft.checked}
                        onCheckedChange={(checked) =>
                          setLines((s) => ({
                            ...s,
                            [l.id]: {
                              checked: checked === true,
                              quantity: checked === true ? String(left) : '',
                            },
                          }))
                        }
                      />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm">{l.productName}</div>
                        <div className="text-xs text-muted-foreground">
                          {l.productSku} - {left} creditable
                        </div>
                      </div>
                      <Input
                        className="w-24"
                        type="number"
                        min="0"
                        max={left}
                        step="any"
                        disabled={!draft.checked}
                        value={draft.quantity}
                        onChange={(e) =>
                          setLines((s) => ({ ...s, [l.id]: { ...draft, quantity: e.target.value } }))
                        }
                      />
                    </div>
                  )
                })}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                Every line has already been credited. A memo can still refund shipping or an
                adjustment.
              </p>
            )}

            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1">
                <Label htmlFor="cm-shipping">Shipping refund</Label>
                <Input
                  id="cm-shipping"
                  type="number"
                  min="0"
                  step="0.01"
                  value={shippingRefund}
                  onChange={(e) => setShippingRefund(e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="cm-adjustment">Adjustment</Label>
                <Input
                  id="cm-adjustment"
                  type="number"
                  min="0"
                  step="0.01"
                  value={adjustmentRefund}
                  onChange={(e) => setAdjustmentRefund(e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="cm-fee">Fee</Label>
                <Input
                  id="cm-fee"
                  type="number"
                  min="0"
                  step="0.01"
                  value={adjustmentFee}
                  onChange={(e) => setAdjustmentFee(e.target.value)}
                />
              </div>
            </div>

            <div className="space-y-1">
              <Label>Refund to</Label>
              <Select value={refundTarget} onValueChange={setRefundTarget}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="original">Original payment method</SelectItem>
                  <SelectItem value="gift_card">New gift card</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1">
              <Label htmlFor="cm-reason">Reason</Label>
              <Input
                id="cm-reason"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="Shown on the memo"
              />
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button disabled={issuing} onClick={submit}>
              {issuing ? 'Issuing...' : 'Issue credit memo'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DetailPageSection>
  )
}
