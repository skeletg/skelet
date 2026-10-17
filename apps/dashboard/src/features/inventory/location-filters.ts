import type { InventoryLocationFilter } from '@/graphql/schema.types'
import { LOCATION_TYPES, type LocationType } from './location-types'

export function locationFiltersForQuery(filters: { id: string; value: unknown }[]): InventoryLocationFilter {
  const get = (id: string) => filters.find(filter => filter.id === id)?.value
  const rawTypes = get('type')
  const types = Array.isArray(rawTypes) ? rawTypes.filter((value): value is LocationType => LOCATION_TYPES.includes(value as LocationType)) : []
  const boolean = (id: string) => typeof get(id) === 'boolean' ? get(id) as boolean : undefined
  return { types, city: typeof get('city') === 'string' ? (get('city') as string).trim() : undefined, isActive: boolean('isActive'), storefrontVisible: boolean('storefrontVisible'), pickupEnabled: boolean('pickupEnabled') }
}
