import { useMemo } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useAppForm } from '@/components/form/form'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { DetailPageSection } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFormSection } from '@/shared/saving/useFormSection'
import {
  ChannelDocument,
  ChannelsDocument,
  UpdateChannelDocument,
  type ChannelQuery,
} from '@/features/channels/Channels.gql'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import {
  channelIdentityDiff,
  channelIdentitySchema,
  friendlyCurrencyError,
  toChannelIdentityInput,
  toChannelIdentityValues,
} from './schemas/channelIdentity'

// IdentityEditor handles the per-channel identity columns that didn't fit the config registry model:
// name, currency code, public URL, and the is_active toggle - Slug and default locale are
// intentionally missing - slug is immutable post-create; default locale lives next to the
// supported-locales picker in LocalesEditor
//
// Edits are staged and applied through the SettingsPage header Save + SaveConfirmDialog (the
// dashboard-wide convention, see docs/conventions.md). No stored draft: the section shares the
// Settings page's registry with sections that don't keep one
export function IdentityEditor({ channelId }: { channelId: string }) {
  const { data, loading, error, refetch } = useQuery(ChannelDocument, {
    variables: { id: channelId },
    notifyOnNetworkStatusChange: true,
  })
  const channel = data?.channel

  if (loading && !channel) {
    return (
      <DetailPageSection description="Loading channel…">
        <div />
      </DetailPageSection>
    )
  }

  if (error || !channel) {
    return (
      <DetailPageSection>
        <LoadError
          title="Couldn't load channel"
          message={error?.message ?? 'Channel not found.'}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </DetailPageSection>
    )
  }

  // Keyed on the channel - switching channel in the header scope picker starts a fresh form
  return <IdentityForm key={channel.id} channel={channel} />
}

function IdentityForm({ channel }: { channel: NonNullable<ChannelQuery['channel']> }) {
  const permissions = usePermissions()
  const canEdit = permissions.canAccessChannelScope('channels.manage', channel.id)

  const currenciesQuery = useQuery(CurrenciesPickerDocument)
  // A channel whose currency was archived (or predates the picker) would otherwise render an
  // empty box that silently rewrites the value on save - keep its code selectable
  const currencies = useMemo(() => {
    const list = currenciesQuery.data?.currencies ?? []
    const code = channel.currencyCode
    if (code && !list.some((c) => c.code === code)) {
      return [...list, { code, name: 'no longer active' }]
    }
    return list
  }, [currenciesQuery.data, channel.currencyCode])

  const [updateChannel] = useMutation(UpdateChannelDocument, {
    refetchQueries: [
      { query: ChannelsDocument },
      { query: ChannelDocument, variables: { id: channel.id } },
    ],
  })

  const baseline = useMemo(() => toChannelIdentityValues(channel), [channel])
  const form = useAppForm({
    defaultValues: baseline,
    validators: { onChange: channelIdentitySchema },
    onSubmit: async ({ value }) => {
      try {
        await updateChannel({ variables: { input: toChannelIdentityInput(value, channel) } })
      } catch (err) {
        // The friendly message replaces an unreadable backend one - keep the original as the cause
        // so it still reaches the console rather than vanishing
        throw new Error(friendlyCurrencyError(err instanceof Error ? err.message : 'Save failed.'), { cause: err })
      }
      form.reset(value)
    },
  })

  useFormSection({ id: 'channel-identity', title: 'Identity', form, baseline, diff: channelIdentityDiff })

  return (
    <DetailPageSection
      description={`Name, currency, URL, and active state for "${channel.name}". Slug (${channel.slug}) is immutable. Apply edits via the header Save button.`}
    >
      <fieldset disabled={!canEdit} className="grid gap-4">
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="name">{(f) => <f.TextField id="identity-name" label="Name" />}</form.AppField>
          {/* The store's own currencies, same picker the create dialog uses - typing a code
              here only ever produced a CURRENCY_NOT_FOUND at save */}
          <form.AppField name="currencyCode">
            {(f) => (
              <f.SelectField
                id="identity-currency"
                label="Currency"
                placeholder="Pick a currency"
                disabled={!canEdit || currencies.length === 0}
                options={currencies.map((c) => ({
                  value: c.code,
                  label: (
                    <>
                      <span className="font-mono text-xs">{c.code}</span> · {c.name}
                    </>
                  ),
                }))}
                hint="Add currencies under Settings - Currencies. Changing this is blocked when listings inherit the channel's price - fix those first."
              />
            )}
          </form.AppField>
          <form.AppField name="publicUrl">
            {(f) => (
              <f.TextField
                id="identity-public-url"
                label="Public URL"
                type="url"
                placeholder="https://wheels.example.com"
                className="sm:col-span-2"
                hint="Used to build verification + password-reset links in transactional emails, and to match incoming Host headers to this channel."
              />
            )}
          </form.AppField>
        </div>

        <form.AppField name="isActive">
          {(f) => (
            <f.SwitchField
              id="identity-active"
              label="Active"
              description="Inactive channels remain readable but the storefront filters them out of channel-switcher dropdowns."
              disabled={!canEdit}
            />
          )}
        </form.AppField>

        {!canEdit && (
          <Alert>
            <AlertTitle>Read-only</AlertTitle>
            <AlertDescription>You don't have channels.manage on this channel.</AlertDescription>
          </Alert>
        )}
      </fieldset>
    </DetailPageSection>
  )
}
