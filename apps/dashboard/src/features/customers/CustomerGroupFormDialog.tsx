import type { ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { CreateCustomerGroupDocument, CustomerGroupsDocument, UpdateCustomerGroupDocument } from './CustomerGroups.gql'
import {
  customerGroupSchema,
  toCustomerGroupInput,
  toCustomerGroupValues,
  type ExistingCustomerGroup,
} from './schemas/customerGroup'

export type { ExistingCustomerGroup }

export function CustomerGroupFormDialog({
  group,
  trigger,
  open,
  onOpenChange,
}: {
  group?: ExistingCustomerGroup
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} trigger={trigger}>
      {(close) => <CustomerGroupForm group={group} onClose={close} />}
    </FormDialog>
  )
}

function CustomerGroupForm({ group, onClose }: { group?: ExistingCustomerGroup; onClose: () => void }) {
  const refetchQueries = [{ query: CustomerGroupsDocument }]
  const [createGroup] = useMutation(CreateCustomerGroupDocument, { refetchQueries })
  const [updateGroup] = useMutation(UpdateCustomerGroupDocument, { refetchQueries })

  const dialog = useDialogForm({
    baseline: toCustomerGroupValues(group),
    schema: customerGroupSchema,
    submit: async (value) => {
      const input = toCustomerGroupInput(value)
      if (group) await updateGroup({ variables: { id: group.id, input } })
      else await createGroup({ variables: { input: { slug: value.slug.trim(), ...input } } })
    },
    onSaved: onClose,
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title={group ? `Edit ${group.name}` : 'New customer group'}
      description={
        group
          ? 'Update the group. Slug is immutable - create a new group instead if you need a different slug.'
          : 'Create a customer group. Customers can be assigned to it individually.'
      }
      submitLabel={group ? 'Save changes' : 'Create group'}
      onCancel={onClose}
    >
      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
        <form.AppField name="slug">
          {(f) => (
            <f.TextField
              id="cg-slug"
              label="Slug"
              placeholder="wholesale"
              disabled={!!group}
              hint={group ? 'Slug is immutable.' : undefined}
            />
          )}
        </form.AppField>
        <form.AppField name="name">{(f) => <f.TextField id="cg-name" label="Name" placeholder="Wholesale" />}</form.AppField>
      </div>
      <form.AppField name="description">
        {(f) => <f.TextareaField id="cg-description" label="Description" placeholder="Who belongs in this group?" />}
      </form.AppField>
    </FormDialogBody>
  )
}
