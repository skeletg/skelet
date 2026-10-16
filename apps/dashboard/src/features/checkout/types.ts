import type { CheckoutSettingsQuery } from './Checkout.gql'

// The server's view of a channel's checkout - typed by codegen, except the JSON scalars narrowed below
export type Configuration = CheckoutSettingsQuery['checkoutConfiguration']
export type Kind = Configuration['kinds'][number]
export type ServerOption = Configuration['options'][number]
export type Source = Configuration['pointSources'][number]
export type Readiness = ServerOption['readiness'][number]

export type Mode = 'required' | 'optional' | 'hidden'

// A pickup point (locker, parcel shop, PO box) the merchant lists - id is empty until the page-header Save creates it
export type Point = {
  key: string
  id: string
  name: string
  street1: string
  street2: string
  city: string
  region: string
  postalCode: string
  countryCode: string
  instructions: string
  hours: string
  isActive: boolean
}

// The editable part of a delivery option - what DeliveryOptionInput takes, plus its id
export type Option = {
  id: string
  kind: string
  name: string
  description: string
  enabled: boolean
  sortOrder: number
  blockModes: Record<string, Mode>
  pointSource: string
  carrierSlug: string
}

export function toMode(value: unknown): Mode | undefined {
  const mode = typeof value === 'string' ? value.toLowerCase() : ''
  return mode === 'required' || mode === 'optional' || mode === 'hidden' ? mode : undefined
}

export function toModes(value: unknown): Record<string, Mode> {
  const out: Record<string, Mode> = {}
  if (!value || typeof value !== 'object') return out
  for (const [key, raw] of Object.entries(value)) {
    const mode = toMode(raw)
    if (mode) out[key] = mode
  }
  return out
}

export function toOption(o: ServerOption): Option {
  return {
    id: o.id,
    kind: o.kind,
    name: o.name,
    description: o.description,
    enabled: o.enabled,
    sortOrder: o.sortOrder,
    blockModes: toModes(o.blockModes),
    pointSource: o.pointSource,
    carrierSlug: o.carrierSlug,
  }
}

export function toPoints(points: Configuration['deliveryPoints']): Point[] {
  return points.map(p => ({ ...p, key: p.id }))
}

// The mode an option asks a block in - locked blocks are always asked, NA ones never
export function blockMode(block: Kind['blocks'][number], modes: Record<string, Mode>): Mode {
  if (block.floor === 'LOCKED') return 'required'
  if (block.floor === 'NA') return 'hidden'
  return modes[block.type] ?? toMode(block.default) ?? 'optional'
}
