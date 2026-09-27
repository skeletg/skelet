import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import {
  PaymentMethodsDocument,
  SetPaymentMethodCountriesDocument,
  SetPaymentMethodEnabledDocument,
  type PaymentMethodsQuery,
} from './Payments.gql'

type PaymentMethodAdmin = PaymentMethodsQuery['paymentMethods'][number]

// PaymentAvailability is the per-channel enablement + country-restriction block
// for payment providers, embedded in the Payment settings section (Settings →
// Payment) - Enablement/countries live in the payment_methods table (queried at
// checkout), so they're per-channel and managed here rather than as plain
// config fields - When `providerSlug` is set (drilled into one provider in the
// settings sidebar) it shows just that provider; otherwise all of them
export function PaymentAvailability({ providerSlug }: { providerSlug?: string }) {
  const { effectiveChannelId, isChannel } = useConfigScope()

  const { data, loading, error } = useQuery(PaymentMethodsDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
  })

  const methods = useMemo(() => {
    let list = [...(data?.paymentMethods ?? [])]
    if (providerSlug) list = list.filter((m) => m.providerSlug === providerSlug)
    return list.sort(
      (a, b) => a.sortOrder - b.sortOrder || a.providerSlug.localeCompare(b.providerSlug),
    )
  }, [data, providerSlug])

  return (
    <DetailPageSection
      title="Availability"
      description="Enable providers and restrict which countries each accepts at checkout - per channel."
    >
      {!isChannel ? (
        <p className="text-sm text-muted-foreground">
          Availability is per channel. Switch the header scope to a channel to enable providers
          and set country rules.
        </p>
      ) : loading && methods.length === 0 ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : error ? (
        <Alert variant="destructive">
          <AlertTitle>Failed to load</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : methods.length === 0 ? (
        <p className="text-sm text-muted-foreground">No payment providers installed.</p>
      ) : (
        <div className="grid max-w-3xl gap-3">
          {methods.map((m) => (
            <ProviderCard
              // Key includes the saved country config so the card remounts with
              // fresh local state after a refetch - avoids a sync-in-effect
              key={`${m.providerSlug}:${m.countryMode}:${(m.allowedCountries ?? []).join(',')}`}
              method={m}
              channelId={effectiveChannelId as string}
            />
          ))}
        </div>
      )}
    </DetailPageSection>
  )
}

function ProviderCard({
  method,
  channelId,
}: {
  method: PaymentMethodAdmin
  channelId: string
}) {
  const [mode, setMode] = useState<'all' | 'specific'>(
    method.countryMode === 'specific' ? 'specific' : 'all',
  )
  const [countriesText, setCountriesText] = useState((method.allowedCountries ?? []).join(', '))
  const [saveError, setSaveError] = useState<string | null>(null)

  const refetchQueries = [{ query: PaymentMethodsDocument, variables: { channelId } }]

  const [setEnabled, { loading: toggling }] = useMutation(SetPaymentMethodEnabledDocument, { refetchQueries })

  const [setCountries, { loading: savingCountries }] = useMutation(SetPaymentMethodCountriesDocument, { refetchQueries })

  async function onToggle(next: boolean) {
    setSaveError(null)
    try {
      await setEnabled({ variables: { channelId, providerSlug: method.providerSlug, enabled: next } })
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
      await setCountries({ variables: { channelId, providerSlug: method.providerSlug, mode, countries } })
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Update failed.')
    }
  }

  const savedCountries = (method.allowedCountries ?? []).join(', ')
  const dirty =
    mode !== (method.countryMode === 'specific' ? 'specific' : 'all') ||
    (mode === 'specific' && countriesText.trim() !== savedCountries)

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium">{method.displayName}</p>
          <p className="font-mono text-[10px] text-muted-foreground">{method.providerSlug}</p>
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-xs text-muted-foreground">
            {method.isEnabled ? 'Enabled' : 'Disabled'}
          </Label>
          <Switch checked={method.isEnabled} disabled={toggling} onCheckedChange={onToggle} />
        </div>
      </div>

      <div className="mt-4 grid gap-3 border-t pt-3 sm:grid-cols-[200px_1fr]">
        <div className="grid gap-1">
          <Label className="text-xs">Accepted in</Label>
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
            <Label htmlFor={`pm-countries-${method.providerSlug}`} className="text-xs">
              Allowed countries (ISO-2, comma-separated)
            </Label>
            <Input
              id={`pm-countries-${method.providerSlug}`}
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
