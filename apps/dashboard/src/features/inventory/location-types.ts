export const LOCATION_TYPES = ['RETAIL_STORE', 'WAREHOUSE', 'STORAGE', 'DROPSHIPPER', 'FULFILLMENT_PARTNER', 'UNCLASSIFIED'] as const
export type LocationType = (typeof LOCATION_TYPES)[number]
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
export function normalizeLocationType(value?: string): LocationType {
  const normalized = value?.trim().toUpperCase().replaceAll('-', '_').replaceAll(' ', '_')
  if (normalized === 'RETAIL' || normalized === 'STORE' || normalized === 'SHOP') return 'RETAIL_STORE'
  if (normalized === 'DROP_SHIPPER') return 'DROPSHIPPER'
  if (normalized === 'FULFILLMENT_CENTER') return 'FULFILLMENT_PARTNER'
  return LOCATION_TYPES.includes(normalized as LocationType) ? normalized as LocationType : 'UNCLASSIFIED'
}
