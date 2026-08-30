import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { OrderStatusBadge } from '@/features/orders/OrderStatusBadge'
import { OrdersPagedDocument } from '@/features/orders/Orders.gql'
import { WidgetSection } from './primitives'

const PAY_LABEL: Record<string, string> = {
  PAID: 'Paid',
  UNPAID: 'Unpaid',
  AUTHORIZED: 'Authorized',
  PARTIALLY_CAPTURED: 'Part. captured',
  PARTIALLY_REFUNDED: 'Part. refunded',
  REFUNDED: 'Refunded',
  VOIDED: 'Voided',
}

export function RecentOrdersWidget() {
  const { effectiveChannelId } = useConfigScope()
  // paged, so the dashboard asks for eight rows instead of the whole order table
  const { data, loading, error } = useQuery(OrdersPagedDocument, {
    variables: { limit: 8, channelId: effectiveChannelId ?? null },
    fetchPolicy: 'cache-and-network',
  })
  const orders = data?.ordersPaged.items ?? []

  return (
    <WidgetSection
      title="Recent orders"
      description="The latest orders across the store."
      to="/orders"
      loading={loading && orders.length === 0}
      error={error ?? null}
      empty={!loading && orders.length === 0}
    >
      <div className="overflow-x-auto rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
              <th className="px-3 py-2">Order</th>
              <th className="px-3 py-2">Customer</th>
              <th className="px-3 py-2">Status</th>
              <th className="px-3 py-2">Payment</th>
              <th className="px-3 py-2 text-right">Total</th>
              <th className="px-3 py-2 text-right">Placed</th>
            </tr>
          </thead>
          <tbody>
            {orders.map((o) => {
              const name = `${o.shippingAddress?.firstName ?? ''} ${o.shippingAddress?.lastName ?? ''}`.trim()
              return (
                <tr key={o.id} className="border-b last:border-0 hover:bg-muted/40">
                  <td className="px-3 py-2">
                    <Link to={`/orders/${o.id}`} className="font-medium hover:underline">
                      #{o.number}
                    </Link>
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">{name || '-'}</td>
                  <td className="px-3 py-2"><OrderStatusBadge status={o.status} /></td>
                  <td className="px-3 py-2">
                    <Badge variant={o.paymentStatus === 'PAID' ? 'default' : 'outline'}>
                      {PAY_LABEL[o.paymentStatus] ?? o.paymentStatus}
                    </Badge>
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">{formatCents(o.total, o.currencyCode)}</td>
                  <td className="px-3 py-2 text-right text-muted-foreground">
                    {new Date(o.createdAt).toLocaleDateString(undefined, { dateStyle: 'medium' })}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </WidgetSection>
  )
}
