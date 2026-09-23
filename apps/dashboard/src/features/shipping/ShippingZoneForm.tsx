import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { gql } from '@apollo/client'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import { LoadError } from '@/shared/errors/LoadError'

import type { CalcType, ShippingMethod, ShippingZone } from './ShippingZonesList'

const ZONE_QUERY = gql`
  query ShippingZone($id: ID!) {
    shippingZone(id: $id) {
      id
      channelId
      code
      name
      sortOrder
      isActive
      countries
      methods {
        id
        zoneId
        code
        name
        calcType
        flatCents
        freeAboveCents
        sortOrder
        isActive
      }
    }
  }
`

const CREATE_ZONE = gql`
  mutation CreateShippingZone($input: CreateShippingZoneInput!) {
    createShippingZone(input: $input) {
      id
    }
  }
`

const UPDATE_ZONE = gql`
  mutation UpdateShippingZone($input: UpdateShippingZoneInput!) {
    updateShippingZone(input: $input) {
      id
    }
  }
`

const CREATE_METHOD = gql`
  mutation CreateShippingMethod($input: CreateShippingMethodInput!) {
    createShippingMethod(input: $input) {
      id
    }
  }
`

const UPDATE_METHOD = gql`
  mutation UpdateShippingMethod($input: UpdateShippingMethodInput!) {
    updateShippingMethod(input: $input) {
      id
    }
  }
`

const DELETE_METHOD = gql`
  mutation DeleteShippingMethod($id: ID!) {
    deleteShippingMethod(id: $id)
  }
`

export function ShippingZoneCreatePage() {
  const [params] = useSearchParams()
  const channelId = params.get('channelId') ?? ''
  return <ShippingZoneFormPage mode="create" defaultChannelId={channelId} />
}

export function ShippingZoneEditPage({ zoneId }: { zoneId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = zoneId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery<
    { shippingZone: ShippingZone | null },
    { id: string }
  >(ZONE_QUERY, { variables: { id }, skip: !id, fetchPolicy: 'cache-and-network' })

  if (loading && !data) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading zone…</p>
  }
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load shipping zone"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.shippingZone) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Zone not found.</p>
  }
  return (
    <ShippingZoneFormPage
      mode="edit"
      zone={data.shippingZone}
      onRefetch={() => void refetch()}
    />
  )
}

