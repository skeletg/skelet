import { useMemo } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { FieldError } from '@/components/form/fields'
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
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import {
  CreateLocaleDocument,
  LocaleDocument,
  LocalesAdminListDocument,
  UpdateLocaleDocument,
} from './Locales.gql'
import {
  LOCALE_LABELS,
  WEEKDAY_LABELS,
  localeDiff,
  localeFieldErrors,
  localeSchema,
  toLocaleInput,
  toLocaleValues,
  type ExistingLocale,
  type LocaleValues,
} from './schemas/locale'

export function LocaleCreatePage() {
  return <LocaleFormPage mode="create" />
}

export function LocaleEditPage() {
  const params = useParams<{ code?: string }>()
  const code = params.code ?? ''
  const { data, loading, error, refetch } = useQuery(LocaleDocument, {
    variables: { code },
    skip: !code,
    notifyOnNetworkStatusChange: true,
  })

  if (loading) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading locale…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load locale"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.locale) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Locale not found.</p>
  }

  return <LocaleFormPage mode="edit" locale={data.locale} />
}

function LocaleFormPage({ mode, locale }: { mode: 'create' | 'edit'; locale?: ExistingLocale }) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'
  const countries = useQuery(CountriesPickerDocument).data?.countries ?? []

  const refetchQueries = [{ query: LocalesAdminListDocument }]
  const [createLocale] = useMutation(CreateLocaleDocument, { refetchQueries })
  const [updateLocale] = useMutation(UpdateLocaleDocument, { refetchQueries })

  const baseline = useMemo(() => toLocaleValues(locale), [locale])
  const record = useRecordForm({
    id: 'locale',
    title: 'Locale',
    label: isEdit ? `Locale · ${locale?.name ?? locale?.code ?? ''}` : 'New language',
    isEdit,
    baseline,
    schema: localeSchema,
    diff: localeDiff,
    persist: { routeKey: `locale:${locale?.code ?? 'new'}` },
    submit: async (value) => {
      const input = toLocaleInput(value)
      if (locale) await updateLocale({ variables: { input: { ...input, code: locale.code } } })
      else await createLocale({ variables: { input } })
    },
    onSaved: () => navigate('/locales'),
    fieldErrorsFrom: localeFieldErrors,
  })
  const { form, draft } = record
  const clash = (field: keyof LocaleValues) => <ClashNote draft={draft} field={field} />

  return (
    <RecordFormPage
      record={record}
      backTo="/locales"
      backLabel="Back to languages"
      title={isEdit ? locale?.name ?? locale?.code ?? 'Language' : 'New language'}
      meta={
        isEdit && locale?.code ? (
          <code className="font-mono">{locale.code}</code>
        ) : !isEdit ? (
          <span>Locale codes the system accepts. Assign them to channels from the channel's edit page.</span>
        ) : null
      }
      createLabel="Create language"
      labels={LOCALE_LABELS}
      // Plugin sections render OUTSIDE the core form so their own state/buttons can't trip the locale save
      after={<PluginSections target="locale" entity={locale ?? null} mode={isEdit ? 'edit' : 'create'} />}
    >
      <DetailPageSection
        title="Identity"
        description="BCP 47 code with English and native names. Code is the primary key."
      >
        <div className="grid gap-4">
          <form.AppField name="code">
            {(f) => (
              <f.TextField
                id="locale-code"
                label="Code"
                maxLength={20}
                disabled={isEdit}
                placeholder="en, es-mx, pt-br"
                parse={(raw) => raw.toLowerCase()}
                hint={
                  isEdit
                    ? 'Code is immutable - delete and recreate if you need to rename.'
                    : 'Lowercased as you type. Pattern: ^[a-z]{2,3}(-[a-z0-9]{2,8})*$'
                }
              />
            )}
          </form.AppField>
          <form.AppField name="name">
            {(f) => (
              <f.TextField
                id="locale-name"
                label="Name"
                placeholder="Spanish (Mexico)"
                hint="English label shown in admin lists."
                note={clash('name')}
              />
            )}
          </form.AppField>
          <form.AppField name="nativeName">
            {(f) => (
              <f.TextField
                id="locale-native"
                label="Native name"
                placeholder="Español (México)"
                hint="Shown to customers in locale pickers. Defaults to Name if left empty."
                note={clash('nativeName')}
              />
            )}
          </form.AppField>
          <form.AppField name="description">
            {(f) => (
              <f.TextareaField
                id="locale-description"
                label="Description"
                rows={3}
                placeholder="Optional internal notes about this locale."
                note={clash('description')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Display & state">
        <div className="grid gap-4">
          <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
            <form.AppField name="flagEmoji">
              {(f) => (
                <f.TextField
                  id="locale-flag"
                  label="Flag emoji"
                  maxLength={8}
                  placeholder="🇫🇷"
                  hint="Single emoji shown in language switcher."
                  note={clash('flagEmoji')}
                />
              )}
            </form.AppField>
            <form.AppField name="sortOrder">
              {(f) => (
                <f.NumberField id="locale-sort" label="Sort order" hint="Lower values appear first." note={clash('sortOrder')} />
              )}
            </form.AppField>
          </div>
          <div className="grid gap-2 rounded-md border p-3">
            <form.AppField name="isActive">
              {(f) => <f.CheckboxField id="locale-active" label="Active - inactive locales cannot be assigned to new content" />}
            </form.AppField>
            <form.AppField name="isRTL">
              {(f) => <f.CheckboxField id="locale-rtl" label="Right-to-left script" />}
            </form.AppField>
          </div>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Formatting"
        description="Powers date/number rendering on the channel. Empty = use library defaults."
      >
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="dateFormat">
            {(f) => <f.TextField id="locale-date" label="Date format" placeholder="DD.MM.YYYY" note={clash('dateFormat')} />}
          </form.AppField>
          <form.AppField name="timeFormat">
            {(f) => <f.TextField id="locale-time" label="Time format" placeholder="HH:mm" note={clash('timeFormat')} />}
          </form.AppField>
          <form.AppField name="decimalSeparator">
            {(f) => (
              <f.TextField
                id="locale-decimal"
                label="Decimal separator"
                maxLength={1}
                placeholder="."
                note={clash('decimalSeparator')}
              />
            )}
          </form.AppField>
          <form.AppField name="thousandsSeparator">
            {(f) => (
              <f.TextField
                id="locale-thousands"
                label="Thousands separator"
                maxLength={1}
                placeholder=","
                note={clash('thousandsSeparator')}
              />
            )}
          </form.AppField>
          <form.Field name="firstDayOfWeek">
            {(f) => (
              <div className="grid gap-2 sm:col-span-2">
                <Label htmlFor="locale-fdow">First day of week</Label>
                <Select value={String(f.state.value)} onValueChange={(v) => f.handleChange(Number(v))}>
                  <SelectTrigger id="locale-fdow" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {WEEKDAY_LABELS.map((label, idx) => (
                      <SelectItem key={idx} value={String(idx)}>
                        {idx} - {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError field={f} />
                {clash('firstDayOfWeek')}
              </div>
            )}
          </form.Field>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Geography & SEO"
        description="Affects fallback country and hreflang link tags on the channel."
      >
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.Field name="defaultCountryCode">
            {(f) => (
              <div className="grid gap-2">
                <Label htmlFor="locale-country">Default country code</Label>
                <Select
                  value={f.state.value || '__none__'}
                  onValueChange={(v) => f.handleChange(v === '__none__' ? '' : v)}
                >
                  <SelectTrigger id="locale-country" className="w-full">
                    <SelectValue placeholder="Country" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">- No country -</SelectItem>
                    {countries.map((c) => (
                      <SelectItem key={c.code} value={c.code}>
                        {c.emojiFlag ? `${c.emojiFlag} ` : ''}
                        {c.name}
                        <span className="ml-1 text-xs text-muted-foreground">({c.code})</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">ISO 3166-1 alpha-2. Empty for none.</p>
                <FieldError field={f} />
                {clash('defaultCountryCode')}
              </div>
            )}
          </form.Field>
          <form.AppField name="hreflangValue">
            {(f) => (
              <f.TextField
                id="locale-hreflang"
                label="hreflang value"
                placeholder="es-MX"
                hint="BCP 47. Empty falls back to the locale code."
                note={clash('hreflangValue')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Niche" description="Rarely needed. Set only if you have a specific reason.">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <form.AppField name="script">
            {(f) => <f.TextField id="locale-script" label="Script" placeholder="Latn, Cyrl, Arab" note={clash('script')} />}
          </form.AppField>
          <form.AppField name="pluralRule">
            {(f) => (
              <f.TextField id="locale-plural" label="Plural rule" placeholder="CLDR plural category" note={clash('pluralRule')} />
            )}
          </form.AppField>
          <form.AppField name="emailFromNameOverride">
            {(f) => (
              <f.TextField
                id="locale-from-override"
                label={'Email "from" name override'}
                placeholder="Empty = inherit from channel"
                className="sm:col-span-2"
                note={clash('emailFromNameOverride')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Metadata" description="Free-form key/value pairs for integration-specific data.">
        <form.AppField name="metadata">{(f) => <f.MetadataField note={clash('metadata')} />}</form.AppField>
      </DetailPageSection>
    </RecordFormPage>
  )
}
