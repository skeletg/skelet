import { StoreIcon, WarehouseIcon, BoxesIcon, TruckIcon, PackageCheckIcon, CircleHelpIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { LOCATION_TYPE_LABELS, type LocationType } from './location-types'
const icons: Record<LocationType, typeof StoreIcon> = { RETAIL_STORE: StoreIcon, WAREHOUSE: WarehouseIcon, STORAGE: BoxesIcon, DROPSHIPPER: TruckIcon, FULFILLMENT_PARTNER: PackageCheckIcon, UNCLASSIFIED: CircleHelpIcon }
export function LocationTypeBadge({ type }: { type: LocationType }) {
  const Icon = icons[type]
  return <Badge variant="outline" className={type === 'UNCLASSIFIED' ? 'gap-1 border-warning/40 text-warning' : 'gap-1'}><Icon className="size-3" aria-hidden="true" />{LOCATION_TYPE_LABELS[type]}</Badge>
}