function ShippingZoneFormPage({
  mode,
  zone,
  defaultChannelId,
  onRefetch,
}: {
  mode: 'create' | 'edit'
  zone?: ShippingZone
  defaultChannelId?: string
  onRefetch?: () => void
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const [channelId] = useState(zone?.channelId ?? defaultChannelId ?? '')
  const draft = useDraft({
    code: zone?.code ?? '',
    name: zone?.name ?? '',
    sortOrder: zone?.sortOrder ?? 0,
    isActive: zone?.isActive ?? true,
    countriesText: (zone?.countries ?? []).join(', '),
  })
  const { code, name, sortOrder, isActive, countriesText } = draft.values
  const { setCode, setName, setSortOrder, setIsActive, setCountriesText } = draft.setters
  // Countries entered as a comma-separated text input - simpler than a
  // multi-select picker for now - Parsed on save; whitespace ignored
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [createZone, { loading: creating }] = useMutation(CREATE_ZONE)
  const [updateZone, { loading: updating }] = useMutation(UPDATE_ZONE)
  const loading = creating || updating

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    const countries = countriesText
      .split(',')
      .map((c) => c.trim().toUpperCase())
      .filter(Boolean)

    try {
      if (isEdit && zone) {
        await updateZone({
          variables: {
            input: {
              id: zone.id,
              code,
              name,
              sortOrder,
              isActive,
              countries,
            },
          },
        })
        onRefetch?.()
      } else {
        if (!channelId) {
          setSubmitError('channelId missing - open from the shipping list page.')
          return
        }
        const res = await createZone({
          variables: {
            input: {
              channelId,
              code,
              name,
              sortOrder,
              isActive,
              countries,
            },
          },
        })
        const newId = (res.data as { createShippingZone?: { id?: string } } | null)
          ?.createShippingZone?.id
        await duringCommit(() =>
          newId ? navigate(`/shipping/${newId}`) : navigate('/shipping'),
        )
        return
      }
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col">
      <div className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-lg font-semibold tracking-tight">
              {isEdit ? zone?.name || zone?.code || '(unnamed)' : 'New shipping zone'}
            </h1>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              A zone groups countries served by the same set of methods.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to="/shipping">Cancel</Link>
            </Button>
            <Button type="submit" size="sm" disabled={loading}>
              {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create zone'}
            </Button>
          </div>
        </div>
      </div>

      <div className="divide-y pb-16">
        <section className="grid gap-4 px-6 py-6">
          <div className="grid gap-1">
            <h3 className="text-lg font-semibold tracking-tight">
              <span className="mr-1.5 text-primary">#</span>
              Identity
            </h3>
            <p className="text-sm text-muted-foreground">
              Code is stable across renames - used by carrier-integration plugins as a
              lookup key.
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="code">Code</Label>
              <Input
                id="code"
                required
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="us-domestic"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="name">Display name</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="United States"
              />
            </div>
          </div>
        </section>

        <section className="grid gap-4 px-6 py-6">
          <div className="grid gap-1">
            <h3 className="text-lg font-semibold tracking-tight">
              <span className="mr-1.5 text-primary">#</span>
              Countries
            </h3>
            <p className="text-sm text-muted-foreground">
              ISO 3166-1 alpha-2 codes, comma-separated. The order resolver picks the
              first matching active zone (sort order tie-breaks).
            </p>
          </div>
          <div className="grid gap-2">
            <Input
              value={countriesText}
              onChange={(e) => setCountriesText(e.target.value)}
              placeholder="US, CA, MX"
            />
            <p className="text-xs text-muted-foreground">
              Uppercased and de-duplicated on save.
            </p>
          </div>
        </section>

        <section className="grid gap-4 px-6 py-6">
          <div className="grid gap-1">
            <h3 className="text-lg font-semibold tracking-tight">
              <span className="mr-1.5 text-primary">#</span>
              Display &amp; state
            </h3>
          </div>
          <div className="grid gap-4">
            <div className="grid gap-2 sm:max-w-xs">
              <Label htmlFor="sort">Sort order</Label>
              <Input
                id="sort"
                type="number"
                inputMode="numeric"
                value={sortOrder}
                onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
              />
            </div>
            <div className="flex items-center gap-2 rounded-md border p-3">
              <Checkbox
                id="active"
                checked={isActive}
                onCheckedChange={(v) => setIsActive(!!v)}
              />
              <Label htmlFor="active" className="cursor-pointer">
                Active - inactive zones aren't matched at checkout.
              </Label>
            </div>
          </div>
        </section>

        {submitError && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>{isEdit ? 'Update failed' : 'Create failed'}</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          </div>
        )}

        {isEdit && zone && (
          <MethodsSection zoneID={zone.id} methods={zone.methods} onRefetch={onRefetch} />
        )}
      </div>
    </form>
  )
}

function MethodsSection({
  zoneID,
  methods,
  onRefetch,
}: {
  zoneID: string
  methods: ShippingMethod[]
  onRefetch?: () => void
}) {
  const [creating, setCreating] = useState(false)
  return (
    <section className="grid gap-4 px-6 py-6">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <h3 className="text-lg font-semibold tracking-tight">
            <span className="mr-1.5 text-primary">#</span>
            Methods
          </h3>
          <p className="text-sm text-muted-foreground">
            Each method is one option at checkout. Free-above zeroes shipping once the
            cart subtotal crosses the threshold.
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => setCreating(true)}
          disabled={creating}
        >
          <PlusIcon className="size-4" /> Add method
        </Button>
      </div>
      {methods.length === 0 && !creating && (
        <p className="text-sm text-muted-foreground">
          No methods yet. Click "Add method" to create one.
        </p>
      )}
      <div className="grid gap-3">
        {methods.map((m) => (
          <MethodRow key={m.id} method={m} onChanged={onRefetch} />
        ))}
        {creating && (
          <MethodRow
            mode="create"
            zoneID={zoneID}
            onCancel={() => setCreating(false)}
            onChanged={() => {
              setCreating(false)
              onRefetch?.()
            }}
          />
        )}
      </div>
    </section>
  )
}

