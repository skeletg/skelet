import type { RemoteProviderKind } from '@/graphql/schema.types'

// Human labels + one-line contract hints per remote provider kind - Kept in their own module so the component files
// stay fast-refresh-clean (react-refresh/only-export-components)
export const KIND_LABELS: Record<RemoteProviderKind, string> = {
  SHIPPING_CARRIER: 'Shipping carrier',
  TAX_PROVIDER: 'Tax provider',
  ADDRESS_VALIDATOR: 'Address validator',
  PROMOTION_CONDITION: 'Promotion condition',
  PROMOTION_ACTION: 'Promotion action',
  FRAUD_RISK: 'Fraud / risk',
  CURRENCY_RATE: 'Currency rates',
  PRICING: 'Pricing',
  RECOMMENDATION: 'Recommendations',
  REVIEWS: 'Reviews',
  SEARCH: 'Search',
  CUSTOMER_SEGMENTS: 'Customer segments',
  RETURNS_POLICY: 'Returns policy',
  INVENTORY_AVAILABILITY: 'Inventory availability',
  SHIPPING_TRACKING: 'Shipping tracking',
  DELIVERY_POINTS: 'Delivery points',
}

export const KIND_HINTS: Record<RemoteProviderKind, string> = {
  DELIVERY_POINTS: 'Search and resolve PO boxes and lockers using the shared point contract.',
  SHIPPING_CARRIER: 'Called during rate quoting - respond with {"rates": [...]}.',
  TAX_PROVIDER: 'Called at order placement - respond with {"taxCents": ...}.',
  ADDRESS_VALIDATOR: 'Called at checkout - respond with {"valid": bool, "reason": ...}.',
  PROMOTION_CONDITION: 'Called during cart evaluation - respond with {"match": bool}. Set a cache TTL.',
  PROMOTION_ACTION: 'Called during cart evaluation - respond with discount amounts. Set a cache TTL.',
  FRAUD_RISK: 'Called at the pre-transaction checkout gate - respond with {"decision": "approve|review|reject", "score": 0-100}.',
  CURRENCY_RATE: 'Called by the currency-rates refresh job - respond with {"base": "USD", "rates": {"EUR": 0.92, ...}}.',
  PRICING: 'Called during cart pricing - respond with {"unitPriceCents": ...}. Set a cache TTL; always fails open to catalog price.',
  RECOMMENDATION: 'Called for "also bought" / cart rails - respond with {"productIds": [...]}. Falls back to the native co-occurrence list.',
  REVIEWS: 'Called for product rating aggregates - respond with {"aggregates": {"<productId>": {"count","average","distribution"}}}.',
  SEARCH: 'Called for storefront product search + browse - respond with hits/facets. Falls back to OpenSearch.',
  CUSTOMER_SEGMENTS: 'Called during promotion/flow evaluation - respond with {"segments": ["slug", ...]}. Unioned with native segments.',
  RETURNS_POLICY: 'Called when a return is requested - respond with {"allowed": bool, "reason": ...}. Falls back to allow.',
  INVENTORY_AVAILABILITY: 'Called for salable stock (external WMS/ERP) - respond with {"salable": {"<productId>": qty}}. Set a cache TTL; falls back to native.',
  SHIPPING_TRACKING: 'Called to track a shipment - respond with {"status","url","events":[...]}. Falls back to the carrier\'s own tracking.',
}
