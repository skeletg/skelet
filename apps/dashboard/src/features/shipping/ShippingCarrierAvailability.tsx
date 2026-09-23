import { useMemo, useState } from 'react'
import { gql } from '@apollo/client'
import { useMutation, useQuery } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'

const SHIPPING_CARRIERS_QUERY = gql`
  query ShippingCarriers($channelId: ID!) {
    shippingCarriers(channelId: $channelId) {
      carrierSlug
      displayName
      isEnabled
      sortOrder
      countryMode
      allowedCountries
      supportsTracking
      supportsLabels
    }
  }
`

const SET_ENABLED = gql`
  mutation SetShippingCarrierEnabled($channelId: ID!, $carrierSlug: String!, $enabled: Boolean!) {
    setShippingCarrierEnabled(channelId: $channelId, carrierSlug: $carrierSlug, enabled: $enabled) {
      carrierSlug
      isEnabled
    }
  }
`

const SET_COUNTRIES = gql`
  mutation SetShippingCarrierCountries($channelId: ID!, $carrierSlug: String!, $mode: String!, $countries: [String!]!) {
    setShippingCarrierCountries(channelId: $channelId, carrierSlug: $carrierSlug, mode: $mode, countries: $countries) {
      carrierSlug
      countryMode
      allowedCountries
    }
  }
`

type ShippingCarrierAdmin = {
  carrierSlug: string
  displayName: string
  isEnabled: boolean
  sortOrder: number
  countryMode: string
  allowedCountries: string[]
  supportsTracking: boolean
  supportsLabels: boolean
}

// ShippingCarrierAvailability is the per-channel enablement + country-restriction
// block for shipping carriers, embedded in the Shipping settings section
// (Settings → Shipping) - Enablement/countries live in the shipping_carriers
// table (queried at checkout), so they're per-channel and managed here rather
// than as plain config fields - When `carrierSlug` is set (drilled into one
// carrier in the settings sidebar) it shows just that carrier; otherwise all
export function ShippingCarrierAvailability({ carrierSlug }: { carrierSlug?: string }) {
  const { effectiveChannelId, isChannel } = useConfigScope()

  const { data, loading, error } = useQuery<
    { shippingCarriers: ShippingCarrierAdmin[] },
    { channelId: string }
  >(SHIPPING_CARRIERS_QUERY, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
  })

  const carriers = useMemo(() => {
    let list = [...(data?.shippingCarriers ?? [])]
    if (carrierSlug) list = list.filter((c) => c.carrierSlug === carrierSlug)
    return list.sort(
      (a, b) => a.sortOrder - b.sortOrder || a.carrierSlug.localeCompare(b.carrierSlug),
    )
  }, [data, carrierSlug])

  return (
    <DetailPageSection
      title="Availability"
      description="Enable carriers and restrict which countries each ships to - per channel. Rates from every enabled carrier are offered side-by-side at checkout."
    >
      {!isChannel ? (
        <p className="text-sm text-muted-foreground">
          Availability is per channel. Switch the header scope to a channel to enable carriers
          and set country rules.
        </p>
      ) : loading && carriers.length === 0 ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : error ? (
        <Alert variant="destructive">
          <AlertTitle>Failed to load</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : carriers.length === 0 ? (
        <p className="text-sm text-muted-foreground">No carriers installed.</p>
      ) : (
        <div className="grid max-w-3xl gap-3">
          {carriers.map((c) => (
            <CarrierCard
              // Key includes the saved country config so the card remounts with
              // fresh local state after a refetch - avoids a sync-in-effect
              key={`${c.carrierSlug}:${c.countryMode}:${(c.allowedCountries ?? []).join(',')}`}
              carrier={c}
              channelId={effectiveChannelId as string}
            />
          ))}
        </div>
      )}
    </DetailPageSection>
  )
}

function CarrierCard({
  carrier,
  channelId,
}: {
  carrier: ShippingCarrierAdmin
  channelId: string
}) {
  const [mode, setMode] = useState<'all' | 'specific'>(
    carrier.countryMode === 'specific' ? 'specific' : 'all',
  )
  const [countriesText, setCountriesText] = useState((carrier.allowedCountries ?? []).join(', '))
  const [saveError, setSaveError] = useState<string | null>(null)

  const refetchQueries = [{ query: SHIPPING_CARRIERS_QUERY, variables: { channelId } }]

  const [setEnabled, { loading: toggling }] = useMutation<
    { setShippingCarrierEnabled: { isEnabled: boolean } },
    { channelId: string; carrierSlug: string; enabled: boolean }
  >(SET_ENABLED, { refetchQueries })

  const [setCountries, { loading: savingCountries }] = useMutation<
    unknown,
    { channelId: string; carrierSlug: string; mode: string; countries: string[] }
  >(SET_COUNTRIES, { refetchQueries })

  async function onToggle(next: boolean) {
    setSaveError(null)
    try {
      await setEnabled({ variables: { channelId, carrierSlug: carrier.carrierSlug, enabled: next } })
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Update failed.')
    }
  }

  async function onSaveCountries() {
    setSaveError(null)
    const countries =
      mode === 'specific'
        ? countriesText.split(',').map((c) => c.trim().toUpperCase()).filter(Boolean)
        : []
    try {
      await setCountries({ variables: { channelId, carrierSlug: carrier.carrierSlug, mode, countries } })
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Update failed.')
    }
  }

  const savedCountries = (carrier.allowedCountries ?? []).join(', ')
  const dirty =
    mode !== (carrier.countryMode === 'specific' ? 'specific' : 'all') ||
    (mode === 'specific' && countriesText.trim() !== savedCountries)

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="flex items-center gap-1.5 text-sm font-medium">
            {carrier.displayName}
            {carrier.supportsTracking && (
              <Badge variant="outline" className="text-[10px] uppercase">tracking</Badge>
            )}
            {carrier.supportsLabels && (
              <Badge variant="outline" className="text-[10px] uppercase">labels</Badge>
            )}
          </p>
          <p className="font-mono text-[10px] text-muted-foreground">{carrier.carrierSlug}</p>
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-xs text-muted-foreground">
            {carrier.isEnabled ? 'Enabled' : 'Disabled'}
          </Label>
          <Switch checked={carrier.isEnabled} disabled={toggling} onCheckedChange={onToggle} />
        </div>
      </div>

      <div className="mt-4 grid gap-3 border-t pt-3 sm:grid-cols-[200px_1fr]">
        <div className="grid gap-1">
          <Label className="text-xs">Ships to</Label>
          <Select value={mode} onValueChange={(v) => setMode(v as 'all' | 'specific')}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All countries</SelectItem>
              <SelectItem value="specific">Specific countries</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {mode === 'specific' && (
          <div className="grid gap-1">
            <Label htmlFor={`sc-countries-${carrier.carrierSlug}`} className="text-xs">
              Allowed countries (ISO-2, comma-separated)
            </Label>
            <Input
              id={`sc-countries-${carrier.carrierSlug}`}
              value={countriesText}
              onChange={(e) => setCountriesText(e.target.value)}
              placeholder="US, CA"
            />
          </div>
        )}
      </div>

      {saveError && (
        <Alert variant="destructive" className="mt-3">
          <AlertTitle>Update failed</AlertTitle>
          <AlertDescription>{saveError}</AlertDescription>
        </Alert>
      )}

      {dirty && (
        <div className="mt-3 flex justify-end">
          <Button size="sm" onClick={onSaveCountries} disabled={savingCountries}>
            {savingCountries ? 'Saving…' : 'Save restrictions'}
          </Button>
        </div>
      )}
    </Card>
  )
}
