import { useRef } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { ChannelsDocument, CreateChannelDocument } from './Channels.gql'
import { emptyNewChannel, newChannelSchema, toNewChannelInput } from './schemas/newChannel'

// The minimal create flow for the Channels list - slug, name, currency and default locale, the rest
// under Settings after the redirect. `onCreated` gets the new id so the caller can navigate
export function NewChannelDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (channelId: string) => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="sm:max-w-md">
      {(close) => <NewChannelForm onClose={close} onCreated={onCreated} />}
    </FormDialog>
  )
}

function NewChannelForm({ onClose, onCreated }: { onClose: () => void; onCreated: (channelId: string) => void }) {
  const currencies = useQuery(CurrenciesPickerDocument).data?.currencies ?? []
  const locales = useQuery(LocalesPickerDocument).data?.locales ?? []
  const [createChannel] = useMutation(CreateChannelDocument, { refetchQueries: [ChannelsDocument] })
  const createdId = useRef('')

  const dialog = useDialogForm({
    baseline: emptyNewChannel,
    schema: newChannelSchema,
    submit: async (v) => {
      const res = await createChannel({ variables: { input: toNewChannelInput(v) } })
      const id = res.data?.createChannel.id
      if (!id) throw new Error('Create succeeded but no id returned. Reload to find the new channel.')
      createdId.current = id
    },
    onSaved: () => {
      onClose()
      onCreated(createdId.current)
    },
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title="New channel"
      description="Start with the identity. Everything else - branding, contact, legal, locales, inventory - is configured under Settings after the channel exists."
      submitLabel="Create channel"
      onCancel={onClose}
    >
      <form.AppField name="slug">
        {(f) => (
          <f.TextField
            id="channel-new-slug"
            label="Slug"
            placeholder="us"
            hint="Stable identifier for this channel, used by tooling and the X-Store-Slug header. Storefronts resolve by id or host, not by slug. Cannot be changed after create."
          />
        )}
      </form.AppField>
      <form.AppField name="name">
        {(f) => <f.TextField id="channel-new-name" label="Name" placeholder="United States" />}
      </form.AppField>
      <form.AppField name="currencyCode">
        {(f) => (
          <f.SelectField
            id="channel-new-currency"
            label="Currency"
            placeholder="Pick a currency"
            options={currencies.map((c) => ({
              value: c.code,
              label: (
                <>
                  <span className="font-mono text-xs">{c.code}</span> · {c.name}
                </>
              ),
            }))}
          />
        )}
      </form.AppField>
      <form.AppField name="defaultLocale">
        {(f) => (
          <f.SelectField
            id="channel-new-locale"
            label="Default locale"
            placeholder="Pick a default locale"
            hint="Supported-locales list can be expanded later in Settings."
            options={locales.map((l) => ({
              value: l.code,
              label: (
                <>
                  {l.flagEmoji ? `${l.flagEmoji} ` : ''}
                  <span className="font-mono text-xs">{l.code}</span> · {l.nativeName}
                </>
              ),
            }))}
          />
        )}
      </form.AppField>
    </FormDialogBody>
  )
}
