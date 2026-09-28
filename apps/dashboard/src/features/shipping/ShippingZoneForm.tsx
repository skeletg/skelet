import { useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { useAppForm } from '@/components/form/form'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { sameValue } from '@/lib/form-drafts'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { LoadError } from '@/shared/errors/LoadError'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { submitForm } from '@/shared/saving/useFormSection'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import {
  CreateShippingMethodDocument,
  CreateShippingZoneDocument,
  DeleteShippingMethodDocument,
  ShippingZoneDocument,
  UpdateShippingMethodDocument,
  UpdateShippingZoneDocument,
} from './Shipping.gql'
import {
  methodSchema,
  toMethodInput,
  toMethodValues,
  type CalcType,
  type ExistingMethod,
} from './schemas/method'
import {
  ZONE_LABELS,
  toZoneInput,
  toZoneValues,
  zoneDiff,
  zoneSchema,
  type ExistingZone,
  type ZoneValues,
} from './schemas/zone'

export function ShippingZoneCreatePage() {
  const [params] = useSearchParams()
  return <ShippingZoneFormPage mode="create" channelId={params.get('channelId') ?? ''} />
}

export function ShippingZoneEditPage({ zoneId }: { zoneId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = zoneId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery(ShippingZoneDocument, {
    variables: { id },
    skip: !id,
    fetchPolicy: 'cache-and-network',
  })

  if (loading && !data) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading zone…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError title="Couldn't load shipping zone" message={error.message} onRetry={() => void refetch()} retrying={loading} />
      </div>
    )
  }
  if (!data?.shippingZone) return <p className="px-6 py-6 text-sm text-muted-foreground">Zone not found.</p>
  return (
    <ShippingZoneFormPage
      mode="edit"
      zone={data.shippingZone}
      methods={data.shippingZone.methods}
      channelId={data.shippingZone.channelId}
      onRefetch={() => void refetch()}
    />
  )
}

function ShippingZoneFormPage({
  mode,
  zone,
  methods = [],
  channelId,
  onRefetch,
}: {
  mode: 'create' | 'edit'
  zone?: ExistingZone
  methods?: ExistingMethod[]
  channelId: string
  onRefetch?: () => void
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'
  const [createZone] = useMutation(CreateShippingZoneDocument)
  const [updateZone] = useMutation(UpdateShippingZoneDocument)
  // A new zone opens on its own page so its methods can be added - the id comes back from create
  const createdId = useRef<string | null>(null)

  const baseline = useMemo(() => toZoneValues(zone), [zone])
  const record = useRecordForm({
    id: 'shipping-zone',
    title: 'Shipping zone',
    label: isEdit ? `Shipping zone · ${zone?.name || zone?.code || ''}` : 'New shipping zone',
    isEdit,
    baseline,
    schema: zoneSchema,
    diff: zoneDiff,
    persist: { routeKey: `shipping-zone:${zone?.id ?? `new:${channelId}`}` },
    submit: async (value) => {
      const input = toZoneInput(value)
      if (zone) {
        await updateZone({ variables: { input: { id: zone.id, ...input } } })
        return
      }
      if (!channelId) throw new Error('channelId missing - open this from the shipping list page.')
      const res = await createZone({ variables: { input: { channelId, ...input } } })
      createdId.current = res.data?.createShippingZone.id ?? null
    },
    onSaved: () => {
      if (isEdit) onRefetch?.()
      else navigate(createdId.current ? `/shipping/${createdId.current}` : '/shipping')
    },
  })
  const { form, draft } = record
  const clash = (field: keyof ZoneValues) => <ClashNote draft={draft} field={field} />

  return (
    <RecordFormPage
      record={record}
      backTo="/shipping"
      backLabel="Back to shipping"
      title={isEdit ? zone?.name || zone?.code || '(unnamed)' : 'New shipping zone'}
      meta="A zone groups countries served by the same set of methods."
      createLabel="Create zone"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/shipping">Cancel</Link>
        </Button>
      }
      labels={ZONE_LABELS}
    >
      <DetailPageSection
        title="Identity"
        description="Code is stable across renames - used by carrier-integration plugins as a lookup key."
      >
        <div className="grid items-start gap-4 sm:grid-cols-2">
          <form.AppField name="code">
            {(f) => <f.TextField id="code" label="Code" placeholder="us-domestic" note={clash('code')} />}
          </form.AppField>
          <form.AppField name="name">
            {(f) => <f.TextField id="name" label="Display name" placeholder="United States" note={clash('name')} />}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Countries"
        description="ISO 3166-1 alpha-2 codes, comma-separated. The order resolver picks the first matching active zone (sort order tie-breaks)."
      >
        <form.AppField name="countries">
          {(f) => (
            <f.TextField
              aria-label="Countries"
              placeholder="US, CA, MX"
              hint="Uppercased and de-duplicated on save."
              note={clash('countries')}
            />
          )}
        </form.AppField>
      </DetailPageSection>

      <DetailPageSection title="Display & state">
        <div className="grid gap-4">
          <form.AppField name="sortOrder">
            {(f) => <f.NumberField id="sort" label="Sort order" className="sm:max-w-xs" note={clash('sortOrder')} />}
          </form.AppField>
          <div className="rounded-md border p-3">
            <form.AppField name="isActive">
              {(f) => <f.CheckboxField id="active" label="Active - inactive zones aren't matched at checkout." />}
            </form.AppField>
          </div>
        </div>
      </DetailPageSection>

      {zone && <MethodsSection zoneId={zone.id} methods={methods} onChanged={onRefetch} />}
    </RecordFormPage>
  )
}