function MethodRow({
  method,
  mode = 'edit',
  zoneID,
  onChanged,
  onCancel,
}: {
  method?: ShippingMethod
  mode?: 'edit' | 'create'
  zoneID?: string
  onChanged?: () => void
  onCancel?: () => void
}) {
  const draft = useDraft({
    code: method?.code ?? '',
    name: method?.name ?? '',
    calcType: (method?.calcType ?? 'FLAT') as CalcType,
    flatDollars: method ? (method.flatCents / 100).toFixed(2) : '',
    freeAboveDollars: method ? (method.freeAboveCents / 100).toFixed(2) : '',
    sortOrder: method?.sortOrder ?? 0,
    isActive: method?.isActive ?? true,
    countryMode: (method?.countryMode === 'specific' ? 'specific' : 'all') as 'all' | 'specific',
    countriesText: (method?.allowedCountries ?? []).join(', '),
  })
  const {
    code, name, calcType, flatDollars, freeAboveDollars, sortOrder, isActive, countryMode,
    countriesText,
  } = draft.values
  const {
    setCode, setName, setCalcType, setFlatDollars, setFreeAboveDollars, setSortOrder, setIsActive,
    setCountryMode, setCountriesText,
  } = draft.setters
  const [error, setError] = useState<string | null>(null)

  const [createMethod, { loading: creating }] = useMutation(CREATE_METHOD)
  const [updateMethod, { loading: updating }] = useMutation(UPDATE_METHOD)
  const [deleteMethod] = useMutation<{ deleteShippingMethod: boolean }, { id: string }>(
    DELETE_METHOD,
  )
  const saving = creating || updating

  async function save() {
    setError(null)
    const shared = {
      code,
      name,
      calcType,
      flatCents: Math.round((Number(flatDollars) || 0) * 100),
      freeAboveCents: Math.round((Number(freeAboveDollars) || 0) * 100),
      sortOrder,
      isActive,
      countryMode,
      allowedCountries:
        countryMode === 'specific'
          ? countriesText
              .split(',')
              .map((c) => c.trim().toUpperCase())
              .filter(Boolean)
          : [],
    }
    try {
      if (mode === 'create' && zoneID) {
        await createMethod({ variables: { input: { zoneId: zoneID, ...shared } } })
      } else if (method) {
        await updateMethod({ variables: { input: { id: method.id, ...shared } } })
      }
      onChanged?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function del() {
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
    <div className="grid gap-3 rounded-md border bg-card p-3">
      <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto]">
        <div className="grid gap-1">
          <Label htmlFor={`code-${method?.id ?? 'new'}`} className="text-xs">
            Code
          </Label>
          <Input
            id={`code-${method?.id ?? 'new'}`}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="standard"
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor={`name-${method?.id ?? 'new'}`} className="text-xs">
            Display name
          </Label>
          <Input
            id={`name-${method?.id ?? 'new'}`}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Standard shipping"
          />
        </div>
        <div className="grid gap-1">
          <Label className="text-xs">Type</Label>
          <Select value={calcType} onValueChange={(v) => setCalcType(v as CalcType)}>
            <SelectTrigger className="min-w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="FLAT">Flat</SelectItem>
              <SelectItem value="FREE">Free</SelectItem>
              <SelectItem value="FREE_ABOVE">Free above</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        {calcType !== 'FREE' && (
          <div className="grid gap-1">
            <Label htmlFor={`flat-${method?.id ?? 'new'}`} className="text-xs">
              Flat price ($)
            </Label>
            <Input
              id={`flat-${method?.id ?? 'new'}`}
              type="number"
              inputMode="decimal"
              step="0.01"
              min="0"
              value={flatDollars}
              onChange={(e) => setFlatDollars(e.target.value)}
              placeholder="5.00"
            />
          </div>
        )}
        {calcType === 'FREE_ABOVE' && (
          <div className="grid gap-1">
            <Label htmlFor={`above-${method?.id ?? 'new'}`} className="text-xs">
              Free above ($)
            </Label>
            <Input
              id={`above-${method?.id ?? 'new'}`}
              type="number"
              inputMode="decimal"
              step="0.01"
              min="0"
              value={freeAboveDollars}
              onChange={(e) => setFreeAboveDollars(e.target.value)}
              placeholder="50.00"
            />
          </div>
        )}
        <div className="grid gap-1">
          <Label htmlFor={`sort-${method?.id ?? 'new'}`} className="text-xs">
            Sort order
          </Label>
          <Input
            id={`sort-${method?.id ?? 'new'}`}
            type="number"
            inputMode="numeric"
            value={sortOrder}
            onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
          />
        </div>
      </div>

      <div className="grid gap-3 border-t pt-3 sm:grid-cols-[200px_1fr]">
        <div className="grid gap-1">
          <Label className="text-xs">Ships to</Label>
          <Select value={countryMode} onValueChange={(v) => setCountryMode(v as 'all' | 'specific')}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All countries in zone</SelectItem>
              <SelectItem value="specific">Specific countries</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {countryMode === 'specific' && (
          <div className="grid gap-1">
            <Label htmlFor={`countries-${method?.id ?? 'new'}`} className="text-xs">
              Allowed countries (ISO-2, comma-separated)
            </Label>
            <Input
              id={`countries-${method?.id ?? 'new'}`}
              value={countriesText}
              onChange={(e) => setCountriesText(e.target.value)}
              placeholder="US, CA"
            />
          </div>
        )}
      </div>

      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Checkbox
            id={`active-${method?.id ?? 'new'}`}
            checked={isActive}
            onCheckedChange={(v) => setIsActive(!!v)}
          />
          <Label htmlFor={`active-${method?.id ?? 'new'}`} className="cursor-pointer text-xs">
            Active
          </Label>
        </div>
        <div className="flex items-center gap-2">
          {mode === 'create' && (
            <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
              Cancel
            </Button>
          )}
          {mode === 'edit' && method && (
            <Button type="button" variant="ghost" size="icon" onClick={del} title="Delete method">
              <Trash2Icon className="size-4 text-destructive" />
            </Button>
          )}
          <Button type="button" size="sm" onClick={save} disabled={saving}>
            {saving ? 'Saving…' : mode === 'create' ? 'Create' : 'Save'}
          </Button>
        </div>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}
