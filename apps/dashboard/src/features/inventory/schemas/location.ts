import { z } from 'zod'
import { LOCATION_TYPES, LOCATION_TYPE_LABELS, normalizeLocationType } from '../location-types'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import {
  cleanMetadata,
  metadataDiff,
  metadataSchema,
  toMetadataValues,
  type MetadataEntry,
} from '@/shared/schemas/metadata'
import { numberText, optionalEmail, parseNumberText } from '@/shared/schemas/fields'

// Inventory location form - THE field list. Values, validation, the form's type, the mutation
// input and the confirm-dialog diff all come from here, so a new column is added once
// (see docs/conventions.md, "TanStack Form pages")

export const DAYS = [
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

// What the location query returns, as far as the form cares
export type ExistingLocation = {
  id: string
  slug: string
  name: string
  storefrontVisible?: boolean
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

// Coordinates stay strings in the form - that's what the inputs hold, and '' is how "no coordinate"
// is typed - they become numbers only on the way into the mutation
const coordinate = (limit: number) => numberText({ optional: true, min: -limit, max: limit })

const dayHoursSchema = z
  .object({
    closed: z.boolean().describe('Closed all day'),
    open: z.string().describe('Opening time, as "HH:mm" in the location\'s timezone'),
    close: z.string().describe('Closing time, as "HH:mm" in the location\'s timezone'),
  })
  .describe("One day's opening hours")

export const locationSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required').describe('Stable identifier of the location'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the location'),
    storefrontVisible: z.boolean().describe('Show this location to storefront customers'),
    isActive: z.boolean().describe('Whether the location is in use'),
    type: z.enum(LOCATION_TYPES).describe('Purpose of the inventory location'),
    line1: z.string().describe('Street address - empty string for none'),
    line2: z.string().describe('Apartment, suite or floor - empty string for none'),
    city: z.string().describe('City - empty string for none'),
    region: z.string().describe('State, province or region - empty string for none'),
    postalCode: z.string().describe('Postal or ZIP code - empty string for none'),
    countryCode: z.string().describe('Upper-case ISO 3166-1 alpha-2 country code, such as US - empty string for none'),
    latitude: coordinate(90).describe('Latitude in decimal degrees, -90 to 90, written as text - empty string for none'),
    longitude: coordinate(180).describe('Longitude in decimal degrees, -180 to 180, written as text - empty string for none'),
    timezone: z.string().describe('IANA timezone the opening hours are in, such as Europe/Berlin'),
    hours: z
      .object({
        monday: dayHoursSchema,
        tuesday: dayHoursSchema,
        wednesday: dayHoursSchema,
        thursday: dayHoursSchema,
        friday: dayHoursSchema,
        saturday: dayHoursSchema,
        sunday: dayHoursSchema,
      })
      .describe('Weekly opening hours'),
    email: optionalEmail().describe('Contact email of the location, or an empty string for none'),
    phone: z.string().describe('Contact phone number - empty string for none'),
    contactName: z.string().describe('Person to contact at the location - empty string for none'),
    externalReference: z.string().describe("The location's ID in an outside system, such as an ERP - empty string for none"),
    pickupEnabled: z.boolean().describe('Customers can collect orders here'),
    pickupInstructions: z.string().describe('Instructions shown to customers collecting here - empty string for none'),
    metadata: metadataSchema,
  })
  .describe('An inventory location: a place that holds stock')

export type LocationValues = z.infer<typeof locationSchema>

// What the page calls each field - the diff labels and the restored-draft clash list both read it
export const LOCATION_LABELS: Record<keyof LocationValues, string> = {
  slug: 'Slug',
  name: 'Name',
  storefrontVisible: 'List in store finder',
  isActive: 'Active',
  type: 'Type',
  line1: 'Address line 1',
  line2: 'Address line 2',
  city: 'City',
  region: 'Region',
  postalCode: 'Postal code',
  countryCode: 'Country',
  latitude: 'Latitude',
  longitude: 'Longitude',
  timezone: 'Timezone',
  hours: 'Hours',
  email: 'Email',
  phone: 'Phone',
  contactName: 'Contact name',
  externalReference: 'External reference',
  pickupEnabled: 'Accept online orders for pickup',
  pickupInstructions: 'Pickup instructions',
  metadata: 'Metadata',
}

