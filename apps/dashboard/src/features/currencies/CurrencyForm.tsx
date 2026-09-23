import { useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
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
import {
  CreateCurrencyDocument,
  CurrenciesAdminListDocument,
  CurrencyDocument,
  UpdateCurrencyDocument,
} from './Currencies.gql'
import {
  CURRENCY_LABELS,
  currencyDiff,
  currencySchema,
  exchangeRateOverride,
  toCurrencyInput,
  toCurrencyValues,
  type CurrencyValues,
  type ExistingCurrency,
} from './schemas/currency'

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

function CurrencyFormPage({ mode, currency }: { mode: 'create' | 'edit'; currency?: ExistingCurrency }) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const refetchQueries = [{ query: CurrenciesAdminListDocument }]
  const [createCurrency] = useMutation(CreateCurrencyDocument, { refetchQueries })
  const [updateCurrency] = useMutation(UpdateCurrencyDocument, { refetchQueries })

  const baseline = useMemo(() => toCurrencyValues(currency), [currency])
  const record = useRecordForm({
    id: 'currency',
    title: 'Currency',
    label: isEdit ? `Currency · ${currency?.name ?? currency?.code ?? ''}` : 'New currency',
    isEdit,
    baseline,
    schema: currencySchema,
    diff: currencyDiff,
    persist: { routeKey: `currency:${currency?.code ?? 'new'}` },
    submit: async (value) => {
      const input = toCurrencyInput(value)
      if (currency) {
        await updateCurrency({
          variables: { input: { ...input, code: currency.code, ...exchangeRateOverride(value, currency.exchangeRate) } },
        })
      } else {
        await createCurrency({ variables: { input } })
      }
    },
    onSaved: () => navigate('/currencies'),
  })
  const { form, draft } = record
  const clash = (field: keyof CurrencyValues) => <ClashNote draft={draft} field={field} />

  return (
    <RecordFormPage
      record={record}
      backTo="/currencies"
      backLabel="Back to currencies"
      title={isEdit ? currency?.name ?? currency?.code ?? 'Currency' : 'New currency'}
      meta={
        isEdit && currency?.code ? (
          <code className="font-mono">{currency.code}</code>
        ) : !isEdit ? (
          <span>ISO 4217 currencies. Used for product pricing, channel defaults, and country fallbacks.</span>
        ) : null
      }
      createLabel="Create currency"
      labels={CURRENCY_LABELS}
      after={<PluginSections target="currency" entity={currency ?? null} mode={isEdit ? 'edit' : 'create'} />}
    >
      <DetailPageSection title="Identity" description="3-letter ISO 4217 code with display name.">
        <div className="grid gap-4">
          <form.AppField name="code">
            {(f) => (
              <f.TextField
                id="currency-code"
                label="Code"
                maxLength={3}
                disabled={isEdit}
                placeholder="USD"
                parse={(raw) => raw.toUpperCase()}
                hint={
                  isEdit
                    ? 'Code is immutable - delete and recreate if you need to change.'
                    : 'Uppercased as you type. Pattern: ^[A-Z]{3}$'
                }
              />
            )}
          </form.AppField>
          <form.AppField name="name">
            {(f) => <f.TextField id="currency-name" label="Name" placeholder="US Dollar" note={clash('name')} />}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Display" description="How this currency renders to customers.">
        <div className="grid gap-4">
          <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
            <form.AppField name="symbol">
              {(f) => <f.TextField id="currency-symbol" label="Symbol" maxLength={4} placeholder="$" note={clash('symbol')} />}
            </form.AppField>
            <form.Field name="symbolPosition">
              {(f) => (
                <div className="grid gap-2">
                  <Label htmlFor="currency-position">Symbol position</Label>
                  <form.Subscribe selector={(s) => s.values.symbol || '$'}>
                    {(symbol) => (
                      <Select value={f.state.value} onValueChange={(v) => f.handleChange(v as 'prefix' | 'suffix')}>
                        <SelectTrigger className="w-full" id="currency-position">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="prefix">Before - {symbol}9.99</SelectItem>
                          <SelectItem value="suffix">After - 9.99 {symbol}</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  </form.Subscribe>
                  {clash('symbolPosition')}
                </div>
              )}
            </form.Field>
            <form.AppField name="decimals">
              {(f) => (
                <f.NumberField
                  id="currency-decimals"
                  label="Decimals"
                  min={0}
                  max={4}
                  hint="USD/EUR = 2, JPY/KRW = 0. Most currencies use 2."
                  note={clash('decimals')}
                />
              )}
            </form.AppField>
            <form.AppField name="sortOrder">
              {(f) => <f.NumberField id="currency-sort" label="Sort order" note={clash('sortOrder')} />}
            </form.AppField>
            {isEdit && (
              <form.AppField name="exchangeRate">
                {(f) => (
                  <f.TextField
                    id="currency-rate"
                    label="Exchange rate"
                    type="number"
                    inputMode="decimal"
                    step="any"
                    min={0}
                    placeholder="auto"
                    hint={
                      <>
                        Units per 1 base currency.{' '}
                        {currency?.rateUpdatedAt
                          ? `Updated ${new Date(currency.rateUpdatedAt).toLocaleString()}.`
                          : 'Never fetched - the currency-rates job runs every 12h.'}
                      </>
                    }
                    note={clash('exchangeRate')}
                  />
                )}
              </form.AppField>
            )}
          </div>
          <div className="rounded-md border p-3">
            <form.AppField name="isActive">
              {(f) => (
                <f.CheckboxField id="currency-active" label="Active - inactive currencies cannot be assigned to new content" />
              )}
            </form.AppField>
          </div>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Metadata" description="Free-form key/value pairs for integration-specific data.">
        <form.AppField name="metadata">{(f) => <f.MetadataField note={clash('metadata')} />}</form.AppField>
      </DetailPageSection>
    </RecordFormPage>
  )
}
