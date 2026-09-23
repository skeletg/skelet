import { useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { PluginSections } from '@/plugins/detail-sections'
import { LoadError } from '@/shared/errors/LoadError'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import {
  CountriesAdminListDocument,
  CountryDocument,
  CreateCountryDocument,
  UpdateCountryDocument,
} from './Countries.gql'
import {
  COUNTRY_LABELS,
  countryDiff,
  countrySchema,
  toCountryInput,
  toCountryValues,
  type ExistingCountry,
} from './schemas/country'

const NO_CURRENCY = '__none__'

export function CountryCreatePage() {
  return <CountryFormPage mode="create" />
}

export function CountryEditPage() {
  const params = useParams<{ code?: string }>()
  const code = params.code ?? ''
  const { data, loading, error, refetch } = useQuery(CountryDocument, {
    variables: { code },
    skip: !code,
    notifyOnNetworkStatusChange: true,
  })

  if (loading) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading country…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load country"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.country) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Country not found.</p>
  }

  return <CountryFormPage mode="edit" country={data.country} />
}

function CountryFormPage({ mode, country }: { mode: 'create' | 'edit'; country?: ExistingCountry }) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const currenciesQuery = useQuery(CurrenciesPickerDocument)
  const currencies = useMemo(
    () => [...(currenciesQuery.data?.currencies ?? [])].sort((a, b) => a.code.localeCompare(b.code)),
    [currenciesQuery.data],
  )

  const refetchQueries = [{ query: CountriesAdminListDocument }]
  const [createCountry] = useMutation(CreateCountryDocument, { refetchQueries })
  const [updateCountry] = useMutation(UpdateCountryDocument, { refetchQueries })

  const baseline = useMemo(() => toCountryValues(country), [country])
  const record = useRecordForm({
    id: 'country',
    title: 'Country',
    label: isEdit ? `Country · ${country?.name ?? country?.code ?? ''}` : 'New country',
    isEdit,
    baseline,
    schema: countrySchema,
    diff: countryDiff,
    persist: { routeKey: `country:${country?.code ?? 'new'}` },
    submit: async (value) => {
      const input = toCountryInput(value)
      if (country) await updateCountry({ variables: { input: { ...input, code: country.code } } })
      else await createCountry({ variables: { input } })
    },
    onSaved: () => navigate('/countries'),
  })
  const { form, draft } = record
  const clash = (field: string) => <ClashNote draft={draft} field={field} />

  return (
    <RecordFormPage
      record={record}
      backTo="/countries"
      backLabel="Back to countries"
      title={isEdit ? country?.name ?? country?.code ?? 'Country' : 'New country'}
      meta={
        isEdit && country?.code ? (
          <code className="font-mono">{country.code}</code>
        ) : !isEdit ? (
          <span>ISO 3166-1 alpha-2 country with native name, flag, phone code, and a default currency.</span>
        ) : null
      }
      createLabel="Create country"
      labels={COUNTRY_LABELS}
      after={<PluginSections target="country" entity={country ?? null} mode={isEdit ? 'edit' : 'create'} />}
    >
      <DetailPageSection title="Identity" description="2-letter ISO 3166-1 alpha-2 code, name, and native name.">
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="code">
            {(f) => (
              <f.TextField
                id="country-code"
                label="Code"
                maxLength={2}
                disabled={isEdit}
                placeholder="US"
                parse={(raw) => raw.toUpperCase()}
                hint={isEdit ? 'Code is immutable.' : 'Uppercased as you type. Pattern: ^[A-Z]{2}$'}
              />
            )}
          </form.AppField>
          <form.AppField name="emojiFlag">
            {(f) => <f.TextField id="country-flag" label="Flag emoji" maxLength={8} placeholder="🇺🇸" note={clash('emojiFlag')} />}
          </form.AppField>
          <form.AppField name="name">
            {(f) => <f.TextField id="country-name" label="Name" placeholder="United States" note={clash('name')} />}
          </form.AppField>
          <form.AppField name="nativeName">
            {(f) => (
              <f.TextField id="country-native" label="Native name" placeholder="United States" note={clash('nativeName')} />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Defaults"
        description="Phone calling code and default currency. Default currency is FK-validated."
      >
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="phoneCode">
            {(f) => <f.TextField id="country-phone" label="Phone code" maxLength={6} placeholder="+1" note={clash('phoneCode')} />}
          </form.AppField>
          <form.Field name="defaultCurrencyCode">
            {(f) => (
              <div className="grid gap-2">
                <Label htmlFor="country-currency">Default currency</Label>
                <Select
                  value={f.state.value || NO_CURRENCY}
                  onValueChange={(v) => f.handleChange(v === NO_CURRENCY ? '' : v)}
                  disabled={currenciesQuery.loading}
                >
                  <SelectTrigger className="w-full" id="country-currency">
                    <SelectValue placeholder={currenciesQuery.loading ? 'Loading…' : 'No default'} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_CURRENCY}>No default</SelectItem>
                    {currencies.map((entry) => (
                      <SelectItem key={entry.code} value={entry.code}>
                        <span className="mr-1.5 font-mono">{entry.symbol || '·'}</span>
                        <span>{entry.name}</span>
                        <span className="ml-1.5 text-xs text-muted-foreground">({entry.code})</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  Picker shows active currencies. Manage the catalog on the Currencies page.
                </p>
                {clash('defaultCurrencyCode')}
              </div>
            )}
          </form.Field>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Display & state">
        <div className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <form.AppField name="sortOrder">
              {(f) => <f.NumberField id="country-sort" label="Sort order" note={clash('sortOrder')} />}
            </form.AppField>
          </div>
          <div className="rounded-md border p-3">
            <form.AppField name="isActive">
              {(f) => (
                <f.CheckboxField id="country-active" label="Active - inactive countries cannot be assigned to new addresses" />
              )}
            </form.AppField>
          </div>
        </div>
      </DetailPageSection>

      {country && <IsoReference country={country} />}

      <DetailPageSection title="Metadata">
        <form.AppField name="metadata">{(f) => <f.MetadataField note={clash('metadata')} />}</form.AppField>
      </DetailPageSection>
    </RecordFormPage>
  )
}

// Reference data from the country/region seed - shown, never edited here
function IsoReference({ country }: { country: ExistingCountry }) {
  return (
    <DetailPageSection
      title="ISO codes & subdivisions"
      description="Reference data, set by the country/region seed (data patches), not edited here."
    >
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label>ISO alpha-3</Label>
          <Input value={country.iso3Code || '-'} readOnly disabled />
        </div>
        <div className="grid gap-2">
          <Label>ISO numeric-3</Label>
          <Input value={country.numericCode || '-'} readOnly disabled />
        </div>
      </div>
      <div className="grid gap-1.5">
        <Label>Subdivisions (ISO 3166-2)</Label>
        {country.regions.length === 0 ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
            No subdivisions seeded - region is free-text for this country.
          </p>
        ) : (
          <div className="overflow-hidden rounded-md border">
            {country.regions.map((rg) => (
              <div key={rg.code} className="flex items-center gap-3 border-b px-3 py-1.5 text-sm last:border-b-0">
                <code className="w-16 shrink-0 font-mono text-xs text-muted-foreground">
                  {country.code}-{rg.code}
                </code>
                <span className="min-w-0 flex-1 truncate">{rg.name}</span>
                {!rg.isActive && <span className="text-[10px] text-muted-foreground">inactive</span>}
              </div>
            ))}
          </div>
        )}
        <p className="text-xs text-muted-foreground">
          {country.regions.length} subdivision{country.regions.length === 1 ? '' : 's'}. Managed via the region seed;
          extendable through a data patch.
        </p>
      </div>
    </DetailPageSection>
  )
}
