import { useRef, type ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Label } from '@/components/ui/label'
import { PermissionSelector } from '@/components/PermissionSelector'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CreateIntegrationDocument, IntegrationsDocument, UpdateIntegrationDocument } from './Integrations.gql'
import {
  integrationSchema,
  toIntegrationCreateInput,
  toIntegrationUpdateInput,
  toIntegrationValues,
  type ExistingIntegration,
} from './schemas/integration'

export type { ExistingIntegration }

type CreatedToken = { token: string; name: string }

export function IntegrationFormDialog({
  integration,
  trigger,
  open,
  onOpenChange,
  onCreated,
}: {
  integration?: ExistingIntegration
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onCreated?: (payload: CreatedToken) => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} trigger={trigger} className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
      {(close) => <IntegrationForm integration={integration} onCreated={onCreated} onClose={close} />}
    </FormDialog>
  )
}

function IntegrationForm({
  integration,
  onCreated,
  onClose,
}: {
  integration?: ExistingIntegration
  onCreated?: (payload: CreatedToken) => void
  onClose: () => void
}) {
  const channels = useQuery(ChannelsDocument, { fetchPolicy: 'cache-first' }).data?.channels ?? []
  const refetchQueries = [{ query: IntegrationsDocument }]
  const [createIntegration] = useMutation(CreateIntegrationDocument, { refetchQueries })
  const [updateIntegration] = useMutation(UpdateIntegrationDocument, { refetchQueries })
  // The raw token comes back from create exactly once - handed to the caller after the dialog closes
  const created = useRef<CreatedToken | null>(null)

  const dialog = useDialogForm({
    baseline: toIntegrationValues(integration),
    schema: integrationSchema,
    submit: async (v) => {
      if (integration) {
        await updateIntegration({ variables: { input: toIntegrationUpdateInput(v, integration.id) } })
        return
      }
      const { data } = await createIntegration({ variables: { input: toIntegrationCreateInput(v) } })
      if (data) created.current = { token: data.createIntegration.rawToken, name: data.createIntegration.integration.name }
    },
    onSaved: () => {
      onClose()
      if (created.current) onCreated?.(created.current)
    },
  })
  const { form } = dialog
  const hasExpiry = useStore(form.store, (s) => s.values.hasExpiry)

  return (
    <FormDialogBody
      dialog={dialog}
      title={integration ? `Edit ${integration.name}` : 'New integration'}
      description={
        integration
          ? 'Update the integration. Changes take effect on the next API call; to rotate credentials use Regenerate token.'
          : "Create an integration. You'll see the API token exactly once after creation."
      }
      submitLabel={integration ? 'Save changes' : 'Create integration'}
      onCancel={onClose}
    >
      <form.AppField name="name">
        {(f) => <f.TextField id="integration-name" label="Name" placeholder="Order placed webhook" />}
      </form.AppField>
      <form.AppField name="description">
        {(f) => <f.TextareaField id="integration-description" label="Description" placeholder="What is this token used for?" />}
      </form.AppField>

      <div className="grid gap-2">
        <Label>Permissions</Label>
        <form.Field name="permissionSlugs">
          {(f) => <PermissionSelector selected={f.state.value} onChange={f.handleChange} />}
        </form.Field>
      </div>

      <form.AppField name="channelId">
        {(f) => (
          <f.SelectField
            id="integration-channel"
            label="Channel"
            noneLabel="All channels (global token)"
            options={channels.map((c) => ({ value: c.id, label: c.name }))}
            hint="A pinned token only satisfies checks for that channel - a feed token for one storefront can't read the others."
          />
        )}
      </form.AppField>

      <div className="grid gap-2">
        <form.AppField name="hasExpiry">{(f) => <f.CheckboxField id="has-expiry" label="Set expiry date" />}</form.AppField>
        {hasExpiry && (
          <form.AppField name="expiresAt">
            {(f) => <f.TextField id="integration-expires" aria-label="Expires at" type="datetime-local" />}
          </form.AppField>
        )}
      </div>
    </FormDialogBody>
  )
}
