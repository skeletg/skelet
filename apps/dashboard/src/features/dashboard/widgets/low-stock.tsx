import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { LowStockLevelsDocument } from '@/features/inventory/Inventory.gql'
import { WidgetSection } from './primitives'

const ROWS = 6

export function LowStockWidget() {
  const { data, loading, error } = useQuery(LowStockLevelsDocument, {
    variables: { limit: ROWS },
    fetchPolicy: 'cache-and-network',
  })
  const rows = data?.lowStockLevels ?? []

  return (
    <WidgetSection
      title="Low stock"
      description="At or under their threshold, so they need restocking before they stop selling."
      to="/inventory/low-stock"
      loading={loading && rows.length === 0}
      error={error ?? null}
      empty={!loading && rows.length === 0}
    >
      <div className="overflow-x-auto rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
              <th className="px-3 py-2">Product</th>
              <th className="px-3 py-2">Location</th>
              <th className="px-3 py-2 text-right">On hand</th>
              <th className="px-3 py-2 text-right">Threshold</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={`${r.inventoryItemId}-${r.locationId}`} className="border-b last:border-0 hover:bg-muted/40">
                <td className="px-3 py-2">
                  <Link to={`/products/${r.productId}`} className="font-medium hover:underline">
                    {r.productSlug}
                  </Link>
                  <div className="font-mono text-[10px] text-muted-foreground">{r.productSku}</div>
                </td>
                <td className="px-3 py-2 text-muted-foreground">{r.locationName}</td>
                <td className="px-3 py-2 text-right font-medium tabular-nums">
                  {r.quantity}
                  {r.backorderable && (
                    <span className="ml-1 text-xs font-normal text-muted-foreground">backorder</span>
                  )}
                </td>
                <td className="px-3 py-2 text-right tabular-nums text-muted-foreground">
                  {r.lowStockThreshold}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </WidgetSection>
  )
}
