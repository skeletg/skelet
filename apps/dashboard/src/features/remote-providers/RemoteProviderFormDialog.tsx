import { useRef, type ReactNode } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import type { RemoteProviderKind } from '@/graphql/schema.types'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { KIND_HINTS, KIND_LABELS } from './kinds'
import { CreateRemoteProviderDocument, RemoteProvidersDocument, UpdateRemoteProviderDocument } from './RemoteProviders.gql'
import {
  remoteProviderSchema,
  toRemoteProviderCreateInput,
  toRemoteProviderUpdateInput,
  toRemoteProviderValues,
  type ExistingProvider,
} from './schemas/remoteProvider'

export type { ExistingProvider }

type CreatedSecret = { secret: string; name: string }

export function RemoteProviderFormDialog({
  trigger,
  existing,
  onCreated,
  onUpdated,
}: {
  trigger: ReactNode
  existing?: ExistingProvider
  onCreated?: (data: CreatedSecret) => void
  onUpdated?: () => void
}) {
  return (
    <FormDialog trigger={trigger} className="flex max-h-[90vh] flex-col sm:max-w-lg">
      {(close) => <RemoteProviderForm existing={existing} onCreated={onCreated} onUpdated={onUpdated} onClose={close} />}
    </FormDialog>
  )
}

function RemoteProviderForm({
  existing,
  onCreated,
  onUpdated,
  onClose,
}: {
  existing?: ExistingProvider
  onCreated?: (data: CreatedSecret) => void
  onUpdated?: () => void
  onClose: () => void
}) {
  const channelsQuery = useQuery(ChannelsDocument)
  const channels = channelsQuery.data?.channels ?? []
  const [createProvider] = useMutation(CreateRemoteProviderDocument, { refetchQueries: [{ query: RemoteProvidersDocument }] })
  const [updateProvider] = useMutation(UpdateRemoteProviderDocument)
  // The raw signing secret comes back from create exactly once - handed over after the dialog closes
  const created = useRef<CreatedSecret | null>(null)

  const dialog = useDialogForm({
    baseline: toRemoteProviderValues(existing),
    schema: remoteProviderSchema,
    submit: async (v) => {
      if (existing) {
        const input = toRemoteProviderUpdateInput(v, existing)
        if (input) await updateProvider({ variables: { input } })
        return
      }
      const { data } = await createProvider({ variables: { input: toRemoteProviderCreateInput(v) } })
      if (data) created.current = { secret: data.createRemoteProvider.rawSecret, name: data.createRemoteProvider.provider.name }
    },
    onSaved: () => {
      onClose()
      if (existing) onUpdated?.()
      else if (created.current) onCreated?.(created.current)
    },
  })
  const { form } = dialog
  const kind = useStore(form.store, (s) => s.values.kind) as RemoteProviderKind

  return (
    <FormDialogBody
      dialog={dialog}
      scrollable
      title={existing ? 'Edit remote provider' : 'New remote provider'}
      description={
        existing
          ? 'Update this endpoint. Kind, slug and store scope are fixed at creation.'
          : 'Register an HTTP endpoint that implements a pluggable role. Skelet calls it mid-request with a signed JSON envelope; its response is the answer.'
      }
      submitLabel={existing ? 'Save changes' : 'Create provider'}
      onCancel={onClose}
    >
      <form.AppField name="kind">
        {(f) => (
          <f.SelectField
            id="rp-kind"
            label="Role"
            disabled={!!existing}
            options={(Object.keys(KIND_LABELS) as RemoteProviderKind[]).map((k) => ({ value: k, label: KIND_LABELS[k] }))}
            hint={KIND_HINTS[kind]}
          />
        )}
      </form.AppField>

      <div className="grid items-start gap-4 sm:grid-cols-2">
        <form.AppField name="slug">
          {(f) => <f.TextField id="rp-slug" label="Slug" placeholder="my-tax-service" disabled={!!existing} className="font-mono" />}
        </form.AppField>
        <form.AppField name="name">{(f) => <f.TextField id="rp-name" label="Name" placeholder="My tax service" />}</form.AppField>
      </div>

      <form.AppField name="targetUrl">
        {(f) => (
          <f.TextField
            id="rp-url"
            label="Endpoint URL"
            type="url"
            placeholder="https://api.example.com/skelet/tax"
            className="font-mono"
          />
        )}
      </form.AppField>

      <form.AppField name="channelId">
        {(f) => (
          <f.SelectField
            id="rp-store"
            label="Channel scope"
            noneLabel="All stores (global)"
            disabled={channelsQuery.loading || !!existing}
            options={channels.map((ch) => ({ value: ch.id, label: ch.name }))}
          />
        )}
      </form.AppField>

      <form.AppField name="config">
        {(f) => (
          <f.TextareaField
            id="rp-config"
            label="Config (JSON, optional)"
            placeholder='{"apiKey": "..."}'
            rows={4}
            className="font-mono text-xs"
            hint="Forwarded verbatim in every invocation envelope - receiver-side keys and options."
          />
        )}
      </form.AppField>

      <div className="grid items-start gap-4 sm:grid-cols-3">
        <form.AppField name="timeoutMs">
          {(f) => <f.TextField id="rp-timeout" label="Timeout (ms)" type="number" min={0} max={30000} placeholder="Default: 5000" />}
        </form.AppField>
        <form.AppField name="cacheTtlS">
          {(f) => <f.TextField id="rp-cache" label="Cache TTL (s)" type="number" min={0} max={3600} placeholder="0 = off" />}
        </form.AppField>
        <form.AppField name="failurePolicy">
          {(f) => (
            <f.SelectField
              id="rp-policy"
              label="On failure"
              options={[
                { value: 'FAIL_OPEN', label: 'Fall back to core' },
                { value: 'FAIL_CLOSED', label: 'Abort the operation' },
              ]}
            />
          )}
        </form.AppField>
      </div>
    </FormDialogBody>
  )
}
