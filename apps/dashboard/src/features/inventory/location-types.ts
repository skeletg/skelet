import type { InventoryLocationType } from '@/graphql/schema.types'

export type LocationType = InventoryLocationType
export const LOCATION_TYPES = ['RETAIL_STORE', 'WAREHOUSE', 'STORAGE', 'DROPSHIPPER', 'FULFILLMENT_PARTNER', 'UNCLASSIFIED'] as const satisfies readonly LocationType[]
export const LOCATION_TYPE_LABELS: Record<LocationType, string> = {
  RETAIL_STORE: 'Retail store', WAREHOUSE: 'Warehouse', STORAGE: 'Storage',
  DROPSHIPPER: 'Dropshipper', FULFILLMENT_PARTNER: 'Fulfillment partner', UNCLASSIFIED: 'Needs classification',
}
export const LOCATION_TYPE_DESCRIPTIONS: Record<LocationType, string> = {
  RETAIL_STORE: 'Customers can visit this store. Public listing and online pickup are configured separately.',
  WAREHOUSE: 'Stockholding and order fulfillment. Customers cannot find or collect from this location.',
  STORAGE: 'Internal or reserve stock. This location stays private.',
  DROPSHIPPER: 'A supplier holds and ships inventory on your behalf. This location stays private.',
  FULFILLMENT_PARTNER: 'An external logistics provider stores and fulfills orders. This location stays private.',
  UNCLASSIFIED: 'Choose the purpose of this location. Unclassified locations stay private.',
}
