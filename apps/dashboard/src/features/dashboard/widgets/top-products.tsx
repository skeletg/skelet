import { formatCents } from '@/lib/format'
import { SALES_DAYS, TOP_PRODUCTS_LIMIT, useSalesReport } from './sales'
import { WidgetSection } from './primitives'

// Shares the revenue section's query - same document, same variables, one request
export function TopProductsWidget() {
  const { loading, error, currency, topProducts } = useSalesReport()

  return (
    <WidgetSection
      title="Top products"
      description={`Best sellers by revenue over the last ${SALES_DAYS} days.`}
      loading={loading && topProducts.length === 0}
      error={error}
      empty={!loading && topProducts.length === 0}
    >
      <div className="overflow-x-auto rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
              <th className="px-3 py-2">Product</th>
              <th className="px-3 py-2 text-right">Units</th>
              <th className="px-3 py-2 text-right">Revenue</th>
              <th className="px-3 py-2 text-right">Margin</th>
            </tr>
          </thead>
          <tbody>
            {topProducts.slice(0, TOP_PRODUCTS_LIMIT).map((p) => (
              <tr key={p.sku} className="border-b last:border-0 hover:bg-muted/40">
                <td className="px-3 py-2">
                  <div className="font-medium">{p.name}</div>
                  <div className="font-mono text-[10px] text-muted-foreground">{p.sku}</div>
                </td>
                <td className="px-3 py-2 text-right tabular-nums">{Number(p.unitsSold).toLocaleString()}</td>
                <td className="px-3 py-2 text-right tabular-nums">{formatCents(p.revenue, currency)}</td>
                <td className="px-3 py-2 text-right tabular-nums text-muted-foreground">
                  {formatCents(p.margin, currency)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </WidgetSection>
  )
}
