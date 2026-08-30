import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { CartsDocument } from '@/features/carts/Carts.gql'
import { NeedsChannel, WidgetSection } from './primitives'

const ROWS = 5

function since(iso: string): string {
  const hours = Math.round((Date.now() - new Date(iso).getTime()) / 3_600_000)
  if (hours < 1) return 'just now'
  if (hours < 24) return `${hours}h ago`
  return `${Math.round(hours / 24)}d ago`
}

export function AbandonedCartsWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(CartsDocument, {
    variables: { channelId: effectiveChannelId ?? '', abandoned: true, limit: ROWS },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const rows = data?.carts.items ?? []
  const total = data?.carts.totalCount ?? 0

  return (
    <WidgetSection
      title="Abandoned carts"
      description="Baskets left behind - a recovery email or a nudge still lands."
      to="/carts"
      loading={!!effectiveChannelId && loading && rows.length === 0}
      error={error ?? null}
      trailing={
        total > 0 ? <span className="text-xs text-muted-foreground">{total.toLocaleString()} total</span> : null
      }
    >
      {!effectiveChannelId ? (
        <NeedsChannel what="abandoned carts" />
      ) : rows.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">Nothing abandoned right now.</p>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                <th className="px-3 py-2">Customer</th>
                <th className="px-3 py-2 text-right">Items</th>
                <th className="px-3 py-2 text-right">Value</th>
                <th className="px-3 py-2 text-right">Last touched</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((c) => (
                <tr key={c.id} className="border-b last:border-0 hover:bg-muted/40">
                  <td className="px-3 py-2">
                    <Link to={`/carts/${c.id}`} className="font-medium hover:underline">
                      {c.customerName || c.customerEmail || 'Guest'}
                    </Link>
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">{c.itemCount}</td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {formatCents(c.subtotal, c.currencyCode)}
                  </td>
                  <td className="px-3 py-2 text-right text-muted-foreground">{since(c.updatedAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </WidgetSection>
  )
}
