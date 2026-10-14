import { StoreIcon, WarehouseIcon, BoxesIcon, TruckIcon, PackageCheckIcon, CircleHelpIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { LOCATION_TYPE_LABELS, normalizeLocationType } from './location-types'
const icons = { RETAIL_STORE: StoreIcon, WAREHOUSE: WarehouseIcon, STORAGE: BoxesIcon, DROPSHIPPER: TruckIcon, FULFILLMENT_PARTNER: PackageCheckIcon, UNCLASSIFIED: CircleHelpIcon }
export function LocationTypeBadge({ type }: { type: string }) {
  const value = normalizeLocationType(type)
  const Icon = icons[value]
  return <Badge variant="outline" className={value === 'UNCLASSIFIED' ? 'gap-1 border-warning/40 text-warning' : 'gap-1'}><Icon className="size-3" aria-hidden="true" />{LOCATION_TYPE_LABELS[value]}</Badge>
}
