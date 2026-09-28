import { useMutation } from '@apollo/client/react'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { UpsertShippingBoxDocument } from './Shipping.gql'
import { boxSchema, toBoxInput, toBoxValues, type ExistingBox } from './schemas/box'

export type ShippingBoxRow = ExistingBox

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: string
  existing?: ShippingBoxRow | null
  onSaved: () => void
}

export function ShippingBoxFormDialog({ open, onOpenChange, ...props }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <ShippingBoxForm {...props} onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}

// Lives inside DialogContent, so each open mounts it fresh from `existing`
function ShippingBoxForm({ channelId, existing, onSaved, onClose }: Omit<Props, 'open' | 'onOpenChange'> & { onClose: () => void }) {
  const [upsert] = useMutation(UpsertShippingBoxDocument)
  const dialog = useDialogForm({
    baseline: toBoxValues(existing),
    schema: boxSchema,
    submit: async (value) => {
      await upsert({ variables: { input: { channelId, ...toBoxInput(value) } } })
    },
    onSaved: () => {
      onClose()
      onSaved()
    },
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title={existing ? 'Edit box' : 'New box'}
      description={
        existing
          ? 'Update this carton - quotes and labels pick it up immediately.'
          : 'A real carton from your warehouse. Once at least one active box exists, rate quotes and labels pack line items into cartons.'
      }
      submitLabel={existing ? 'Save changes' : 'Create box'}
      onCancel={onClose}
    >
      <div className="grid grid-cols-2 items-start gap-4">
        <form.AppField name="code">
          {(f) => (
            <f.TextField
              id="box-code"
              label="Code"
              placeholder="e.g. small-A"
              disabled={!!existing}
              hint={existing ? 'Code is the upsert key - fixed after creation' : undefined}
            />
          )}
        </form.AppField>
        <form.AppField name="name">{(f) => <f.TextField id="box-name" label="Name" placeholder="Small box A" />}</form.AppField>
      </div>

      <div className="grid gap-2">
        <Label>Inner dimensions (mm)</Label>
        <div className="grid grid-cols-3 items-start gap-2">
          <form.AppField name="length">
            {(f) => <f.TextField id="box-length" label="Length" type="number" min="1" />}
          </form.AppField>
          <form.AppField name="width">
            {(f) => <f.TextField id="box-width" label="Width" type="number" min="1" />}
          </form.AppField>
          <form.AppField name="height">
            {(f) => <f.TextField id="box-height" label="Height" type="number" min="1" />}
          </form.AppField>
        </div>
      </div>

      <div className="grid grid-cols-2 items-start gap-4">
        <form.AppField name="maxWeight">
          {(f) => <f.TextField id="box-max-weight" label="Max load (g)" type="number" min="0" placeholder="0 = unlimited" />}
        </form.AppField>
        <form.AppField name="boxWeight">
          {(f) => (
            <f.TextField id="box-weight" label="Empty box weight (g)" type="number" min="0" placeholder="added to each parcel" />
          )}
        </form.AppField>
      </div>

      <div className="flex items-end justify-between gap-4">
        <form.AppField name="sortOrder">
          {(f) => <f.NumberField id="box-sort" label="Sort order" className="w-24" />}
        </form.AppField>
        <form.Field name="isActive">
          {(f) => (
            <div className="flex items-center gap-3 pb-2">
              <Switch id="box-active" checked={f.state.value} onCheckedChange={f.handleChange} />
              <Label htmlFor="box-active" className="font-normal">
                Active
              </Label>
            </div>
          )}
        </form.Field>
      </div>
    </FormDialogBody>
  )
}
