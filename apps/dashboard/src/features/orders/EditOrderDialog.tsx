import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { PlusIcon, Trash2Icon, Undo2Icon } from 'lucide-react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { FieldError } from '@/components/form/fields'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EditOrderDocument, OrderDocument, OrdersDocument, type OrderQuery } from './Orders.gql'
import { editOrderSchema, hasOrderChanges, toEditOrderInput, toEditOrderValues } from './schemas/editOrder'

type OrderData = OrderQuery['order']

// Edits a pre-fulfillment order: change line quantities, remove lines, add simple-product lines by SKU
export function EditOrderDialog({ order, open, onOpenChange }: { order: OrderData; open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange}>
      {(close) => <EditOrderForm order={order} onClose={close} />}
    </FormDialog>
  )
}

function EditOrderForm({ order, onClose }: { order: OrderData; onClose: () => void }) {
  const [editOrder] = useMutation(EditOrderDocument, {
    refetchQueries: [{ query: OrderDocument, variables: { id: order.id } }, { query: OrdersDocument }],
  })
  const dialog = useDialogForm({
    baseline: toEditOrderValues(order.lines),
    schema: editOrderSchema,
    submit: async (v) => {
      await editOrder({ variables: { input: { orderId: order.id, ...toEditOrderInput(v, order.lines) } } })
    },
    onSaved: onClose,
  })
  const { form } = dialog
  const changed = useStore(form.store, (s) => hasOrderChanges(toEditOrderInput(s.values, order.lines)))
  const byId = new Map(order.lines.map((l) => [l.id, l]))

  return (
    <FormDialogBody
      dialog={dialog}
      title="Edit order"
      description="Adjust quantities, remove lines, or add simple products. Totals are recomputed and stock re-reserved on save (a shortage aborts the edit). Bundle line quantities can't be edited."
      submitLabel="Apply changes"
      canSubmit={changed}
      onCancel={onClose}
    >
      <form.Field name="lines" mode="array">
        {(lines) => (
          <div className="grid gap-2">
            {lines.state.value.map((row, i) => {
              const line = byId.get(row.id)
              return (
                <div key={row.id} className={`grid gap-1 rounded border px-3 py-2 ${row.removed ? 'opacity-50' : ''}`}>
                  <div className="flex items-center gap-3">
                    <div className="min-w-0 flex-1">
                      <div className={`truncate text-sm font-medium ${row.removed ? 'line-through' : ''}`}>{line?.productName}</div>
                      <div className="font-mono text-[10px] text-muted-foreground">{line?.productSku}</div>
                    </div>
                    <form.Field name={`lines[${i}].quantity`}>
                      {(f) => (
                        <Input
                          type="number"
                          min={0}
                          step="any"
                          aria-label={`${line?.productName ?? 'Line'} quantity`}
                          value={f.state.value}
                          disabled={row.removed}
                          onChange={(e) => f.handleChange(e.target.value)}
                          onBlur={f.handleBlur}
                          className="w-24 tabular-nums"
                        />
                      )}
                    </form.Field>
                    <form.Field name={`lines[${i}].removed`}>
                      {(f) => (
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="shrink-0"
                          onClick={() => f.handleChange(!f.state.value)}
                          aria-label={f.state.value ? 'Keep line' : 'Remove line'}
                        >
                          {f.state.value ? <Undo2Icon className="size-4" /> : <Trash2Icon className="size-4 text-destructive" />}
                        </Button>
                      )}
                    </form.Field>
                  </div>
                  <form.Field name={`lines[${i}].quantity`}>{(f) => <FieldError field={f} />}</form.Field>
                </div>
              )
            })}
          </div>
        )}
      </form.Field>

      <form.Field name="adds" mode="array">
        {(adds) => (
          <div className="grid gap-2">
            {adds.state.value.map((_, i) => (
              <div key={i} className="flex items-start gap-2">
                <form.Field name={`adds[${i}].sku`}>
                  {(f) => (
                    <Input
                      placeholder="SKU"
                      aria-label={`New line ${i + 1} SKU`}
                      value={f.state.value}
                      onChange={(e) => f.handleChange(e.target.value)}
                      className="font-mono"
                    />
                  )}
                </form.Field>
                <form.Field name={`adds[${i}].quantity`}>
                  {(f) => (
                    <div className="grid gap-1">
                      <Input
                        type="number"
                        min={0}
                        step="any"
                        placeholder="qty"
                        aria-label={`New line ${i + 1} quantity`}
                        value={f.state.value}
                        onChange={(e) => f.handleChange(e.target.value)}
                        onBlur={f.handleBlur}
                        className="w-24 tabular-nums"
                      />
                      <FieldError field={f} />
                    </div>
                  )}
                </form.Field>
                <Button type="button" variant="ghost" size="icon" className="shrink-0" aria-label="Remove new line" onClick={() => adds.removeValue(i)}>
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            ))}
            <Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => adds.pushValue({ sku: '', quantity: '1' })}>
              <PlusIcon className="size-4" />
              Add product
            </Button>
          </div>
        )}
      </form.Field>
    </FormDialogBody>
  )
}
