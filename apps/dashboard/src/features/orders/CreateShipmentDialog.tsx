import { useMemo } from 'react'
import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { FieldError } from '@/components/form/fields'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { CreateFulfillmentDocument, OrderDocument, OrdersPagedDocument, type OrderQuery } from './Orders.gql'
import { remainingByLine, shipmentSchema, toShipmentInput, toShipmentValues } from './schemas/shipment'

type OrderData = OrderQuery['order']

// Records a partial shipment - the order settles to partially_fulfilled or fulfilled. The one-click
// "Fulfill remaining" header action stays for the common ship-everything case
export function CreateShipmentDialog({ order, open, onOpenChange }: { order: OrderData; open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange}>
      {(close) => <CreateShipmentForm order={order} onClose={close} />}
    </FormDialog>
  )
}

function CreateShipmentForm({ order, onClose }: { order: OrderData; onClose: () => void }) {
  const [createFulfillment] = useMutation(CreateFulfillmentDocument, {
    refetchQueries: [{ query: OrderDocument, variables: { id: order.id } }, OrdersPagedDocument],
  })
  const remaining = useMemo(() => remainingByLine(order.lines, order.fulfillments), [order.lines, order.fulfillments])
  const byId = new Map(order.lines.map((l) => [l.id, l]))

  const dialog = useDialogForm({
    baseline: toShipmentValues(order.lines, remaining),
    schema: shipmentSchema,
    submit: async (v) => {
      await createFulfillment({ variables: { input: toShipmentInput(v, order.id) } })
    },
    onSaved: onClose,
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title="Create shipment"
      description="Ship a subset of the order. Quantities default to what's still unshipped per line."
      submitLabel="Create shipment"
      onCancel={onClose}
    >
      <form.Field name="lines" mode="array">
        {(lines) => (
          <div className="grid gap-2">
            {lines.state.value.map((row, i) => {
              const line = byId.get(row.id)
              return (
                <div key={row.id} className="grid gap-1 rounded border px-3 py-2">
                  <div className="flex items-center gap-3">
                    <form.Field name={`lines[${i}].checked`}>
                      {(f) => (
                        <Checkbox aria-label={`Ship ${line?.name ?? 'line'}`} checked={f.state.value} onCheckedChange={(v) => f.handleChange(!!v)} />
                      )}
                    </form.Field>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium">{line?.name}</div>
                      <div className="font-mono text-[10px] text-muted-foreground">
                        {line?.sku} · {row.remaining} unshipped
                      </div>
                    </div>
                    <form.Field name={`lines[${i}].quantity`}>
                      {(f) => (
                        <Input
                          type="number"
                          min={0}
                          max={row.remaining}
                          step="any"
                          aria-label={`${line?.name ?? 'Line'} quantity`}
                          value={f.state.value}
                          disabled={!row.checked}
                          onChange={(e) => f.handleChange(e.target.value)}
                          onBlur={f.handleBlur}
                          className="w-24 tabular-nums"
                        />
                      )}
                    </form.Field>
                  </div>
                  <form.Field name={`lines[${i}].quantity`}>{(f) => <FieldError field={f} />}</form.Field>
                </div>
              )
            })}
            <FieldError field={lines} />
          </div>
        )}
      </form.Field>
      <div className="grid grid-cols-2 items-start gap-3">
        <form.AppField name="carrier">
          {(f) => <f.TextField id="ship-carrier" label="Carrier (optional)" placeholder="USPS, FedEx…" />}
        </form.AppField>
        <form.AppField name="tracking">
          {(f) => <f.TextField id="ship-tracking" label="Tracking (optional)" placeholder="1Z999…" />}
        </form.AppField>
      </div>
      <form.AppField name="insured">
        {(f) => (
          <f.TextField
            id="ship-insured"
            label="Insured value (optional)"
            type="number"
            min={0}
            step="0.01"
            placeholder="Declared value for shipment insurance"
            className="tabular-nums"
          />
        )}
      </form.AppField>
    </FormDialogBody>
  )
}
