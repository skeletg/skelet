import { useMemo, useState, useId } from 'react'
import { useMutation } from '@apollo/client/react'

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
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { SectionNote } from './SectionNote'
import { formatCents } from '@/lib/format'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  RequestReturnDocument,
  ApproveReturnDocument,
  RejectReturnDocument,
  ReceiveReturnDocument,
  IssueReturnRefundDocument,
  OrderDocument,
  type OrderQuery,
  OrdersPagedDocument,
} from './Orders.gql'

type OrderData = OrderQuery['order']

type LineDraft = { checked: boolean; quantity: string }

// OrderReturnsSection lists RMAs and drives their lifecycle (request → approve → receive → refund, or reject) - Refund
// here is RMA-driven (issueReturnRefund: original tender or gift card), distinct from the gateway actions in Payments
export function OrderReturnsSection({ order }: { order: OrderData }) {
  const fid = useId()
  const permissions = usePermissions()
  const canUpdate = permissions.canAccessChannelScope('orders.update', order.channelId)
  const canRefund = permissions.canAccessChannelScope('orders.refund', order.channelId)
  const refetch = [{ query: OrderDocument, variables: { id: order.id } }, OrdersPagedDocument]

  const [requestReturn, requestState] = useMutation(RequestReturnDocument, { refetchQueries: refetch })
  const [approveReturn] = useMutation(ApproveReturnDocument, { refetchQueries: refetch })
  const [rejectReturn] = useMutation(RejectReturnDocument, { refetchQueries: refetch })
  const [receiveReturn] = useMutation(ReceiveReturnDocument, { refetchQueries: refetch })
  const [issueRefund, refundState] = useMutation(IssueReturnRefundDocument, { refetchQueries: refetch })

  const [error, setError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  // New-return dialog state
  const [open, setOpen] = useState(false)
  const [lineDrafts, setLineDrafts] = useState<Record<string, LineDraft>>({})
  const [reason, setReason] = useState('')
  const [restock, setRestock] = useState(true)
  const [refundTarget, setRefundTarget] = useState<'original' | 'gift_card'>('original')

  const nameOf = (lineId: string) => order.lines.find((l) => l.id === lineId)?.name ?? lineId

  // Only shipped orders can be returned (backend enforces too)
  const shippable = order.status === 'FULFILLED' || order.status === 'PARTIALLY_FULFILLED'

  function openDialog() {
    setError(null)
    setReason('')
    setRestock(true)
    setRefundTarget('original')
    setLineDrafts(
      Object.fromEntries(order.lines.map((l) => [l.id, { checked: false, quantity: String(l.quantity) }])),
    )
    setOpen(true)
  }

  const selectedLines = useMemo(
    () =>
      order.lines
        .filter((l) => lineDrafts[l.id]?.checked)
        .map((l) => ({ orderLineId: l.id, quantity: Number(lineDrafts[l.id]?.quantity ?? '0') }))
        .filter((l) => l.quantity > 0),
    [order.lines, lineDrafts],
  )

  async function onCreate() {
    if (selectedLines.length === 0) {
      setError('Select at least one line with a positive quantity.')
      return
    }
    setError(null)
    try {
      await requestReturn({
        variables: {
          input: {
            orderId: order.id,
            lines: selectedLines.map((l) => ({ orderLineId: l.orderLineId, quantity: l.quantity })),
            reason: reason.trim() || null,
            restock,
            refundTarget,
          },
        },
      })
      setOpen(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to open return.')
    }
  }

  async function runReturnAction(id: string, fn: () => Promise<unknown>) {
    setError(null)
    setBusyId(id)
    try {
      await fn()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Action failed.')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <DetailPageSection
      title="Returns"
      description="Goods coming back. Approve, receive, then refund - which raises a credit memo below."
      trailing={
        canUpdate ? (
          <Button size="sm" variant="outline" disabled={!shippable} onClick={openDialog}>
            New return
          </Button>
        ) : undefined
      }
    >
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Return action failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {order.returns.length === 0 && !shippable && (
        <SectionNote>
          Returns open once something has shipped - there is nothing to send back yet.
        </SectionNote>
      )}
      {order.returns.length === 0 ? (
        shippable ? <p className="text-sm text-muted-foreground">No returns yet.</p> : null
      ) : (
        <div className="grid gap-3">
          {[...order.returns]
            .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
            .map((r) => {
              const busy = busyId === r.id
              return (
                <div key={r.id} className="rounded-md border p-4 text-sm">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2 font-medium">
                      <span className="font-mono">{r.rmaNumber}</span>
                      <Badge variant="outline" className="text-[10px] uppercase">{r.status}</Badge>
                      {r.refundAmount > 0 && (
                        <span className="text-xs font-normal text-muted-foreground">
                          refunded {formatCents(r.refundAmount, order.currencyCode)} · {r.refundTarget}
                        </span>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      {r.status === 'requested' && canUpdate && (
                        <>
                          <Button size="sm" disabled={busy} onClick={() => runReturnAction(r.id, () => approveReturn({ variables: { id: r.id } }))}>
                            Approve
                          </Button>
                          <Button size="sm" variant="outline" disabled={busy} onClick={() => runReturnAction(r.id, () => rejectReturn({ variables: { id: r.id } }))}>
                            Reject
                          </Button>
                        </>
                      )}
                      {r.status === 'approved' && canUpdate && (
                        <Button size="sm" disabled={busy} onClick={() => runReturnAction(r.id, () => receiveReturn({ variables: { id: r.id } }))}>
                          Mark received{r.restock ? ' + restock' : ''}
                        </Button>
                      )}
                      {r.status === 'received' && canRefund && (
                        <Button size="sm" disabled={busy || refundState.loading} onClick={() => runReturnAction(r.id, () => issueRefund({ variables: { id: r.id, reason: null } }))}>
                          {refundState.loading ? 'Refunding…' : `Issue refund (${r.refundTarget})`}
                        </Button>
                      )}
                    </div>
                  </div>
                  <ul className="mt-2 space-y-0.5 text-xs text-muted-foreground">
                    {r.lines.map((l) => (
                      <li key={l.id}>
                        {nameOf(l.orderLineId)} × <span className="tabular-nums">{l.quantity}</span>
                        {' · '}
                        {formatCents(l.refundUnitCents, order.currencyCode)}/unit
                      </li>
                    ))}
                  </ul>
                  {r.reason && <p className="mt-1 text-xs text-muted-foreground">Reason: {r.reason}</p>}
                </div>
              )
            })}
        </div>
      )}

      <Dialog open={open} onOpenChange={(v) => { if (!v) setOpen(false) }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Open a return</DialogTitle>
            <DialogDescription>
              Select the shipped items to return. Quantities can't exceed what shipped minus what's
              already returned (enforced on save).
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-2">
              {order.lines.map((l) => {
                const draft = lineDrafts[l.id] ?? { checked: false, quantity: String(l.quantity) }
                return (
                  <div key={l.id} className="flex items-center gap-3 rounded border px-3 py-2">
                    <Checkbox
                      checked={draft.checked}
                      onCheckedChange={(v) =>
                        setLineDrafts((prev) => ({ ...prev, [l.id]: { ...draft, checked: !!v } }))
                      }
                    />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium">{l.name}</div>
                      <div className="font-mono text-[10px] text-muted-foreground">{l.sku}</div>
                    </div>
                    <Input
                      type="number"
                      min={0}
                      max={l.quantity}
                      step="any"
                      value={draft.quantity}
                      disabled={!draft.checked}
                      onChange={(e) =>
                        setLineDrafts((prev) => ({ ...prev, [l.id]: { ...draft, quantity: e.target.value } }))
                      }
                      className="w-24 tabular-nums"
                    />
                  </div>
                )
              })}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ret-reason">Reason (optional)</Label>
              <Input id="ret-reason" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="defective, wrong item…" />
            </div>
            <div className="flex items-center justify-between gap-3">
              <div className="grid gap-1">
                <Label htmlFor={`${fid}-refund-to`} className="text-xs">Refund to</Label>
                <Select value={refundTarget} onValueChange={(v) => setRefundTarget(v as 'original' | 'gift_card')}>
                  <SelectTrigger id={`${fid}-refund-to`} className="w-44"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="original">Original payment</SelectItem>
                    <SelectItem value="gift_card">Gift card</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-2">
                <Switch id="ret-restock" checked={restock} onCheckedChange={setRestock} />
                <Label htmlFor="ret-restock" className="cursor-pointer text-sm">Restock received items</Label>
              </div>
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertTitle>Failed</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={requestState.loading}>
              Cancel
            </Button>
            <Button type="button" onClick={onCreate} disabled={requestState.loading || selectedLines.length === 0}>
              {requestState.loading ? 'Opening…' : 'Open return'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DetailPageSection>
  )
}
