import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
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
import { LoadError } from '@/shared/errors/LoadError'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { LOCATION_TYPES, LOCATION_TYPE_LABELS, LOCATION_TYPE_DESCRIPTIONS } from './location-types'
import { LocationTypeBadge } from './LocationTypeBadge'
import { FlatModeDormancyBanner } from './FlatModeDormancyBanner'
import {
  CreateInventoryLocationDocument,
  InventoryLocationDocument,
  InventoryLocationsDocument,
  UpdateInventoryLocationDocument,
} from './Inventory.gql'
import {
  DAYS,
  LOCATION_LABELS,
  locationDiff,
  locationSchema,
  toLocationInput,
  toLocationValues,
  type ExistingLocation,
} from './schemas/location'

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

  // UI state, not the admin's work - stays out of the form (and out of the stored draft)
  const [slugTouched, setSlugTouched] = useState(false)

  const countries = useQuery(CountriesPickerDocument).data?.countries ?? []

  const refetchQueries = location
    ? [
        { query: InventoryLocationsDocument },
        { query: InventoryLocationDocument, variables: { id: location.id } },
      ]
    : [{ query: InventoryLocationsDocument }]
  const [createLocation] = useMutation(CreateInventoryLocationDocument, { refetchQueries })
  const [updateLocation] = useMutation(UpdateInventoryLocationDocument, { refetchQueries })

  // Keyed on the record - it's the form's defaultValues and what the diff and stored draft compare to
  const baseline = useMemo(() => toLocationValues(location), [location])

  const record = useRecordForm({
    id: 'inventory-location',
    title: 'Location',
    label: isEdit ? `Inventory location · ${location?.name ?? ''}` : 'New inventory location',
    isEdit,
    baseline,
    schema: locationSchema,
    diff: locationDiff,
    persist: { routeKey: `inventory-location:${location?.id ?? 'new'}` },
    submit: async (value) => {
      const input = toLocationInput(value)
      if (!location) {
        await createLocation({ variables: { input: { slug: value.slug, ...input } } })
        return
      }
      // Hours only when they moved: the form reads "none set" as closed every day, so writing an untouched
      // week back turned a location with no hours into one closed all week
      const hoursMoved = JSON.stringify(value.hours) !== JSON.stringify(baseline.hours)
      await updateLocation({
        variables: {
          input: {
            id: location.id,
            ...input,
            hours: hoursMoved ? input.hours : undefined,
            clearCoords: input.latitude == null && input.longitude == null,
          },
        },
      })

    },
    onSaved: () => navigate('/inventory/locations'),
  })
  const { form, draft } = record
  const clash = (field: string) => <ClashNote draft={draft} field={field} />

  return (
    <RecordFormPage
      record={record}
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
      createLabel="Create location"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/inventory/locations">Cancel</Link>
        </Button>
      }
      labels={LOCATION_LABELS}
    >
      {/* empty:hidden - the banner renders nothing outside flat mode, and without this the
          wrapper still drew 2rem of blank space and a divide-y rule under the header */}
      <div className="px-6 py-4 empty:hidden">
        <FlatModeDormancyBanner />
      </div>

      <DetailPageSection
        title="Basics"
        description="Identity, classifier, and active flag. Inactive locations stay assigned to channels but don't take new reservations."
      >
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField
            name="name"
            listeners={{
              // New locations derive the slug from the name until the admin edits it themselves
              onChange: ({ value }) => {
                if (!isEdit && !slugTouched) form.setFieldValue('slug', slugify(value))
              },
            }}
          >
            {(f) => <f.TextField id="loc-name" label="Name" placeholder="Main warehouse" note={clash('name')} />}
          </form.AppField>
          <form.AppField name="slug">
            {(f) => (
              <f.TextField
                id="loc-slug"
                label="Slug"
                disabled={isEdit}
                placeholder="main-warehouse"
                onValueChange={() => setSlugTouched(true)}
                hint={isEdit ? 'Slug is immutable.' : 'Auto-filled from name - edit to override.'}
              />
            )}
          </form.AppField>
        </div>

        <form.Subscribe selector={(s) => s.values.type}>
          {(type) => <div className="space-y-2">
            <form.AppField name="type">{(f) => <f.SelectField id="loc-type" label="Location type" options={LOCATION_TYPES.map(value => ({ value, label: LOCATION_TYPE_LABELS[value] }))} hint={LOCATION_TYPE_DESCRIPTIONS[type]} note={clash('type')} />}</form.AppField>
            <LocationTypeBadge type={type} />
            {baseline.type === 'RETAIL_STORE' && type !== 'RETAIL_STORE' && <p className="text-sm text-warning">Saving this type change removes the location from the store finder and disables online pickup. Review these changes with Save.</p>}
          </div>}
        </form.Subscribe>

        <form.AppField name="isActive">
          {(f) => (
            <f.SwitchField
              id="loc-active"
              label="Active"
              description="Inactive locations stay assigned to channels but don't take new reservations."
              note={clash('isActive')}
            />
          )}
        </form.AppField>
      </DetailPageSection>

      <DetailPageSection
        title="Address"
        description="Address for inventory operations. Public retail stores also show it to customers in the store finder."
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <form.AppField name="line1">
            {(f) => <f.TextField placeholder="Line 1" aria-label="Line 1" note={clash('line1')} />}
          </form.AppField>
          <form.AppField name="line2">
            {(f) => <f.TextField placeholder="Line 2" aria-label="Line 2" note={clash('line2')} />}
          </form.AppField>
          <form.AppField name="city">
            {(f) => <f.TextField placeholder="City" aria-label="City" note={clash('city')} />}
          </form.AppField>
          <form.AppField name="region">
            {(f) => <f.TextField placeholder="Region / state / province" aria-label="Region" note={clash('region')} />}
          </form.AppField>
          <form.AppField name="postalCode">
            {(f) => <f.TextField placeholder="Postal code" aria-label="Postal code" note={clash('postalCode')} />}
          </form.AppField>
          <form.Field name="countryCode">
            {(f) => (
              <div className="grid gap-1.5">
                <Select
                  value={f.state.value || '__none__'}
                  onValueChange={(v) => f.handleChange(v === '__none__' ? '' : v)}
                >
                  <SelectTrigger className="w-full" aria-label="Country">
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
                {clash('countryCode')}
              </div>
            )}
          </form.Field>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Coordinates"
        description="Optional. ~10cm precision (DECIMAL(9,6)). Paste from Google Maps."
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <form.AppField name="latitude">
            {(f) => (
              <f.TextField
                id="loc-lat"
                label="Latitude"
                type="number"
                step="0.000001"
                inputMode="decimal"
                placeholder="-90 to 90"
                note={clash('latitude')}
              />
            )}
          </form.AppField>
          <form.AppField name="longitude">
            {(f) => (
              <f.TextField
                id="loc-lng"
                label="Longitude"
                type="number"
                step="0.000001"
                inputMode="decimal"
                placeholder="-180 to 180"
                note={clash('longitude')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Hours of operation"
        description={`Per-day open/close times. Toggle "Closed" for days the location isn't open.`}
      >
        <div className="grid gap-2">
          {DAYS.map(([key, label]) => (
            <div key={key} className="grid grid-cols-[60px_1fr_auto_1fr] items-center gap-3">
              <span className="text-sm font-medium">{label}</span>
              <form.Subscribe selector={(s) => s.values.hours[key].closed}>
                {(closed) => (
                  <>
                    <form.Field name={`hours.${key}.open`}>
                      {(f) => (
                        <Input
                          type="time"
                          aria-label={`${label} opens`}
                          value={f.state.value}
                          disabled={closed}
                          onChange={(e) => f.handleChange(e.target.value)}
                        />
                      )}
                    </form.Field>
                    <span className="text-xs text-muted-foreground">to</span>
                    <form.Field name={`hours.${key}.close`}>
                      {(f) => (
                        <Input
                          type="time"
                          aria-label={`${label} closes`}
                          value={f.state.value}
                          disabled={closed}
                          onChange={(e) => f.handleChange(e.target.value)}
                        />
                      )}
                    </form.Field>
                  </>
                )}
              </form.Subscribe>
              <div className="col-span-4 flex items-center justify-end gap-2 pt-1">
                <Label htmlFor={`closed-${key}`} className="text-xs text-muted-foreground">Closed</Label>
                <form.Field name={`hours.${key}.closed`}>
                  {(f) => <Switch id={`closed-${key}`} checked={f.state.value} onCheckedChange={f.handleChange} />}
                </form.Field>
              </div>
            </div>
          ))}
          {clash('hours')}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Contact"
        description="Warehouse-side contact details. Used in fulfillment notes and admin alerts; not customer-facing."
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <form.AppField name="email">
            {(f) => <f.TextField placeholder="Email" aria-label="Email" type="email" note={clash('email')} />}
          </form.AppField>
          <form.AppField name="phone">
            {(f) => <f.TextField placeholder="Phone" aria-label="Phone" note={clash('phone')} />}
          </form.AppField>
          <form.AppField name="contactName">
            {(f) => (
              <f.TextField
                placeholder="Contact name"
                aria-label="Contact name"
                className="sm:col-span-2"
                note={clash('contactName')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Sync"
        description="Timezone for hours-of-operation rendering, plus the external reference an ERP/WMS uses to identify this location."
      >
        <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
          <form.AppField name="timezone">
            {(f) => (
              <f.TextField
                id="loc-tz"
                label="Timezone"
                placeholder="Europe/Berlin"
                hint="IANA time zone, e.g. Europe/Berlin or America/New_York."
                note={clash('timezone')}
              />
            )}
          </form.AppField>
          <form.AppField name="externalReference">
            {(f) => (
              <f.TextField
                id="loc-extref"
                label="External reference"
                placeholder="WMS-MAIN-01"
                hint="ERP/WMS code for this location (indexed for sync lookups)."
                note={clash('externalReference')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <form.Subscribe selector={(s) => ({ type: s.values.type, visible: s.values.storefrontVisible, pickup: s.values.pickupEnabled, active: s.values.isActive })}>
        {({ type, visible, pickup, active }) => type === 'RETAIL_STORE' ? (
          <DetailPageSection title="Customer access" description="A retail store can welcome walk-in shoppers without offering online pickup.">
            <div className="space-y-4">
              <form.AppField name="storefrontVisible">{(f) => <f.SwitchField id="loc-storefront-visible" label="List in store finder" description="Show this store and its stock on assigned channels when Store availability is enabled." note={clash('storefrontVisible')} />}</form.AppField>
              <form.AppField name="pickupEnabled">{(f) => <f.SwitchField id="loc-pickup-enabled" label="Accept online orders for pickup" description="Offer this active store as a free click-and-collect option at checkout." note={clash('pickupEnabled')} />}</form.AppField>
              <form.AppField name="pickupInstructions">{(f) => <f.TextField id="loc-pickup-instructions" label="Pickup instructions" placeholder="Bring your order number…" disabled={!pickup} note={clash('pickupInstructions')} />}</form.AppField>
              <p className="rounded-md border p-3 text-sm" role="status">
                {!active ? 'This store is inactive and unavailable to customers.' : `${visible ? 'Customers can check stock and visit this store on its assigned channels when Store availability is enabled.' : 'This store is hidden from the store finder.'} ${pickup ? 'Online pickup is enabled.' : 'Online pickup is disabled.'}`}
              </p>
            </div>
          </DetailPageSection>
        ) : <DetailPageSection title="Customer access"><p className="text-sm text-muted-foreground">This location is internal. Only retail stores can appear in the store finder or accept online pickup orders.</p></DetailPageSection>}
      </form.Subscribe>

      <DetailPageSection
        title="Free-form metadata"
        description="Anything that doesn't fit the structured fields above (carrier accounts, custom flags, …). Backed by a JSONB column."
      >
        <form.AppField name="metadata">{(f) => <f.MetadataField note={clash('metadata')} />}</form.AppField>
      </DetailPageSection>
    </RecordFormPage>
  )
}
