import { useMemo } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useConfigScope } from '@/lib/config-scope'
import { ChannelAvailability, type AvailabilityChange } from '@/shared/availability/ChannelAvailability'
import {
  SetShippingCarrierCountriesDocument,
  SetShippingCarrierEnabledDocument,
  SetShippingCarrierWeekendDeliveryDocument,
  ShippingCarriersDocument,
} from './Shipping.gql'

// The built-in carrier's estimates come from its methods, which each say whether they deliver on weekends
const BUILTIN = 'builtin'

// ShippingCarrierAvailability is the per-channel enablement + country rules for shipping carriers - shown on Checkout's Delivery tab
// and in Settings → Shipping above each carrier's fields. Enablement/countries live in the shipping_carriers table (queried at
// checkout), so they stage here rather than as config fields - When `carrierSlug` is set it shows just that carrier; otherwise all
export function ShippingCarrierAvailability({ carrierSlug, disabled }: { carrierSlug?: string; disabled?: boolean }) {
  const { effectiveChannelId, isChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''

  const { data, loading, error } = useQuery(ShippingCarriersDocument, { variables: { channelId }, skip: !effectiveChannelId })
  const refetchQueries = [{ query: ShippingCarriersDocument, variables: { channelId } }]
  const [setEnabled] = useMutation(SetShippingCarrierEnabledDocument, { refetchQueries, awaitRefetchQueries: true })
  const [setCountries] = useMutation(SetShippingCarrierCountriesDocument, { refetchQueries, awaitRefetchQueries: true })
  const [setWeekends] = useMutation(SetShippingCarrierWeekendDeliveryDocument, { refetchQueries, awaitRefetchQueries: true })

  const items = useMemo(
    () =>
      [...(data?.shippingCarriers ?? [])]
        .filter((c) => !carrierSlug || c.carrierSlug === carrierSlug)
        .sort((a, b) => a.sortOrder - b.sortOrder || a.carrierSlug.localeCompare(b.carrierSlug))
        .map((c) => ({
          slug: c.carrierSlug,
          name: c.displayName,
          enabled: c.isEnabled,
          countryMode: c.countryMode,
          allowedCountries: c.allowedCountries,
          badges: [...(c.supportsTracking ? ['tracking'] : []), ...(c.supportsLabels ? ['labels'] : [])],
          deliversOnWeekends: c.carrierSlug === BUILTIN ? undefined : c.deliversOnWeekends,
        })),
    [data, carrierSlug],
  )

  async function save(change: AvailabilityChange) {
    if (change.enabled !== undefined) {
      await setEnabled({ variables: { channelId, carrierSlug: change.slug, enabled: change.enabled } })
    }
    if (change.countries) {
      await setCountries({ variables: { channelId, carrierSlug: change.slug, ...change.countries } })
    }
    if (change.deliversOnWeekends !== undefined) {
      await setWeekends({ variables: { channelId, carrierSlug: change.slug, deliversOnWeekends: change.deliversOnWeekends } })
    }
  }

  return (
    <ChannelAvailability
      key={channelId}
      sectionId={`carrier-availability${carrierSlug ? `:${carrierSlug}` : ''}`}
      title="Carriers"
      description="Which carriers quote rates on this channel, and which countries each ships to. Rates from every enabled carrier are offered side by side at checkout."
      countriesLabel="Ships to"
      items={items}
      loading={loading}
      error={error?.message}
      isChannel={isChannel}
      emptyText="No carriers installed."
      disabled={disabled}
      onSave={save}
    />
  )
}
