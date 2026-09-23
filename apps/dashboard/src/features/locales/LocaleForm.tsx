import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { LoadError } from '@/shared/errors/LoadError'
import {
  CreateLocaleDocument,
  LocaleDocument,
  LocalesAdminListDocument,
  UpdateLocaleDocument,
} from './Locales.gql'

type MetadataEntry = { key: string; value: string }

// Field-level error keys map 1:1 to backend message phrases - Updated when the
// backend message changes - surfaced inline next to the offending input
type FieldErrors = Partial<{
  code: string
  firstDayOfWeek: string
  defaultCountryCode: string
  hreflangValue: string
}>

// Inspect a backend error message and route it to a specific field, plus a general message for the alert - Anything we
// don't recognise becomes a general message only
function classifyError(message: string): { fieldErrors: FieldErrors; general: string | null } {
  const fieldErrors: FieldErrors = {}
  if (/first[_ ]?day[_ ]?of[_ ]?week/i.test(message) && /must be 0\.\.6/i.test(message)) {
    fieldErrors.firstDayOfWeek = 'First day of week must be 0..6.'
  }
  if (/default[_ ]?country[_ ]?code/i.test(message) && /ISO 3166-1 alpha-2/i.test(message)) {
    fieldErrors.defaultCountryCode = 'Must be empty or a 2-letter ISO 3166-1 country code.'
  }
  if (/hreflang[_ ]?value/i.test(message) && /BCP[_ ]?47/i.test(message)) {
    fieldErrors.hreflangValue = "Doesn't look like a BCP 47 code."
  }
  return { fieldErrors, general: Object.keys(fieldErrors).length === 0 ? message : null }
}

const CODE_REGEX = /^[a-z]{2,3}(-[a-z0-9]{2,8})*$/

const WEEKDAY_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

type ExistingLocale = {
  code: string
  name: string
  nativeName: string
  description: string
  isActive: boolean
  isRTL: boolean
  sortOrder: number
  flagEmoji: string
  dateFormat: string
  timeFormat: string
  decimalSeparator: string
  thousandsSeparator: string
  firstDayOfWeek: number
  defaultCountryCode: string
  hreflangValue: string
  script: string
  pluralRule: string
  emailFromNameOverride: string
  metadata: MetadataEntry[]
}

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

