import { useMemo } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope } from '@/lib/config-scope'
import { ChannelAvailability, type AvailabilityChange } from '@/shared/availability/ChannelAvailability'
import { PaymentMethodsDocument, SetPaymentMethodCountriesDocument, SetPaymentMethodEnabledDocument } from './Payments.gql'

// PaymentAvailability is the per-channel enablement + country rules for payment providers - shown on Checkout's Payment tab and in
// Settings → Payment above each provider's fields. Enablement/countries live in the payment_methods table (queried at checkout), so
// they stage here rather than as config fields - When `providerSlug` is set it shows just that provider; otherwise all of them
export function PaymentAvailability({ providerSlug, disabled }: { providerSlug?: string; disabled?: boolean }) {
  const { effectiveChannelId, isChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const permissions = usePermissions()
  const canManage = !!effectiveChannelId && permissions.canAccessChannelScope('payments.manage', effectiveChannelId)

  const { data, loading, error } = useQuery(PaymentMethodsDocument, { variables: { channelId }, skip: !effectiveChannelId })
  const refetchQueries = [{ query: PaymentMethodsDocument, variables: { channelId } }]
  const [setEnabled] = useMutation(SetPaymentMethodEnabledDocument, { refetchQueries, awaitRefetchQueries: true })
  const [setCountries] = useMutation(SetPaymentMethodCountriesDocument, { refetchQueries, awaitRefetchQueries: true })

  const items = useMemo(
    () =>
      [...(data?.paymentMethods ?? [])]
        .filter((m) => !providerSlug || m.providerSlug === providerSlug)
        .sort((a, b) => a.sortOrder - b.sortOrder || a.providerSlug.localeCompare(b.providerSlug))
        .map((m) => ({
          slug: m.providerSlug,
          name: m.displayName,
          enabled: m.isEnabled,
          countryMode: m.countryMode,
          allowedCountries: m.allowedCountries,
          badges: [m.isOffline ? 'Manual payment' : 'Gateway'],
          unavailableReason: m.isAvailable === false ? 'The provider plugin is disabled. Enable it in Payment providers before offering this method.' : undefined,
        })),
    [data, providerSlug],
  )

  async function save(change: AvailabilityChange) {
    if (change.enabled !== undefined) {
      await setEnabled({ variables: { channelId, providerSlug: change.slug, enabled: change.enabled } })
    }
    if (change.countries) {
      await setCountries({ variables: { channelId, providerSlug: change.slug, ...change.countries } })
    }
  }

  return (
    <ChannelAvailability
      key={channelId}
      sectionId={`payment-availability${providerSlug ? `:${providerSlug}` : ''}`}
      title="Payment methods"
      description="Which payment providers this channel offers at checkout, and in which countries each is accepted."
      countriesLabel="Accepted in"
      items={items}
      loading={loading}
      error={error?.message}
      isChannel={isChannel}
      emptyText="No payment providers installed."
      disabled={disabled || !canManage}
      onSave={save}
    />
  )
}
