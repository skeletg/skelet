import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { PluginSections } from '@/plugins/detail-sections'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { LoadError } from '@/shared/errors/LoadError'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import {
  CountriesAdminListDocument,
  CountryDocument,
  CreateCountryDocument,
  UpdateCountryDocument,
} from './Countries.gql'

type MetadataEntry = { key: string; value: string }

type RegionEntry = { code: string; name: string; isActive: boolean; sortOrder: number }

type ExistingCountry = {
  code: string
  name: string
  nativeName: string
  emojiFlag: string
  phoneCode: string
  iso3Code: string
  numericCode: string
  defaultCurrencyCode: string
  isActive: boolean
  sortOrder: number
  metadata: MetadataEntry[]
  regions: RegionEntry[]
}

const CODE_REGEX = /^[A-Z]{2}$/
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

function CountryFormPage({
  mode,
  country,
}: {
  mode: 'create' | 'edit'
  country?: ExistingCountry
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const [code, setCode] = useState(country?.code ?? '')
  const [name, setName] = useState(country?.name ?? '')
  const [nativeName, setNativeName] = useState(country?.nativeName ?? '')
  const [emojiFlag, setEmojiFlag] = useState(country?.emojiFlag ?? '')
  const [phoneCode, setPhoneCode] = useState(country?.phoneCode ?? '')
  const [defaultCurrencyCode, setDefaultCurrencyCode] = useState(country?.defaultCurrencyCode ?? '')
  const [isActive, setIsActive] = useState(country?.isActive ?? true)
  const [sortOrder, setSortOrder] = useState(country?.sortOrder ?? 0)
  const [metadata, setMetadata] = useState<MetadataEntry[]>(
    country?.metadata.map((e) => ({ ...e })) ?? [],
  )

  const [submitError, setSubmitError] = useState<string | null>(null)
  const [codeError, setCodeError] = useState<string | null>(null)

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const currenciesQuery = useQuery(CurrenciesPickerDocument)
  const currencies = useMemo(
    () =>
      [...(currenciesQuery.data?.currencies ?? [])].sort((a, b) =>
        a.code.localeCompare(b.code),
      ),
    [currenciesQuery.data],
  )

  const refetchQueries = [{ query: CountriesAdminListDocument }]
  const [createCountry, { loading: creating }] = useMutation(CreateCountryDocument, {
    refetchQueries,
  })
  const [updateCountry, { loading: updating }] = useMutation(UpdateCountryDocument, {
    refetchQueries,
  })
  const loading = creating || updating

  const draft = { name, nativeName, emojiFlag, phoneCode, defaultCurrencyCode, isActive, sortOrder }
  const original = {
    name: country?.name ?? '',
    nativeName: country?.nativeName ?? '',
    emojiFlag: country?.emojiFlag ?? '',
    phoneCode: country?.phoneCode ?? '',
    defaultCurrencyCode: country?.defaultCurrencyCode ?? '',
    isActive: country?.isActive ?? true,
    sortOrder: country?.sortOrder ?? 0,
  }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const countryDiff = useMemo(
    () =>
      isEdit
        ? fieldDiff(original, draft, [
            { label: 'Name', value: (o) => o.name },
            { label: 'Native name', value: (o) => o.nativeName },
            { label: 'Flag', value: (o) => o.emojiFlag },
            { label: 'Phone code', value: (o) => o.phoneCode },
            { label: 'Default currency', value: (o) => o.defaultCurrencyCode },
            { label: 'Active', value: (o) => o.isActive },
            { label: 'Sort order', value: (o) => o.sortOrder },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isEdit, originalKey, draftKey],
  )

  useSaveSection({ id: 'country', title: 'Country', diff: countryDiff, save: () => applyChanges() })

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  function cleanMetadata(entries: MetadataEntry[]): MetadataEntry[] {
    return entries
      .map((e) => ({ key: e.key.trim(), value: e.value.trim() }))
      .filter((e) => e.key.length > 0)
  }

  async function applyChanges() {
    setSubmitError(null)
    setCodeError(null)

    const trimmedCode = code.trim().toUpperCase()
    if (!isEdit) {
      if (!CODE_REGEX.test(trimmedCode)) {
        setCodeError('Use ISO 3166-1 alpha-2: exactly 2 uppercase letters (e.g. US, DE).')
        throw new Error('Use ISO 3166-1 alpha-2: exactly 2 uppercase letters (e.g. US, DE).')
      }
    }

    const cleanedMetadata = cleanMetadata(metadata)
    const sharedFields = {
      name,
      nativeName: nativeName || undefined,
      emojiFlag: emojiFlag || undefined,
      phoneCode: phoneCode || undefined,
      defaultCurrencyCode: defaultCurrencyCode || undefined,
      isActive,
      sortOrder,
      metadata: cleanedMetadata,
    }

    try {
      if (isEdit && country) {
        await updateCountry({
          variables: { input: { code: country.code, ...sharedFields } },
        })
      } else {
        await createCountry({
          variables: { input: { code: trimmedCode, ...sharedFields } },
        })
      }
      navigate('/countries')
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
      throw err // the dialog keeps itself open and surfaces the message
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/countries"
        backLabel="Back to countries"
        title={isEdit ? country?.name ?? country?.code ?? 'Country' : 'New country'}
        meta={
          isEdit && country?.code ? (
            <code className="font-mono">{country.code}</code>
          ) : !isEdit ? (
            <span>
              ISO 3166-1 alpha-2 country with native name, flag, phone code, and a default
              currency.
            </span>
          ) : null
        }
        actions={
          <Button
            type="button"
            size="sm"
            disabled={loading || (isEdit && !isDirty)}
            onClick={() => (isEdit ? setConfirmingSave(true) : void applyChanges())}
          >
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create country'}
          </Button>
        }
      />

      <div>
        <DetailPageBody>
          <DetailPageSection
            title="Identity"
            description="2-letter ISO 3166-1 alpha-2 code, name, and native name."
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
              <div className="grid gap-2">
                <Label htmlFor="country-code">Code</Label>
                <Input
                  id="country-code"
                  required
                  maxLength={2}
                  value={code}
                  onChange={(e) => setCode(e.target.value.toUpperCase())}
                  disabled={isEdit}
                  placeholder="US"
                  aria-invalid={!!codeError}
                  className={cn(codeError && 'border-destructive')}
                />
                <p className="text-xs text-muted-foreground">
                  {isEdit
                    ? 'Code is immutable.'
                    : 'Uppercased on save. Pattern: ^[A-Z]{2}$'}
                </p>
                {codeError && <p className="text-xs text-destructive">{codeError}</p>}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="country-flag">Flag emoji</Label>
                <Input
                  id="country-flag"
                  maxLength={8}
                  value={emojiFlag}
                  onChange={(e) => setEmojiFlag(e.target.value)}
                  placeholder="🇺🇸"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="country-name">Name</Label>
                <Input
                  id="country-name"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="United States"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="country-native">Native name</Label>
                <Input
                  id="country-native"
                  value={nativeName}
                  onChange={(e) => setNativeName(e.target.value)}
                  placeholder="United States"
                />
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Defaults"
            description="Phone calling code and default currency. Default currency is FK-validated."
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
              <div className="grid gap-2">
                <Label htmlFor="country-phone">Phone code</Label>
                <Input
                  id="country-phone"
                  maxLength={6}
                  value={phoneCode}
                  onChange={(e) => setPhoneCode(e.target.value)}
                  placeholder="+1"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="country-currency">Default currency</Label>
                <Select
                  value={defaultCurrencyCode || NO_CURRENCY}
                  onValueChange={(v) => setDefaultCurrencyCode(v === NO_CURRENCY ? '' : v)}
                  disabled={currenciesQuery.loading}
                >
                  <SelectTrigger className="w-full" id="country-currency">
                    <SelectValue
                      placeholder={currenciesQuery.loading ? 'Loading…' : 'No default'}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_CURRENCY}>No default</SelectItem>
                    {currencies.map((entry) => (
                      <SelectItem key={entry.code} value={entry.code}>
                        <span className="mr-1.5 font-mono">{entry.symbol || '·'}</span>
                        <span>{entry.name}</span>
                        <span className="ml-1.5 text-xs text-muted-foreground">
                          ({entry.code})
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  Picker shows active currencies. Manage the catalog on the Currencies page.
                </p>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection title="Display & state">
            <div className="grid gap-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor="country-sort">Sort order</Label>
                  <Input
                    id="country-sort"
                    type="number"
                    inputMode="numeric"
                    value={sortOrder}
                    onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
                  />
                </div>
              </div>
              <div className="flex items-center gap-2 rounded-md border p-3">
                <Checkbox
                  id="country-active"
                  checked={isActive}
                  onCheckedChange={(v) => setIsActive(!!v)}
                />
                <Label htmlFor="country-active" className="cursor-pointer">
                  Active - inactive countries cannot be assigned to new addresses
                </Label>
              </div>
            </div>
          </DetailPageSection>

          {country && (
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
                      <div
                        key={rg.code}
                        className="flex items-center gap-3 border-b px-3 py-1.5 text-sm last:border-b-0"
                      >
                        <code className="w-16 shrink-0 font-mono text-xs text-muted-foreground">
                          {country.code}-{rg.code}
                        </code>
                        <span className="min-w-0 flex-1 truncate">{rg.name}</span>
                        {!rg.isActive && (
                          <span className="text-[10px] text-muted-foreground">inactive</span>
                        )}
                      </div>
                    ))}
                  </div>
                )}
                <p className="text-xs text-muted-foreground">
                  {country.regions.length} subdivision{country.regions.length === 1 ? '' : 's'}.
                  Managed via the region seed; extendable through a data patch.
                </p>
              </div>
            </DetailPageSection>
          )}

          <DetailPageSection title="Metadata">
            <KeyValueEditor entries={metadata} onChange={setMetadata} />
          </DetailPageSection>

          {submitError && (
            <div className="px-6 pb-4">
              <Alert variant="destructive">
                <AlertTitle>{isEdit ? 'Update failed' : 'Create failed'}</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          )}
        </DetailPageBody>
      </div>

      <DetailPageBody className="border-t-0">
        <PluginSections
          target="country"
          entity={country ?? null}
          mode={isEdit ? 'edit' : 'create'}
        />
      </DetailPageBody>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Country · ${country?.name ?? country?.code ?? ''}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

function KeyValueEditor({
  entries,
  onChange,
}: {
  entries: MetadataEntry[]
  onChange: (entries: MetadataEntry[]) => void
}) {
  function update(idx: number, patch: Partial<MetadataEntry>) {
    onChange(entries.map((e, i) => (i === idx ? { ...e, ...patch } : e)))
  }
  function remove(idx: number) {
    onChange(entries.filter((_, i) => i !== idx))
  }
  function add() {
    onChange([...entries, { key: '', value: '' }])
  }

  return (
    <div className="grid gap-2">
      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground">No entries.</p>
      ) : (
        <div className="grid gap-2">
          {entries.map((entry, idx) => (
            <div key={idx} className="grid grid-cols-[1fr_2fr_auto] gap-2">
              <Input
                value={entry.key}
                onChange={(e) => update(idx, { key: e.target.value })}
                placeholder="key"
              />
              <Input
                value={entry.value}
                onChange={(e) => update(idx, { value: e.target.value })}
                placeholder="value"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => remove(idx)}
                aria-label="Remove entry"
              >
                <Trash2Icon className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <Button type="button" variant="outline" size="sm" onClick={add} className="w-fit">
        <PlusIcon className="size-4" />
        Add entry
      </Button>
    </div>
  )
}