function LocaleFormPage({
  mode,
  locale,
}: {
  mode: 'create' | 'edit'
  locale?: ExistingLocale
}) {
  const countryOptions = useQuery(CountriesPickerDocument).data?.countries ?? []
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const [code, setCode] = useState(locale?.code ?? '')
  const [name, setName] = useState(locale?.name ?? '')
  const [nativeName, setNativeName] = useState(locale?.nativeName ?? '')
  const [description, setDescription] = useState(locale?.description ?? '')

  const [isActive, setIsActive] = useState(locale?.isActive ?? true)
  const [isRTL, setIsRTL] = useState(locale?.isRTL ?? false)
  const [sortOrder, setSortOrder] = useState(locale?.sortOrder ?? 0)
  const [flagEmoji, setFlagEmoji] = useState(locale?.flagEmoji ?? '')

  const [dateFormat, setDateFormat] = useState(locale?.dateFormat ?? '')
  const [timeFormat, setTimeFormat] = useState(locale?.timeFormat ?? '')
  const [decimalSeparator, setDecimalSeparator] = useState(locale?.decimalSeparator ?? '.')
  const [thousandsSeparator, setThousandsSeparator] = useState(locale?.thousandsSeparator ?? ',')
  const [firstDayOfWeek, setFirstDayOfWeek] = useState(locale?.firstDayOfWeek ?? 1)

  const [defaultCountryCode, setDefaultCountryCode] = useState(locale?.defaultCountryCode ?? '')
  const [hreflangValue, setHreflangValue] = useState(locale?.hreflangValue ?? '')

  const [script, setScript] = useState(locale?.script ?? '')
  const [pluralRule, setPluralRule] = useState(locale?.pluralRule ?? '')
  const [emailFromNameOverride, setEmailFromNameOverride] = useState(
    locale?.emailFromNameOverride ?? '',
  )

  const [metadata, setMetadata] = useState<MetadataEntry[]>(
    locale?.metadata.map((e) => ({ ...e })) ?? [],
  )

  const [submitError, setSubmitError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const refetchQueries = [{ query: LocalesAdminListDocument }]
  const [createLocale, { loading: creating }] = useMutation(CreateLocaleDocument, { refetchQueries })
  const [updateLocale, { loading: updating }] = useMutation(UpdateLocaleDocument, { refetchQueries })
  const loading = creating || updating

  function cleanMetadata(entries: MetadataEntry[]): MetadataEntry[] {
    return entries
      .map((e) => ({ key: e.key.trim(), value: e.value.trim() }))
      .filter((e) => e.key.length > 0)
  }

  const draft = {
    name, nativeName, description, isActive, isRTL, sortOrder, flagEmoji, dateFormat, timeFormat,
    decimalSeparator, thousandsSeparator, firstDayOfWeek, defaultCountryCode, hreflangValue,
    script, pluralRule, emailFromNameOverride,
  }
  const original = {
    name: locale?.name ?? '',
    nativeName: locale?.nativeName ?? '',
    description: locale?.description ?? '',
    isActive: locale?.isActive ?? true,
    isRTL: locale?.isRTL ?? false,
    sortOrder: locale?.sortOrder ?? 0,
    flagEmoji: locale?.flagEmoji ?? '',
    dateFormat: locale?.dateFormat ?? '',
    timeFormat: locale?.timeFormat ?? '',
    decimalSeparator: locale?.decimalSeparator ?? '.',
    thousandsSeparator: locale?.thousandsSeparator ?? ',',
    firstDayOfWeek: locale?.firstDayOfWeek ?? 1,
    defaultCountryCode: locale?.defaultCountryCode ?? '',
    hreflangValue: locale?.hreflangValue ?? '',
    script: locale?.script ?? '',
    pluralRule: locale?.pluralRule ?? '',
    emailFromNameOverride: locale?.emailFromNameOverride ?? '',
  }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const localeDiff = useMemo(
    () =>
      isEdit
        ? fieldDiff(original, draft, [
            { label: 'Name', value: (o) => o.name },
            { label: 'Native name', value: (o) => o.nativeName },
            { label: 'Description', value: (o) => o.description },
            { label: 'Active', value: (o) => o.isActive },
            { label: 'Right to left', value: (o) => o.isRTL },
            { label: 'Sort order', value: (o) => o.sortOrder },
            { label: 'Flag', value: (o) => o.flagEmoji },
            { label: 'Date format', value: (o) => o.dateFormat },
            { label: 'Time format', value: (o) => o.timeFormat },
            { label: 'Decimal separator', value: (o) => o.decimalSeparator },
            { label: 'Thousands separator', value: (o) => o.thousandsSeparator },
            { label: 'First day of week', value: (o) => o.firstDayOfWeek },
            { label: 'Default country', value: (o) => o.defaultCountryCode },
            { label: 'hreflang value', value: (o) => o.hreflangValue },
            { label: 'Script', value: (o) => o.script },
            { label: 'Plural rule', value: (o) => o.pluralRule },
            { label: 'Email from-name override', value: (o) => o.emailFromNameOverride },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isEdit, originalKey, draftKey],
  )

  useSaveSection({ id: 'locale', title: 'Locale', diff: localeDiff, save: () => applyChanges() })

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
    setFieldErrors({})

    const trimmedCode = code.trim().toLowerCase()
    if (!isEdit) {
      if (!trimmedCode) {
        setFieldErrors({ code: 'Code is required.' })
        return
      }
      if (!CODE_REGEX.test(trimmedCode)) {
        setFieldErrors({ code: 'Use BCP-47 style: lowercase, 2-3 letters then optional -region (e.g. en, es-mx, pt-br).' })
        return
      }
    }

    const trimmedCountry = defaultCountryCode.trim().toUpperCase()
    if (trimmedCountry && !/^[A-Z]{2}$/.test(trimmedCountry)) {
      setFieldErrors({ defaultCountryCode: 'Must be empty or exactly 2 letters.' })
      return
    }

    const cleanedMetadata = cleanMetadata(metadata)

    const sharedFields = {
      name,
      nativeName: nativeName || undefined,
      description: description || '',
      isActive,
      isRTL,
      sortOrder,
      flagEmoji: flagEmoji || '',
      dateFormat: dateFormat || '',
      timeFormat: timeFormat || '',
      decimalSeparator: decimalSeparator || '.',
      thousandsSeparator: thousandsSeparator || ',',
      firstDayOfWeek,
      defaultCountryCode: trimmedCountry,
      hreflangValue: hreflangValue || '',
      script: script || '',
      pluralRule: pluralRule || '',
      emailFromNameOverride: emailFromNameOverride || '',
      metadata: cleanedMetadata,
    }

    try {
      if (isEdit && locale) {
        await updateLocale({
          variables: { input: { code: locale.code, ...sharedFields } },
        })
      } else {
        await createLocale({
          variables: { input: { code: trimmedCode, ...sharedFields } },
        })
      }
      navigate('/locales')
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Something went wrong.'
      const { fieldErrors: fe, general } = classifyError(message)
      setFieldErrors(fe)
      setSubmitError(general ?? (Object.keys(fe).length === 0 ? message : null))
      throw err // the dialog keeps itself open and surfaces the message
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/locales"
        backLabel="Back to languages"
        title={isEdit ? locale?.name ?? locale?.code ?? 'Language' : 'New language'}
        meta={
          isEdit && locale?.code ? (
            <code className="font-mono">{locale.code}</code>
          ) : !isEdit ? (
            <span>
              Locale codes the system accepts. Assign them to channels from the channel's
              edit page.
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
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create language'}
          </Button>
        }
      />

      <div>
        <DetailPageBody>
          <DetailPageSection
            title="Identity"
            description="BCP 47 code with English and native names. Code is the primary key."
          >
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="locale-code">Code</Label>
                <Input
                  id="locale-code"
                  required
                  maxLength={20}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  disabled={isEdit}
                  placeholder="en, es-mx, pt-br"
                  aria-invalid={!!fieldErrors.code}
                  className={cn(fieldErrors.code && 'border-destructive')}
                />
                <p className="text-xs text-muted-foreground">
                  {isEdit
                    ? 'Code is immutable - delete and recreate if you need to rename.'
                    : 'Lowercased on save. Pattern: ^[a-z]{2,3}(-[a-z0-9]{2,8})*$'}
                </p>
                {fieldErrors.code && (
                  <p className="text-xs text-destructive">{fieldErrors.code}</p>
                )}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-name">Name</Label>
                <Input
                  id="locale-name"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Spanish (Mexico)"
                />
                <p className="text-xs text-muted-foreground">
                  English label shown in admin lists.
                </p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-native">Native name</Label>
                <Input
                  id="locale-native"
                  value={nativeName}
                  onChange={(e) => setNativeName(e.target.value)}
                  placeholder="Español (México)"
                />
                <p className="text-xs text-muted-foreground">
                  Shown to customers in locale pickers. Defaults to Name if left empty.
                </p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-description">Description</Label>
                <Textarea
                  id="locale-description"
                  rows={3}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Optional internal notes about this locale."
                />
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection title="Display & state">
            <div className="grid gap-4">
              <div className="grid items-start grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor="locale-flag">Flag emoji</Label>
                  <Input
                    id="locale-flag"
                    maxLength={8}
                    value={flagEmoji}
                    onChange={(e) => setFlagEmoji(e.target.value)}
                    placeholder="🇫🇷"
                  />
                  <p className="text-xs text-muted-foreground">
                    Single emoji shown in language switcher.
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="locale-sort">Sort order</Label>
                  <Input
                    id="locale-sort"
                    type="number"
                    inputMode="numeric"
                    value={sortOrder}
                    onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
                  />
                  <p className="text-xs text-muted-foreground">
                    Lower values appear first.
                  </p>
                </div>
              </div>
              <div className="grid gap-2 rounded-md border p-3">
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="locale-active"
                    checked={isActive}
                    onCheckedChange={(v) => setIsActive(!!v)}
                  />
                  <Label htmlFor="locale-active" className="cursor-pointer">
                    Active - inactive locales cannot be assigned to new content
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="locale-rtl"
                    checked={isRTL}
                    onCheckedChange={(v) => setIsRTL(!!v)}
                  />
                  <Label htmlFor="locale-rtl" className="cursor-pointer">
                    Right-to-left script
                  </Label>
                </div>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Formatting"
            description="Powers date/number rendering on the channel. Empty = use library defaults."
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
              <div className="grid gap-2">
                <Label htmlFor="locale-date">Date format</Label>
                <Input
                  id="locale-date"
                  value={dateFormat}
                  onChange={(e) => setDateFormat(e.target.value)}
                  placeholder="DD.MM.YYYY"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-time">Time format</Label>
                <Input
                  id="locale-time"
                  value={timeFormat}
                  onChange={(e) => setTimeFormat(e.target.value)}
                  placeholder="HH:mm"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-decimal">Decimal separator</Label>
                <Input
                  id="locale-decimal"
                  maxLength={1}
                  value={decimalSeparator}
                  onChange={(e) => setDecimalSeparator(e.target.value)}
                  placeholder="."
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-thousands">Thousands separator</Label>
                <Input
                  id="locale-thousands"
                  maxLength={1}
                  value={thousandsSeparator}
                  onChange={(e) => setThousandsSeparator(e.target.value)}
                  placeholder=","
                />
              </div>
              <div className="grid gap-2 sm:col-span-2">
                <Label htmlFor="locale-fdow">First day of week</Label>
                <Select
                  value={String(firstDayOfWeek)}
                  onValueChange={(v) => setFirstDayOfWeek(Number(v))}
                >
                  <SelectTrigger
                    id="locale-fdow"
                    aria-invalid={!!fieldErrors.firstDayOfWeek}
                    className={cn('w-full', fieldErrors.firstDayOfWeek && 'border-destructive')}
                  >
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
                {fieldErrors.firstDayOfWeek && (
                  <p className="text-xs text-destructive">{fieldErrors.firstDayOfWeek}</p>
                )}
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Geography & SEO"
            description="Affects fallback country and hreflang link tags on the channel."
          >
            <div className="grid items-start grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="locale-country">Default country code</Label>
                <Select
                  value={defaultCountryCode || '__none__'}
                  onValueChange={(v) => setDefaultCountryCode(v === '__none__' ? '' : v)}
                >
                  <SelectTrigger
                    id="locale-country"
                    className={cn('w-full', fieldErrors.defaultCountryCode && 'border-destructive')}
                  >
                    <SelectValue placeholder="Country" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">- No country -</SelectItem>
                    {countryOptions.map((c) => (
                      <SelectItem key={c.code} value={c.code}>
                        {c.emojiFlag ? `${c.emojiFlag} ` : ''}
                        {c.name}
                        <span className="ml-1 text-xs text-muted-foreground">({c.code})</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  ISO 3166-1 alpha-2 (uppercased on save). Empty for none.
                </p>
                {fieldErrors.defaultCountryCode && (
                  <p className="text-xs text-destructive">{fieldErrors.defaultCountryCode}</p>
                )}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-hreflang">hreflang value</Label>
                <Input
                  id="locale-hreflang"
                  value={hreflangValue}
                  onChange={(e) => setHreflangValue(e.target.value)}
                  placeholder="es-MX"
                  aria-invalid={!!fieldErrors.hreflangValue}
                  className={cn(fieldErrors.hreflangValue && 'border-destructive')}
                />
                <p className="text-xs text-muted-foreground">
                  BCP 47. Empty falls back to the locale code.
                </p>
                {fieldErrors.hreflangValue && (
                  <p className="text-xs text-destructive">{fieldErrors.hreflangValue}</p>
                )}
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Niche"
            description="Rarely needed. Set only if you have a specific reason."
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="locale-script">Script</Label>
                <Input
                  id="locale-script"
                  value={script}
                  onChange={(e) => setScript(e.target.value)}
                  placeholder="Latn, Cyrl, Arab"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="locale-plural">Plural rule</Label>
                <Input
                  id="locale-plural"
                  value={pluralRule}
                  onChange={(e) => setPluralRule(e.target.value)}
                  placeholder="CLDR plural category"
                />
              </div>
              <div className="grid gap-2 sm:col-span-2">
                <Label htmlFor="locale-from-override">Email "from" name override</Label>
                <Input
                  id="locale-from-override"
                  value={emailFromNameOverride}
                  onChange={(e) => setEmailFromNameOverride(e.target.value)}
                  placeholder="Empty = inherit from channel"
                />
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Metadata"
            description="Free-form key/value pairs for integration-specific data."
          >
            <KeyValueEditor entries={metadata} onChange={setMetadata} />
          </DetailPageSection>

          {(submitError || (Object.keys(fieldErrors).length > 0)) && (
            <div className="px-6 pb-4">
              {submitError ? (
                <Alert variant="destructive">
                  <AlertTitle>{isEdit ? 'Update failed' : 'Create failed'}</AlertTitle>
                  <AlertDescription>{submitError}</AlertDescription>
                </Alert>
              ) : (
                <Alert variant="destructive">
                  <AlertTitle>Fix the highlighted fields and try again</AlertTitle>
                  <AlertDescription>
                    {Object.values(fieldErrors).filter(Boolean).join(' ')}
                  </AlertDescription>
                </Alert>
              )}
            </div>
          )}
        </DetailPageBody>
      </div>

      {/* Plugin sections render OUTSIDE the core form so their own
          state/buttons can't trip the locale submit */}
      <DetailPageBody className="border-t-0">
        <PluginSections
          target="locale"
          entity={locale ?? null}
          mode={isEdit ? 'edit' : 'create'}
        />
      </DetailPageBody>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Locale · ${locale?.name ?? locale?.code ?? ''}`}
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
