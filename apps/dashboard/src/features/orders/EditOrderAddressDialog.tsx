import { useMemo, useRef } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { AddressFields } from '@/components/form/AddressFields'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { formatCents } from '@/lib/format'
import { addressChanged, toAddressValues } from '@/shared/schemas/address'
import { EditOrderAddressDocument, OrderDocument, type OrderQuery } from './Orders.gql'
import { orderAddressSchema, toOrderAddressInput, type OrderAddressValues } from './schemas/editOrderAddress'

type OrderData = OrderQuery['order']

// Rewrites the address snapshot of a placed, pre-fulfillment order - see schemas/editOrderAddress.
// Once anything ships the snapshot freezes server-side
export function EditOrderAddressDialog({
  order,
  open,
  onOpenChange,
  onSaved,
}: {
  order: OrderData
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: (summary: string) => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
      {(close) => <EditOrderAddressForm order={order} onSaved={onSaved} onClose={close} />}
    </FormDialog>
  )
}

function EditOrderAddressForm({ order, onSaved, onClose }: { order: OrderData; onSaved: (summary: string) => void; onClose: () => void }) {
  const [editAddress] = useMutation(EditOrderAddressDocument, {
    refetchQueries: [{ query: OrderDocument, variables: { id: order.id } }, 'OrdersPaged'],
  })
  const stored = useMemo<OrderAddressValues>(
    () => ({ shipping: toAddressValues(order.shippingAddress), billing: toAddressValues(order.billingAddress) }),
    [order.shippingAddress, order.billingAddress],
  )
  const summary = useRef('')

  const dialog = useDialogForm({
    baseline: stored,
    schema: orderAddressSchema(stored),
    submit: async (v) => {
      const { input, shippingChanged, billingChanged } = toOrderAddressInput(v, stored, order.id)
      const before = { tax: order.taxAmount, shipping: order.shippingAmount, total: order.total }
      const res = await editAddress({ variables: { input } })
      summary.current = summarize(before, res.data?.editOrderAddress, shippingChanged, billingChanged, order.currencyCode)
    },
    onSaved: () => {
      onSaved(summary.current)
      onClose()
    },
  })
  const changed = useStore(dialog.form.store, (s) => addressChanged(s.values.shipping, stored.shipping) || addressChanged(s.values.billing, stored.billing))

  return (
    <FormDialogBody
      dialog={dialog}
      title="Edit addresses"
      description="Rewrites the order's address snapshot. Only the side you change is sent. Changing the shipping address recomputes tax and re-quotes the recorded shipping method - if the carrier can't be reached the old shipping amount is kept."
      submitLabel="Save addresses"
      canSubmit={changed}
      onCancel={onClose}
    >
      <div className="grid gap-4 md:grid-cols-2">
        <AddressSide form={dialog.form} side="shipping" title="Shipping address" stored={stored.shipping} />
        <AddressSide form={dialog.form} side="billing" title="Billing address" stored={stored.billing} />
      </div>
    </FormDialogBody>
  )
}

function AddressSide({
  form,
  side,
  title,
  stored,
}: {
  form: ReturnType<typeof useDialogForm<OrderAddressValues>>['form']
  side: 'shipping' | 'billing'
  title: string
  stored: OrderAddressValues['shipping']
}) {
  const dirty = useStore(form.store, (s) => addressChanged(s.values[side], stored))
  return (
    <div className="rounded-md border p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</div>
        {dirty && <span className="text-xs text-muted-foreground">edited</span>}
      </div>
      <AddressFields form={form} fields={side} idPrefix={side} />
    </div>
  )
}

// What the operator needs to know after the write: which side moved, and whether the recompute
// shifted any money
function summarize(
  before: { tax: number; shipping: number; total: number },
  after: { taxAmount: number; shippingAmount: number; total: number } | null | undefined,
  shippingChanged: boolean,
  billingChanged: boolean,
  currency: string,
) {
  const what = shippingChanged && billingChanged ? 'Shipping + billing address' : shippingChanged ? 'Shipping address' : 'Billing address'
  if (!after) return `${what} updated.`
  const moves: string[] = []
  const move = (label: string, from: number, to: number) => {
    if (from !== to) moves.push(`${label} ${formatCents(from, currency)} → ${formatCents(to, currency)}`)
  }
  move('tax', before.tax, after.taxAmount)
  move('shipping', before.shipping, after.shippingAmount)
  move('total', before.total, after.total)
  return moves.length === 0 ? `${what} updated - totals unchanged.` : `${what} updated - ${moves.join(', ')}.`
}
