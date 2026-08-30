import { useMemo, useState } from 'react'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { FlatModeDormancyBanner } from './FlatModeDormancyBanner'
import { Button } from '@/components/ui/button'
import { LoadError } from '@/shared/errors/LoadError'
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
import { slugify } from '@/lib/slug'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import {
  CreateInventoryLocationDocument,
  InventoryLocationDocument,
  InventoryLocationsDocument,
  UpdateInventoryLocationDocument,
  SetLocationPickupDocument,
} from './Inventory.gql'

// Common type values surfaced as datalist suggestions - The column is
// free-form so admins can type anything; these are just hints
const TYPE_SUGGESTIONS = ['warehouse', 'retail', 'drop-shipper', 'fulfillment-center']

const DAYS = [
  ['monday', 'Mon'],
  ['tuesday', 'Tue'],
  ['wednesday', 'Wed'],
  ['thursday', 'Thu'],
  ['friday', 'Fri'],
  ['saturday', 'Sat'],
  ['sunday', 'Sun'],
] as const

type DayKey = (typeof DAYS)[number][0]
type DayHours = { closed: boolean; open: string; close: string }
type HoursMap = Record<DayKey, DayHours>

function emptyHours(): HoursMap {
  const out = {} as HoursMap
  for (const [key] of DAYS) {
    out[key] = { closed: true, open: '09:00', close: '17:00' }
  }
  return out
}

function hoursFromJSON(raw: unknown): HoursMap {
  const out = emptyHours()
  if (!raw || typeof raw !== 'object') return out
  const obj = raw as Record<string, unknown>
  for (const [key] of DAYS) {
    const day = obj[key]
    if (!day || typeof day !== 'object') continue
    const d = day as Record<string, unknown>
    out[key] = {
      closed: Boolean(d.closed),
      open: typeof d.open === 'string' ? d.open : '09:00',
      close: typeof d.close === 'string' ? d.close : '17:00',
    }
  }
  return out
}

function hoursToJSON(h: HoursMap): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key] of DAYS) {
    const d = h[key]
    out[key] = d.closed ? { closed: true } : { closed: false, open: d.open, close: d.close }
  }
  return out
}

type MetadataEntry = { key: string; value: string }

type ExistingLocation = {
  id: string
  slug: string
  name: string
  isActive: boolean
  type: string
  line1: string
  line2: string
  city: string
  region: string
  postalCode: string
  countryCode: string
  latitude?: number | null
  longitude?: number | null
  timezone: string
  hours: unknown
  email: string
  phone: string
  contactName: string
  externalReference: string
  pickupEnabled: boolean
  pickupInstructions: string
  metadata: MetadataEntry[]
}

// Create + edit render full-width from InventoryLocationsList - They accept an optional locationId prop so the parent
// passes the URL param down; standalone usage falls back to useParams so the entry points still work
export function InventoryLocationCreatePage() {
  return <InventoryLocationFormPage mode="create" />
}

export function InventoryLocationEditPage({ locationId }: { locationId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = locationId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery(InventoryLocationDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })

  if (loading) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading location…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load location"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.inventoryLocation) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Location not found.</p>
  }

  return <InventoryLocationFormPage mode="edit" location={data.inventoryLocation} />
}