function emptyHours(): HoursMap {
  const out = {} as HoursMap
  for (const [key] of DAYS) out[key] = { closed: true, open: '09:00', close: '17:00' }
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


export function toLocationValues(location?: ExistingLocation): LocationValues {
  return {
    slug: location?.slug ?? '',
    name: location?.name ?? '',
    storefrontVisible: location?.storefrontVisible ?? false,
    isActive: location?.isActive ?? true,
    type: normalizeLocationType(location?.type),
    line1: location?.line1 ?? '',
    line2: location?.line2 ?? '',
    city: location?.city ?? '',
    region: location?.region ?? '',
    postalCode: location?.postalCode ?? '',
    countryCode: location?.countryCode ?? '',
    latitude: location?.latitude != null ? String(location.latitude) : '',
    longitude: location?.longitude != null ? String(location.longitude) : '',
    timezone: location?.timezone ?? '',
    hours: hoursFromJSON(location?.hours),
    email: location?.email ?? '',
    phone: location?.phone ?? '',
    contactName: location?.contactName ?? '',
    externalReference: location?.externalReference ?? '',
    pickupEnabled: location?.pickupEnabled ?? false,
    pickupInstructions: location?.pickupInstructions ?? '',
    metadata: toMetadataValues(location?.metadata),
  }
}

// Shared by create and update - slug, id and clearCoords are the only per-mutation extras
export function effectiveLocationValues(v: LocationValues): LocationValues {
  return v.type === 'RETAIL_STORE' ? v : { ...v, storefrontVisible: false, pickupEnabled: false }
}

export function toLocationInput(values: LocationValues) {
  const v = effectiveLocationValues(values)
  return {
    name: v.name,
    storefrontVisible: v.storefrontVisible,
    isActive: v.isActive,
    pickupEnabled: v.pickupEnabled,
    pickupInstructions: v.pickupInstructions,
    type: v.type,
    line1: v.line1,
    line2: v.line2,
    city: v.city,
    region: v.region,
    postalCode: v.postalCode,
    countryCode: v.countryCode,
    latitude: parseNumberText(v.latitude),
    longitude: parseNumberText(v.longitude),
    timezone: v.timezone,
    hours: hoursToJSON(v.hours),
    email: v.email,
    phone: v.phone,
    contactName: v.contactName,
    externalReference: v.externalReference,
    metadata: cleanMetadata(v.metadata),
  }
}

// Plain fields diff one-to-one; hours are one entry per day and metadata one per key, so the
// confirm dialog reads "Hours · Mon: 09:00–17:00 → closed" rather than a JSON blob. Slug is
// immutable after create, so it never appears
const SCALAR_FIELDS = (Object.keys(LOCATION_LABELS) as (keyof LocationValues)[]).filter(
  (k) => k !== 'slug' && k !== 'hours' && k !== 'metadata',
)

function dayText(d: DayHours): string {
  return d.closed ? 'closed' : `${d.open}–${d.close}`
}


export function locationDiff(original: LocationValues, values: LocationValues): SectionDiffEntry[] {
  const draft = effectiveLocationValues(values)
  const out = fieldDiff(
    original,
    draft,
    SCALAR_FIELDS.map((k) => ({ label: LOCATION_LABELS[k], value: (o: LocationValues) => k === 'type' ? LOCATION_TYPE_LABELS[o.type] : o[k] })),
  )
  for (const [key, label] of DAYS) {
    const entry = changed(`Hours · ${label}`, dayText(original.hours[key]), dayText(draft.hours[key]))
    if (entry) out.push(entry)
  }
  return [...out, ...metadataDiff(original.metadata, draft.metadata)]
}
