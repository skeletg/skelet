import { useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { CreateFulfillmentDocument, OrderDocument, type OrderQuery } from './Orders.gql'

type OrderData = OrderQuery['order']
type LineDraft = { checked: boolean; quantity: string }

// CreateShipmentDialog records a partial shipment: a per-line quantity picker
// (defaulting to each line's still-unshipped remainder) plus optional carrier +
// tracking - Backed by createFulfillment; the order settles to partially_fulfilled
// or fulfilled - The one-click "Fulfill remaining" header action stays for the
// common ship-everything case
export function CreateShipmentDialog({
  order,
  open,
  onOpenChange,
}: {
  order: OrderData
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const refetch = [{ query: OrderDocument, variables: { id: order.id } }, 'OrdersPaged']
  const [createFulfillment, state] = useMutation(CreateFulfillmentDocument, { refetchQueries: refetch })

  // Remaining = ordered − already-shipped (non-cancelled fulfillments)
  const remainingByLine = useMemo(() => {
    const shipped = new Map<string, number>()
    for (const f of order.fulfillments) {
      if (f.status === 'cancelled') continue
      for (const l of f.lines) shipped.set(l.orderLineId, (shipped.get(l.orderLineId) ?? 0) + l.quantity)
    }
    const out = new Map<string, number>()
    for (const l of order.lines) out.set(l.id, l.quantity - (shipped.get(l.id) ?? 0))
    return out
  }, [order.fulfillments, order.lines])

  const shippableLines = order.lines.filter((l) => (remainingByLine.get(l.id) ?? 0) > 0)

  const [drafts, setDrafts] = useState<Record<string, LineDraft>>({})
  const [carrier, setCarrier] = useState('')
  const [tracking, setTracking] = useState('')
  const [insured, setInsured] = useState('')
  const [error, setError] = useState<string | null>(null)

  // Initialize drafts when the dialog opens (keyed remount via `open` prop from
  // the parent isn't available, so seed on the open transition)
  function seed() {
    setError(null)
    setCarrier('')
    setTracking('')
    setInsured('')
    setDrafts(
      Object.fromEntries(
        shippableLines.map((l) => [l.id, { checked: true, quantity: String(remainingByLine.get(l.id) ?? 0) }]),
      ),
    )
  }

  const selected = order.lines
    .filter((l) => drafts[l.id]?.checked)
    .map((l) => ({ orderLineId: l.id, quantity: Number(drafts[l.id]?.quantity ?? '0') }))
    .filter((l) => l.quantity > 0)

  async function onSubmit() {
    if (selected.length === 0) {
      setError('Select at least one line with a positive quantity.')
      return
    }
    setError(null)
    try {
      await createFulfillment({
        variables: {
          input: {
            orderId: order.id,
            lines: selected.map((l) => ({ orderLineId: l.orderLineId, quantity: l.quantity })),
            carrier: carrier.trim() || null,
            trackingNumber: tracking.trim() || null,
            insuredValueCents: insured.trim() ? Math.round(Number(insured) * 100) : null,
          },
        },
      })
      onOpenChange(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create shipment.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (v) seed()
        onOpenChange(v)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Create shipment</DialogTitle>
          <DialogDescription>
            Ship a subset of the order. Quantities default to what's still unshipped per line.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-2">
            {shippableLines.map((l) => {
              const remaining = remainingByLine.get(l.id) ?? 0
              const draft = drafts[l.id] ?? { checked: true, quantity: String(remaining) }
              return (
                <div key={l.id} className="flex items-center gap-3 rounded border px-3 py-2">
                  <Checkbox
                    checked={draft.checked}
                    onCheckedChange={(v) => setDrafts((p) => ({ ...p, [l.id]: { ...draft, checked: !!v } }))}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{l.productName}</div>
                    <div className="font-mono text-[10px] text-muted-foreground">
                      {l.productSku} · {remaining} unshipped
                    </div>
                  </div>
                  <Input
                    type="number"
                    min={0}
                    max={remaining}
                    step="any"
                    value={draft.quantity}
                    disabled={!draft.checked}
                    onChange={(e) => setDrafts((p) => ({ ...p, [l.id]: { ...draft, quantity: e.target.value } }))}
                    className="w-24 tabular-nums"
                  />
                </div>
              )
            })}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="ship-carrier">Carrier (optional)</Label>
              <Input id="ship-carrier" value={carrier} onChange={(e) => setCarrier(e.target.value)} placeholder="USPS, FedEx…" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ship-tracking">Tracking (optional)</Label>
              <Input id="ship-tracking" value={tracking} onChange={(e) => setTracking(e.target.value)} placeholder="1Z999…" />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="ship-insured">Insured value (optional)</Label>
            <Input
              id="ship-insured"
              type="number"
              min={0}
              step="0.01"
              value={insured}
              onChange={(e) => setInsured(e.target.value)}
              placeholder="Declared value for shipment insurance"
              className="tabular-nums"
            />
          </div>
          {error && (
            <Alert variant="destructive">
              <AlertTitle>Failed</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={state.loading}>
            Cancel
          </Button>
          <Button type="button" onClick={onSubmit} disabled={state.loading || selected.length === 0}>
            {state.loading ? 'Shipping…' : 'Create shipment'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
