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
import { LoadError } from '@/shared/errors/LoadError'
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
import {
  CreateCurrencyDocument,
  CurrenciesAdminListDocument,
  CurrencyDocument,
  UpdateCurrencyDocument,
} from './Currencies.gql'

// Backend column constraint: symbol_position ∈ {"prefix","suffix"} The form values must match that vocabulary verbatim - uppercase
// BEFORE/AFTER fails the service-side check in internal/currency/service.go with "symbol_position must be \"prefix\" or \"suffix\""
type SymbolPosition = 'prefix' | 'suffix'

type MetadataEntry = { key: string; value: string }

type ExistingCurrency = {
  code: string
  name: string
  symbol: string
  decimals: number
  symbolPosition: string
  isActive: boolean
  sortOrder: number
  metadata: MetadataEntry[]
  exchangeRate?: number | null
  rateUpdatedAt?: string | null
}

const CODE_REGEX = /^[A-Z]{3}$/

export function CurrencyCreatePage() {
  return <CurrencyFormPage mode="create" />
}

export function CurrencyEditPage() {
  const params = useParams<{ code?: string }>()
  const code = params.code ?? ''
  const { data, loading, error, refetch } = useQuery(CurrencyDocument, {
    variables: { code },
    skip: !code,
    notifyOnNetworkStatusChange: true,
  })

  if (loading) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading currency…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load currency"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.currency) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Currency not found.</p>
  }

  return <CurrencyFormPage mode="edit" currency={data.currency} />
}

