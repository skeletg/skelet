import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { PlusIcon, Trash2Icon, Undo2Icon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { EditOrderDocument, OrderDocument, type OrderQuery } from './Orders.gql'

type OrderData = OrderQuery['order']
type AddRow = { sku: string; quantity: string }

// EditOrderDialog edits a pre-fulfillment order: change line quantities, remove
// lines, and add simple-product lines by SKU - Backed by editOrder, which
// recomputes totals + re-reserves stock (a shortage aborts) server-side - Bundle
// line quantities can't be changed - the backend rejects that and we surface it
export function EditOrderDialog({
  order,
  open,
  onOpenChange,
}: {
  order: OrderData
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const refetch = [{ query: OrderDocument, variables: { id: order.id } }, 'OrdersPaged']
  const [editOrder, state] = useMutation(EditOrderDocument, { refetchQueries: refetch })

  const [qty, setQty] = useState<Record<string, string>>({})
  const [removed, setRemoved] = useState<Record<string, boolean>>({})
  const [adds, setAdds] = useState<AddRow[]>([])
  const [error, setError] = useState<string | null>(null)

  function seed() {
    setError(null)
    setQty(Object.fromEntries(order.lines.map((l) => [l.id, String(l.quantity)])))
    setRemoved({})
    setAdds([])
  }

  function buildInput() {
    const updateLines = order.lines
      .filter((l) => !removed[l.id])
      .filter((l) => Number(qty[l.id]) !== l.quantity && Number(qty[l.id]) > 0)
      .map((l) => ({ orderLineId: l.id, quantity: Number(qty[l.id]) }))
    const removeLines = order.lines.filter((l) => removed[l.id]).map((l) => l.id)
    const addLines = adds
      .filter((a) => a.sku.trim() !== '' && Number(a.quantity) > 0)
      .map((a) => ({ productSku: a.sku.trim(), quantity: Number(a.quantity) }))
    return { updateLines, removeLines, addLines }
  }

  const { updateLines, removeLines, addLines } = buildInput()
  const hasChanges = updateLines.length + removeLines.length + addLines.length > 0

  async function onSubmit() {
    if (!hasChanges) {
      setError('No changes to apply.')
      return
    }
    setError(null)
    try {
      await editOrder({ variables: { input: { orderId: order.id, updateLines, removeLines, addLines } } })
      onOpenChange(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Edit failed.')
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
          <DialogTitle>Edit order</DialogTitle>
          <DialogDescription>
            Adjust quantities, remove lines, or add simple products. Totals are recomputed and stock
            re-reserved on save (a shortage aborts the edit). Bundle line quantities can't be edited.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-2">
            {order.lines.map((l) => {
              const isRemoved = !!removed[l.id]
              return (
                <div
                  key={l.id}
                  className={`flex items-center gap-3 rounded border px-3 py-2 ${isRemoved ? 'opacity-50' : ''}`}
                >
                  <div className="min-w-0 flex-1">
                    <div className={`truncate text-sm font-medium ${isRemoved ? 'line-through' : ''}`}>
                      {l.productName}
                    </div>
                    <div className="font-mono text-[10px] text-muted-foreground">{l.productSku}</div>
                  </div>
                  <Input
                    type="number"
                    min={0}
                    step="any"
                    value={qty[l.id] ?? String(l.quantity)}
                    disabled={isRemoved}
                    onChange={(e) => setQty((p) => ({ ...p, [l.id]: e.target.value }))}
                    className="w-24 tabular-nums"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="shrink-0"
                    onClick={() => setRemoved((p) => ({ ...p, [l.id]: !p[l.id] }))}
                    aria-label={isRemoved ? 'Keep line' : 'Remove line'}
                  >
                    {isRemoved ? <Undo2Icon className="size-4" /> : <Trash2Icon className="size-4 text-destructive" />}
                  </Button>
                </div>
              )
            })}
          </div>

          <div className="grid gap-2">
            {adds.map((a, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  placeholder="SKU"
                  value={a.sku}
                  onChange={(e) => setAdds((p) => p.map((row, idx) => (idx === i ? { ...row, sku: e.target.value } : row)))}
                  className="font-mono"
                />
                <Input
                  type="number"
                  min={0}
                  step="any"
                  placeholder="qty"
                  value={a.quantity}
                  onChange={(e) => setAdds((p) => p.map((row, idx) => (idx === i ? { ...row, quantity: e.target.value } : row)))}
                  className="w-24 tabular-nums"
                />
                <Button type="button" variant="ghost" size="icon" className="shrink-0" onClick={() => setAdds((p) => p.filter((_, idx) => idx !== i))}>
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            ))}
            <Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => setAdds((p) => [...p, { sku: '', quantity: '1' }])}>
              <PlusIcon className="size-4" />
              Add product
            </Button>
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
          <Button type="button" onClick={onSubmit} disabled={state.loading || !hasChanges}>
            {state.loading ? 'Saving…' : 'Apply changes'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
