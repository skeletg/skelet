import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { formatCents } from '@/lib/format'
import { centsToDollars, dollarsToCents } from '@/shared/schemas/fields'
import { AdminSetCartLinePriceDocument, type CartFieldsFragment } from './Carts.gql'
import { priceFloor, setPriceSchema, type SetPriceValues } from './schemas/setPrice'

type Line = CartFieldsFragment['lines'][number]
type Limits = NonNullable<CartFieldsFragment['priceLimits']>

// Sets the price of one line of a shopper's own cart - the CRUD-dialog exception, so it writes on its own submit.
// The shopper checks out at the price, for up to as many as the line holds now
export function SetPriceDialog({
  line,
  onClose,
  cartId,
  currency,
  limits,
}: {
  line: Line | null
  onClose: () => void
  cartId: string
  currency: string
  limits: Limits
}) {
  return (
    <FormDialog open={line !== null} onOpenChange={(open) => !open && onClose()} className="sm:max-w-md">
      {(close) => (line ? <SetPriceForm line={line} cartId={cartId} currency={currency} limits={limits} onClose={close} /> : null)}
    </FormDialog>
  )
}

function SetPriceForm({ line, cartId, currency, limits, onClose }: { line: Line; cartId: string; currency: string; limits: Limits; onClose: () => void }) {
  const [setPrice] = useMutation(AdminSetCartLinePriceDocument)
  const catalog = line.priceOverride ? (line.listUnitPrice ?? null) : line.unitPrice
  const floor = priceFloor(catalog, limits)
  // A line at its catalog price reads as one promotions apply to - a price set on it starts with them off
  const baseline: SetPriceValues = { price: centsToDollars(line.unitPrice), reason: line.priceReason, promotionsApply: line.priceOverride && line.promotionsApply }
  const dialog = useDialogForm({
    baseline,
    schema: setPriceSchema,
    submit: async (v: SetPriceValues) => {
      await setPrice({ variables: { cartId, lineId: line.id, unitPrice: dollarsToCents(v.price), reason: v.reason.trim(), promotionsApply: v.promotionsApply } })
    },
    onSaved: onClose,
  })
  const { form } = dialog
  const holds = limits.holdsUntil
    ? `until ${new Date(limits.holdsUntil).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })}`
    : 'for as long as the cart lives'
  return (
    <FormDialogBody
      dialog={dialog}
      title={`Set the price of ${line.name}`}
      description={`The shopper checks out at this price, for up to ${line.quantity} of the item, ${holds}. After that the line goes back to its catalog price.`}
      submitLabel="Set price"
      onCancel={onClose}
    >
      <form.AppField name="price">
        {(f) => (
          <f.TextField
            id="sp-price"
            label="Unit price"
            type="number"
            step="0.01"
            min="0"
            hint={
              catalog === null
                ? undefined
                : floor === null
                  ? `The catalog price is ${formatCents(catalog, currency)}.`
                  : `The catalog price is ${formatCents(catalog, currency)}. The lowest you can set is ${formatCents(floor, currency)}.`
            }
          />
        )}
      </form.AppField>
      <form.AppField name="reason">
        {(f) => <f.TextField id="sp-reason" label="Reason" placeholder="Price match" hint="Kept with the line and the order it becomes." />}
      </form.AppField>
      <form.AppField name="promotionsApply">
        {(f) => <f.CheckboxField id="sp-promotions" label="Coupons and promotions can lower it further" />}
      </form.AppField>
    </FormDialogBody>
  )
}