function InventoryLocationFormPage({
  mode,
  location,
}: {
  mode: 'create' | 'edit'
  location?: ExistingLocation
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const [slug, setSlug] = useState(location?.slug ?? '')
  const [slugTouched, setSlugTouched] = useState(false)
  const [name, setName] = useState(location?.name ?? '')
  const [isActive, setIsActive] = useState(location?.isActive ?? true)
  const [type, setType] = useState(location?.type ?? '')

  const [line1, setLine1] = useState(location?.line1 ?? '')
  const [line2, setLine2] = useState(location?.line2 ?? '')
  const [city, setCity] = useState(location?.city ?? '')
  const [region, setRegion] = useState(location?.region ?? '')
  const [postalCode, setPostalCode] = useState(location?.postalCode ?? '')
  const [countryCode, setCountryCode] = useState(location?.countryCode ?? '')

  const [latitude, setLatitude] = useState<string>(
    location?.latitude != null ? String(location.latitude) : '',
  )
  const [longitude, setLongitude] = useState<string>(
    location?.longitude != null ? String(location.longitude) : '',
  )

  const [timezone, setTimezone] = useState(location?.timezone ?? '')
  const [hours, setHours] = useState<HoursMap>(hoursFromJSON(location?.hours))

  const [email, setEmail] = useState(location?.email ?? '')
  const [phone, setPhone] = useState(location?.phone ?? '')
  const [contactName, setContactName] = useState(location?.contactName ?? '')
  const [externalReference, setExternalReference] = useState(location?.externalReference ?? '')
  const [metadata, setMetadata] = useState<MetadataEntry[]>(
    (location?.metadata ?? []).map((m) => ({ key: m.key, value: m.value })),
  )

  const [submitError, setSubmitError] = useState<string | null>(null)

  // Header Save opens the diff modal instead of writing straight through - the dashboard-wide
  // convention (docs/conventions.md). One section here because the whole location is one form,
  // but it goes through the same registry so the dialog renders the same review list
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const countriesQuery = useQuery(CountriesPickerDocument)
  const countries = countriesQuery.data?.countries ?? []

  // What the dialog lists. Only meaningful when editing - a brand new location has nothing to
  // compare against, so create keeps writing straight through
  const draft = {
    name, isActive, type, line1, line2, city, region, postalCode, countryCode,
    latitude, longitude, timezone, email, phone, contactName, externalReference,
  }
  const original = {
    name: location?.name ?? '',
    isActive: location?.isActive ?? true,
    type: location?.type ?? '',
    line1: location?.line1 ?? '',
    line2: location?.line2 ?? '',
    city: location?.city ?? '',
    region: location?.region ?? '',
    postalCode: location?.postalCode ?? '',
    countryCode: location?.countryCode ?? '',
    latitude: location?.latitude != null ? String(location.latitude) : '',
    longitude: location?.longitude != null ? String(location.longitude) : '',
    timezone: location?.timezone ?? '',
    email: location?.email ?? '',
    phone: location?.phone ?? '',
    contactName: location?.contactName ?? '',
    externalReference: location?.externalReference ?? '',
  }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const locationDiff = useMemo(
    () =>
      isEdit
        ? fieldDiff(original, draft, [
            { label: 'Name', value: (o) => o.name },
            { label: 'Active', value: (o) => o.isActive },
            { label: 'Type', value: (o) => o.type },
            { label: 'Address line 1', value: (o) => o.line1 },
            { label: 'Address line 2', value: (o) => o.line2 },
            { label: 'City', value: (o) => o.city },
            { label: 'Region', value: (o) => o.region },
            { label: 'Postal code', value: (o) => o.postalCode },
            { label: 'Country', value: (o) => o.countryCode },
            { label: 'Latitude', value: (o) => o.latitude },
            { label: 'Longitude', value: (o) => o.longitude },
            { label: 'Timezone', value: (o) => o.timezone },
            { label: 'Email', value: (o) => o.email },
            { label: 'Phone', value: (o) => o.phone },
            { label: 'Contact name', value: (o) => o.contactName },
            { label: 'External reference', value: (o) => o.externalReference },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isEdit, originalKey, draftKey],
  )

  function onNameChange(next: string) {
    setName(next)
    if (!isEdit && !slugTouched) setSlug(slugify(next))
  }

  const refetchQueries = isEdit && location
    ? [
        { query: InventoryLocationsDocument },
        { query: InventoryLocationDocument, variables: { id: location.id } },
      ]
    : [{ query: InventoryLocationsDocument }]

  const [createLocation, { loading: creating }] = useMutation(CreateInventoryLocationDocument, { refetchQueries })
  const [updateLocation, { loading: updating }] = useMutation(UpdateInventoryLocationDocument, { refetchQueries })
  const loading = creating || updating

  function parseFloatOrNull(v: string): number | null {
    if (v.trim() === '') return null
    const n = Number(v)
    return Number.isFinite(n) ? n : null
  }

  // Registered so the header Save can list the changes before applying them
  useSaveSection({
    id: 'inventory-location',
    title: 'Location',
    diff: locationDiff,
    save: () => applyChanges(),
  })

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
    const lat = parseFloatOrNull(latitude)
    const lng = parseFloatOrNull(longitude)
    if (latitude.trim() && lat == null) {
      setSubmitError('Latitude must be a number.')
      return
    }
    if (longitude.trim() && lng == null) {
      setSubmitError('Longitude must be a number.')
      return
    }
    const hoursJSON = hoursToJSON(hours)
    const metadataInput = metadata
      .filter((m) => m.key.trim() !== '')
      .map((m) => ({ key: m.key.trim(), value: m.value }))
    try {
      if (isEdit && location) {
        await updateLocation({
          variables: {
            input: {
              id: location.id,
              name,
              isActive,
              type,
              line1,
              line2,
              city,
              region,
              postalCode,
              countryCode,
              latitude: lat,
              longitude: lng,
              clearCoords: lat == null && lng == null,
              timezone,
              hours: hoursJSON,
              email,
              phone,
              contactName,
              externalReference,
              metadata: metadataInput,
            },
          },
        })
      } else {
        await createLocation({
          variables: {
            input: {
              slug,
              name,
              isActive,
              type,
              line1,
              line2,
              city,
              region,
              postalCode,
              countryCode,
              latitude: lat,
              longitude: lng,
              timezone,
              hours: hoursJSON,
              email,
              phone,
              contactName,
              externalReference,
              metadata: metadataInput,
            },
          },
        })
      }
      navigate('/inventory/locations')
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
        <DetailPageHeader
          backTo="/inventory/locations"
          backLabel="Back to inventory locations"
          title={isEdit ? location?.name ?? 'Location' : 'New inventory location'}
          meta={
            isEdit && location?.slug ? (
              <code className="font-mono">{location.slug}</code>
            ) : !isEdit ? (
              <span>Physical places where stock is held - warehouses, retail shops, drop-shippers.</span>
            ) : null
          }
          actions={
            <>
              <Button type="button" variant="outline" size="sm" asChild>
                <Link to="/inventory/locations">Cancel</Link>
              </Button>
              {/* Edit reviews the diff first; create has nothing to compare against */}
              <Button
                type="button"
                size="sm"
                disabled={loading || (isEdit && !isDirty)}
                onClick={() => (isEdit ? setConfirmingSave(true) : void applyChanges())}
              >
                {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create location'}
              </Button>
            </>
          }
        />

        <DetailPageBody>
        {/* empty:hidden - the banner renders nothing outside flat mode, and without this the
            wrapper still drew 2rem of blank space and a divide-y rule under the header */}
        <div className="px-6 py-4 empty:hidden">
          <FlatModeDormancyBanner />
        </div>

        {submitError && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Save failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection
          title="Basics"
          description="Identity, classifier, and active flag. Inactive locations stay assigned to channels but don't take new reservations."
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="loc-name">Name</Label>
              <Input
                id="loc-name"
                required
                placeholder="Main warehouse"
                value={name}
                onChange={(e) => onNameChange(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="loc-slug">Slug</Label>
              <Input
                id="loc-slug"
                required
                disabled={isEdit}
                placeholder="main-warehouse"
                value={slug}
                onChange={(e) => { setSlug(e.target.value); setSlugTouched(true) }}
              />
              <p className="text-xs text-muted-foreground">
                {isEdit ? 'Slug is immutable.' : 'Auto-filled from name - edit to override.'}
              </p>
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="loc-type">Type</Label>
            <Input
              id="loc-type"
              placeholder="warehouse"
              list="loc-type-suggestions"
              value={type}
              onChange={(e) => setType(e.target.value)}
            />
            <datalist id="loc-type-suggestions">
              {TYPE_SUGGESTIONS.map((s) => (
                <option key={s} value={s} />
              ))}
            </datalist>
            <p className="text-xs text-muted-foreground">
              Free-form classifier. Common values: warehouse, retail, drop-shipper,
              fulfillment-center.
            </p>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-md border p-4">
            <div className="grid gap-0.5">
              <Label htmlFor="loc-active" className="text-sm">Active</Label>
              <p className="text-xs text-muted-foreground">
                Inactive locations stay assigned to channels but don't take new reservations.
              </p>
            </div>
            <Switch id="loc-active" checked={isActive} onCheckedChange={setIsActive} />
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Address"
          description="Used in subsidiary documents and warehouse-side fulfillment exports. Empty fields are stored as empty strings, not nulls."
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input placeholder="Line 1" value={line1} onChange={(e) => setLine1(e.target.value)} />
            <Input placeholder="Line 2" value={line2} onChange={(e) => setLine2(e.target.value)} />
            <Input placeholder="City" value={city} onChange={(e) => setCity(e.target.value)} />
            <Input
              placeholder="Region / state / province"
              value={region}
              onChange={(e) => setRegion(e.target.value)}
            />
            <Input placeholder="Postal code" value={postalCode} onChange={(e) => setPostalCode(e.target.value)} />
            <div className="grid gap-1.5">
              <Select value={countryCode || '__none__'} onValueChange={(v) => setCountryCode(v === '__none__' ? '' : v)}>
                <SelectTrigger>
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
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Coordinates"
          description="Optional. ~10cm precision (DECIMAL(9,6)). Paste from Google Maps."
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="loc-lat">Latitude</Label>
              <Input
                id="loc-lat"
                type="number"
                step="0.000001"
                inputMode="decimal"
                placeholder="-90 to 90"
                value={latitude}
                onChange={(e) => setLatitude(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="loc-lng">Longitude</Label>
              <Input
                id="loc-lng"
                type="number"
                step="0.000001"
                inputMode="decimal"
                placeholder="-180 to 180"
                value={longitude}
                onChange={(e) => setLongitude(e.target.value)}
              />
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Hours of operation"
          description={`Per-day open/close times. Toggle "Closed" for days the location isn't open.`}
        >
          <div className="grid gap-2">
            {DAYS.map(([key, label]) => {
              const d = hours[key]
              return (
                <div key={key} className="grid grid-cols-[60px_1fr_auto_1fr] items-center gap-3">
                  <span className="text-sm font-medium">{label}</span>
                  <Input
                    type="time"
                    value={d.open}
                    disabled={d.closed}
                    onChange={(e) => setHours({ ...hours, [key]: { ...d, open: e.target.value } })}
                  />
                  <span className="text-xs text-muted-foreground">to</span>
                  <Input
                    type="time"
                    value={d.close}
                    disabled={d.closed}
                    onChange={(e) => setHours({ ...hours, [key]: { ...d, close: e.target.value } })}
                  />
                  <div className="col-span-4 flex items-center justify-end gap-2 pt-1">
                    <Label htmlFor={`closed-${key}`} className="text-xs text-muted-foreground">Closed</Label>
                    <Switch
                      id={`closed-${key}`}
                      checked={d.closed}
                      onCheckedChange={(v) => setHours({ ...hours, [key]: { ...d, closed: v } })}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Contact"
          description="Warehouse-side contact details. Used in fulfillment notes and admin alerts; not customer-facing."
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input placeholder="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
            <Input placeholder="Phone" value={phone} onChange={(e) => setPhone(e.target.value)} />
            <Input
              placeholder="Contact name"
              value={contactName}
              onChange={(e) => setContactName(e.target.value)}
              className="sm:col-span-2"
            />
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Sync"
          description="Timezone for hours-of-operation rendering, plus the external reference an ERP/WMS uses to identify this location."
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="loc-tz">Timezone</Label>
              <Input
                id="loc-tz"
                placeholder="Europe/Berlin"
                value={timezone}
                onChange={(e) => setTimezone(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">IANA time zone, e.g. Europe/Berlin or America/New_York.</p>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="loc-extref">External reference</Label>
              <Input
                id="loc-extref"
                placeholder="WMS-MAIN-01"
                value={externalReference}
                onChange={(e) => setExternalReference(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">ERP/WMS code for this location (indexed for sync lookups).</p>
            </div>
          </div>
        </DetailPageSection>

        {location && (
          <DetailPageSection
            title="Pickup point"
            description="When enabled (and the location is active), checkout offers this location as a free click-and-collect option."
          >
            <PickupToggle
              locationId={location.id}
              initialEnabled={location.pickupEnabled}
              initialInstructions={location.pickupInstructions}
            />
          </DetailPageSection>
        )}

        <DetailPageSection
          title="Free-form metadata"
          description="Anything that doesn't fit the structured fields above (carrier accounts, custom flags, …). Backed by a JSONB column."
        >
          <div className="grid gap-2">
            {metadata.length === 0 ? (
              <p className="text-sm text-muted-foreground">No metadata yet.</p>
            ) : (
              metadata.map((m, i) => (
                <div key={i} className="grid grid-cols-[1fr_2fr_auto] items-center gap-2">
                  <Input
                    placeholder="Key"
                    value={m.key}
                    onChange={(e) => {
                      const next = [...metadata]
                      next[i] = { ...m, key: e.target.value }
                      setMetadata(next)
                    }}
                  />
                  <Input
                    placeholder="Value"
                    value={m.value}
                    onChange={(e) => {
                      const next = [...metadata]
                      next[i] = { ...m, value: e.target.value }
                      setMetadata(next)
                    }}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    onClick={() => setMetadata(metadata.filter((_, idx) => idx !== i))}
                    aria-label="Remove"
                  >
                    <Trash2Icon className="size-4" />
                  </Button>
                </div>
              ))
            )}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="w-fit"
              onClick={() => setMetadata([...metadata, { key: '', value: '' }])}
            >
              <PlusIcon className="size-4" />
              Add row
            </Button>
          </div>
        </DetailPageSection>
        </DetailPageBody>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Inventory location · ${location?.name ?? ''}`}
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


function PickupToggle({
  locationId,
  initialEnabled,
  initialInstructions,
}: {
  locationId: string
  initialEnabled: boolean
  initialInstructions: string
}) {
  const [enabled, setEnabled] = useState(initialEnabled)
  const [instructions, setInstructions] = useState(initialInstructions)
  const [save, { loading }] = useMutation(SetLocationPickupDocument, {
    refetchQueries: [{ query: InventoryLocationsDocument }],
  })

  return (
    <div className="grid gap-3">
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="size-4 rounded border-border"
        />
        Offer pickup at this location
      </label>
      <div className="grid gap-1.5">
        <Label htmlFor="loc-pickup-instructions">Pickup instructions</Label>
        <Input
          id="loc-pickup-instructions"
          placeholder="Enter through the side door, bring your order number…"
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
          disabled={!enabled}
        />
      </div>
      <div>
        <Button
          type="button"
          variant="outline"
          disabled={loading}
          onClick={() =>
            void save({
              variables: { id: locationId, enabled, instructions },
            })
          }
        >
          {loading ? 'Saving…' : 'Save pickup settings'}
        </Button>
      </div>
    </div>
  )
}