function MethodsSection({ zoneId, methods, onChanged }: { zoneId: string; methods: ExistingMethod[]; onChanged?: () => void }) {
  const [creating, setCreating] = useState(false)
  return (
    <DetailPageSection
      title="Methods"
      description="Each method is one option at checkout. Free-above zeroes shipping once the cart subtotal crosses the threshold."
      trailing={
        <Button type="button" size="sm" variant="outline" onClick={() => setCreating(true)} disabled={creating}>
          <PlusIcon className="size-4" /> Add method
        </Button>
      }
    >
      {methods.length === 0 && !creating && (
        <p className="text-sm text-muted-foreground">No methods yet. Click "Add method" to create one.</p>
      )}
      <div className="grid gap-3">
        {methods.map((m) => (
          <MethodRow key={m.id} method={m} onChanged={onChanged} />
        ))}
        {creating && (
          <MethodRow
            zoneId={zoneId}
            onCancel={() => setCreating(false)}
            onChanged={() => {
              setCreating(false)
              onChanged?.()
            }}
          />
        )}
      </div>
    </DetailPageSection>
  )
}

// One method, edited and saved on its own - an inline CRUD row, the conventions' allowed exception
// to the single header Save
function MethodRow({
  method,
  zoneId,
  onChanged,
  onCancel,
}: {
  method?: ExistingMethod
  zoneId?: string
  onChanged?: () => void
  onCancel?: () => void
}) {
  const key = method?.id ?? 'new'
  const [error, setError] = useState<string | null>(null)
  const [createMethod] = useMutation(CreateShippingMethodDocument)
  const [updateMethod] = useMutation(UpdateShippingMethodDocument)
  const [deleteMethod] = useMutation(DeleteShippingMethodDocument)

  const baseline = useMemo(() => toMethodValues(method), [method])
  const form = useAppForm({
    defaultValues: baseline,
    validators: { onChange: methodSchema },
    onSubmit: async ({ value }) => {
      const input = toMethodInput(value)
      if (method) await updateMethod({ variables: { input: { id: method.id, ...input } } })
      else if (zoneId) await createMethod({ variables: { input: { zoneId, ...input } } })
      form.reset(value)
      onChanged?.()
    },
  })
  const values = useStore(form.store, (s) => s.values)
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  useUnsavedChangesGuard(!sameValue(values, baseline), `Shipping method · ${method?.code || 'new'}`)

  async function save() {
    setError(null)
    try {
      await submitForm(form)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function remove() {
    if (!method) return
    if (!window.confirm(`Delete method ${method.code}? This is permanent.`)) return
    try {
      await deleteMethod({ variables: { id: method.id } })
      onChanged?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div className="grid items-start gap-3 sm:grid-cols-[1fr_1fr_auto]">
        <form.AppField name="code">
          {(f) => <f.TextField id={`code-${key}`} label="Code" placeholder="standard" />}
        </form.AppField>
        <form.AppField name="name">
          {(f) => <f.TextField id={`name-${key}`} label="Display name" placeholder="Standard shipping" />}
        </form.AppField>
        <form.Field name="calcType">
          {(f) => (
            <div className="grid gap-1.5">
              <Label htmlFor={`type-${key}`}>Type</Label>
              <Select value={f.state.value} onValueChange={(v) => f.handleChange(v as CalcType)}>
                <SelectTrigger id={`type-${key}`} className="min-w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="FLAT">Flat</SelectItem>
                  <SelectItem value="FREE">Free</SelectItem>
                  <SelectItem value="FREE_ABOVE">Free above</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}
        </form.Field>
      </div>

      <div className="grid items-start gap-3 sm:grid-cols-3">
        {values.calcType !== 'FREE' && (
          <form.AppField name="flatDollars">
            {(f) => (
              <f.TextField
                id={`flat-${key}`}
                label="Flat price ($)"
                type="number"
                inputMode="decimal"
                step="0.01"
                min="0"
                placeholder="5.00"
              />
            )}
          </form.AppField>
        )}
        {values.calcType === 'FREE_ABOVE' && (
          <form.AppField name="freeAboveDollars">
            {(f) => (
              <f.TextField
                id={`above-${key}`}
                label="Free above ($)"
                type="number"
                inputMode="decimal"
                step="0.01"
                min="0"
                placeholder="50.00"
              />
            )}
          </form.AppField>
        )}
        <form.AppField name="sortOrder">{(f) => <f.NumberField id={`sort-${key}`} label="Sort order" />}</form.AppField>
      </div>

      <div className="grid items-start gap-3 border-t pt-3 sm:grid-cols-[200px_1fr]">
        <form.Field name="countryMode">
          {(f) => (
            <div className="grid gap-1.5">
              <Label htmlFor={`ships-${key}`}>Ships to</Label>
              <Select value={f.state.value} onValueChange={(v) => f.handleChange(v as 'all' | 'specific')}>
                <SelectTrigger id={`ships-${key}`} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All countries in zone</SelectItem>
                  <SelectItem value="specific">Specific countries</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}
        </form.Field>
        {values.countryMode === 'specific' && (
          <form.AppField name="allowedCountries">
            {(f) => (
              <f.TextField id={`countries-${key}`} label="Allowed countries (ISO-2, comma-separated)" placeholder="US, CA" />
            )}
          </form.AppField>
        )}
      </div>

      <div className="flex items-center justify-between gap-2">
        <form.AppField name="isActive">{(f) => <f.CheckboxField id={`active-${key}`} label="Active" />}</form.AppField>
        <div className="flex items-center gap-2">
          {!method && (
            <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
              Cancel
            </Button>
          )}
          {method && (
            <Button type="button" variant="ghost" size="icon" onClick={remove} title="Delete method" aria-label="Delete method">
              <Trash2Icon className="size-4 text-destructive" />
            </Button>
          )}
          <Button type="button" size="sm" onClick={() => void save()} disabled={submitting}>
            {submitting ? 'Saving…' : method ? 'Save' : 'Create'}
          </Button>
        </div>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}