function CurrencyFormPage({
  mode,
  currency,
}: {
  mode: 'create' | 'edit'
  currency?: ExistingCurrency
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const [code, setCode] = useState(currency?.code ?? '')
  const [name, setName] = useState(currency?.name ?? '')
  const [symbol, setSymbol] = useState(currency?.symbol ?? '')
  const [decimals, setDecimals] = useState(currency?.decimals ?? 2)
  const [symbolPosition, setSymbolPosition] = useState<SymbolPosition>(
    (currency?.symbolPosition as SymbolPosition) ?? 'prefix',
  )
  const [isActive, setIsActive] = useState(currency?.isActive ?? true)
  const [sortOrder, setSortOrder] = useState(currency?.sortOrder ?? 0)
  const [exchangeRate, setExchangeRate] = useState(
    currency?.exchangeRate != null ? String(currency.exchangeRate) : '',
  )
  const [metadata, setMetadata] = useState<MetadataEntry[]>(
    currency?.metadata.map((e) => ({ ...e })) ?? [],
  )
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [codeError, setCodeError] = useState<string | null>(null)

  const refetchQueries = [{ query: CurrenciesAdminListDocument }]
  const [createCurrency, { loading: creating }] = useMutation(CreateCurrencyDocument, {
    refetchQueries,
  })
  const [updateCurrency, { loading: updating }] = useMutation(UpdateCurrencyDocument, {
    refetchQueries,
  })
  const loading = creating || updating

  function cleanMetadata(entries: MetadataEntry[]): MetadataEntry[] {
    return entries
      .map((e) => ({ key: e.key.trim(), value: e.value.trim() }))
      .filter((e) => e.key.length > 0)
  }

  const draft = { name, symbol, decimals, symbolPosition, isActive, sortOrder, exchangeRate }
  const original = {
    name: currency?.name ?? '',
    symbol: currency?.symbol ?? '',
    decimals: currency?.decimals ?? 2,
    symbolPosition: (currency?.symbolPosition as SymbolPosition) ?? 'prefix',
    isActive: currency?.isActive ?? true,
    sortOrder: currency?.sortOrder ?? 0,
    exchangeRate: currency?.exchangeRate != null ? String(currency.exchangeRate) : '',
  }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const currencyDiff = useMemo(
    () =>
      isEdit
        ? fieldDiff(original, draft, [
            { label: 'Name', value: (o) => o.name },
            { label: 'Symbol', value: (o) => o.symbol },
            { label: 'Decimals', value: (o) => o.decimals },
            { label: 'Symbol position', value: (o) => o.symbolPosition },
            { label: 'Active', value: (o) => o.isActive },
            { label: 'Sort order', value: (o) => o.sortOrder },
            { label: 'Exchange rate', value: (o) => o.exchangeRate },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isEdit, originalKey, draftKey],
  )

  useSaveSection({ id: 'currency', title: 'Currency', diff: currencyDiff, save: () => applyChanges() })

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

  async function applyChanges() {
    setSubmitError(null)
    setCodeError(null)

    const trimmedCode = code.trim().toUpperCase()
    if (!isEdit) {
      if (!CODE_REGEX.test(trimmedCode)) {
        setCodeError('Use ISO 4217: exactly 3 uppercase letters (e.g. USD, EUR).')
        return
      }
    }

    const cleanedMetadata = cleanMetadata(metadata)
    const sharedFields = {
      name,
      symbol,
      decimals,
      symbolPosition,
      isActive,
      sortOrder,
      metadata: cleanedMetadata,
    }

    try {
      if (isEdit && currency) {
        const rate = Number(exchangeRate)
        await updateCurrency({
          variables: {
            input: {
              code: currency.code,
              ...sharedFields,
              // Only send a manual override when the operator changed it
              ...(exchangeRate && Number.isFinite(rate) && rate > 0 && rate !== currency.exchangeRate
                ? { exchangeRate: rate }
                : {}),
            },
          },
        })
      } else {
        await createCurrency({
          variables: { input: { code: trimmedCode, ...sharedFields } },
        })
      }
      navigate('/currencies')
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
      throw err // the dialog keeps itself open and surfaces the message
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/currencies"
        backLabel="Back to currencies"
        title={isEdit ? currency?.name ?? currency?.code ?? 'Currency' : 'New currency'}
        meta={
          isEdit && currency?.code ? (
            <code className="font-mono">{currency.code}</code>
          ) : !isEdit ? (
            <span>
              ISO 4217 currencies. Used for product pricing, channel defaults, and country
              fallbacks.
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
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create currency'}
          </Button>
        }
      />

      <div>
        <DetailPageBody>
          <DetailPageSection
            title="Identity"
            description="3-letter ISO 4217 code with display name."
          >
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="currency-code">Code</Label>
                <Input
                  id="currency-code"
                  required
                  maxLength={3}
                  value={code}
                  onChange={(e) => setCode(e.target.value.toUpperCase())}
                  disabled={isEdit}
                  placeholder="USD"
                  aria-invalid={!!codeError}
                  className={cn(codeError && 'border-destructive')}
                />
                <p className="text-xs text-muted-foreground">
                  {isEdit
                    ? 'Code is immutable - delete and recreate if you need to change.'
                    : 'Uppercased on save. Pattern: ^[A-Z]{3}$'}
                </p>
                {codeError && <p className="text-xs text-destructive">{codeError}</p>}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="currency-name">Name</Label>
                <Input
                  id="currency-name"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="US Dollar"
                />
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Display"
            description="How this currency renders to customers."
          >
            <div className="grid gap-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
                <div className="grid gap-2">
                  <Label htmlFor="currency-symbol">Symbol</Label>
                  <Input
                    id="currency-symbol"
                    maxLength={4}
                    value={symbol}
                    onChange={(e) => setSymbol(e.target.value)}
                    placeholder="$"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="currency-position">Symbol position</Label>
                  <Select
                    value={symbolPosition}
                    onValueChange={(v) => setSymbolPosition(v as SymbolPosition)}
                  >
                    <SelectTrigger className="w-full" id="currency-position">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="prefix">Before - {symbol || '$'}9.99</SelectItem>
                      <SelectItem value="suffix">After - 9.99 {symbol || '$'}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="currency-decimals">Decimals</Label>
                  <Input
                    id="currency-decimals"
                    type="number"
                    inputMode="numeric"
                    min={0}
                    max={4}
                    value={decimals}
                    onChange={(e) => setDecimals(Number(e.target.value) || 0)}
                  />
                  <p className="text-xs text-muted-foreground">
                    USD/EUR = 2, JPY/KRW = 0. Most currencies use 2.
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="currency-sort">Sort order</Label>
                  <Input
                    id="currency-sort"
                    type="number"
                    inputMode="numeric"
                    value={sortOrder}
                    onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
                  />
                </div>
                {isEdit && (
                  <div className="grid gap-2">
                    <Label htmlFor="currency-rate">Exchange rate</Label>
                    <Input
                      id="currency-rate"
                      type="number"
                      inputMode="decimal"
                      step="any"
                      min={0}
                      value={exchangeRate}
                      onChange={(e) => setExchangeRate(e.target.value)}
                      placeholder="auto"
                    />
                    <p className="text-xs text-muted-foreground">
                      Units per 1 base currency.{' '}
                      {currency?.rateUpdatedAt
                        ? `Updated ${new Date(currency.rateUpdatedAt).toLocaleString()}.`
                        : 'Never fetched - the currency-rates job runs every 12h.'}
                    </p>
                  </div>
                )}
              </div>
              <div className="flex items-center gap-2 rounded-md border p-3">
                <Checkbox
                  id="currency-active"
                  checked={isActive}
                  onCheckedChange={(v) => setIsActive(!!v)}
                />
                <Label htmlFor="currency-active" className="cursor-pointer">
                  Active - inactive currencies cannot be assigned to new content
                </Label>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Metadata"
            description="Free-form key/value pairs for integration-specific data."
          >
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
          target="currency"
          entity={currency ?? null}
          mode={isEdit ? 'edit' : 'create'}
        />
      </DetailPageBody>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Currency · ${currency?.name ?? currency?.code ?? ''}`}
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
