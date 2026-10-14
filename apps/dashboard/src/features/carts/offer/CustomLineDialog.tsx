import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { dollarsToCents, parseNumberText } from '@/shared/schemas/fields'
import { AdminAddCustomCartLineDocument } from '../Carts.gql'
import { customLineSchema, emptyCustomLine, type CustomLineValues } from '../schemas/customLine'

// Adds a free-form charge to an offer - the CRUD-dialog exception, so it writes on its own submit
export function CustomLineDialog({ open, onOpenChange, cartId }: { open: boolean; onOpenChange: (open: boolean) => void; cartId: string }) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="sm:max-w-md">
      {(close) => <CustomLineForm cartId={cartId} onClose={close} />}
    </FormDialog>
  )
}

function CustomLineForm({ cartId, onClose }: { cartId: string; onClose: () => void }) {
  const [add] = useMutation(AdminAddCustomCartLineDocument)
  const dialog = useDialogForm({
    baseline: emptyCustomLine,
    schema: customLineSchema,
    submit: async (v: CustomLineValues) => {
      await add({
        variables: {
          cartId,
          input: {
            name: v.name.trim(),
            unitPrice: dollarsToCents(v.price),
            quantity: parseNumberText(v.quantity) ?? 1,
            taxClass: v.taxClass.trim() || null,
          },
        },
      })
    },
    onSaved: onClose,
  })
  const { form } = dialog
  return (
    <FormDialogBody
      dialog={dialog}
      title="Add a custom line"
      description="A charge with no product behind it - an extra payment, a service, a fee. It never ships."
      submitLabel="Add line"
      onCancel={onClose}
    >
      <form.AppField name="name">{(f) => <f.TextField id="cl-name" label="Name" placeholder="Installation" />}</form.AppField>
      <div className="grid grid-cols-2 items-start gap-2">
        <form.AppField name="price">{(f) => <f.TextField id="cl-price" label="Unit price" type="number" step="0.01" min="0" />}</form.AppField>
        <form.AppField name="quantity">{(f) => <f.TextField id="cl-qty" label="Quantity" type="number" step="any" min="0" />}</form.AppField>
      </div>
      <form.AppField name="taxClass">
        {(f) => <f.TextField id="cl-tax" label="Tax class" placeholder="standard" hint="Tax rules match lines on this class, the same as a product’s." />}
      </form.AppField>
    </FormDialogBody>
  )
}
