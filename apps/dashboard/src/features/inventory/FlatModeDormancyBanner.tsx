import { useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { InventoryModeDocument } from './Inventory.gql'

// Flat inventory mode leaves this data editable so flipping back keeps it usable, but it has no effect on salability or placement until then
export function FlatModeDormancyBanner() {
  const { data } = useQuery(InventoryModeDocument, { fetchPolicy: 'cache-first' })
  if (data?.inventoryMode !== 'FLAT') return null
  return (
    <Alert>
      <AlertTitle>Flat inventory mode is on</AlertTitle>
      <AlertDescription>
        Public stores can still appear in the store directory, but per-store stock checking is unavailable.
        Locations and channel bindings are saved but dormant - storefront salability and order
        placement read the single quantity column on each product. Switch back to multi-location
        mode under Settings → Inventory to re-activate this surface.
      </AlertDescription>
    </Alert>
  )
}
