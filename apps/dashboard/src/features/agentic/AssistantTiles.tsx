import { Tile } from '@/components/layout/panels'
import { formatCents } from '@/lib/format'
import type { AssistantReportQuery } from '@/features/dashboard/Assistant.gql'

// The fields both the Home widget and the overview read - either query's report fits
export type AssistantNumbers = Pick<AssistantReportQuery['assistantReport'], 'turns' | 'failedTurns' | 'unansweredTurns' | 'cartTurns' | 'checkoutTurns' | 'usage' | 'orders'>

const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

function usd(amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD', maximumFractionDigits: amount < 1 ? 3 : 2 }).format(amount)
}

// Revenue stays per currency - the store's main one leads, any other is a count
function revenue(rows: AssistantNumbers['orders'], pick: (r: AssistantNumbers['orders'][number]) => [number, number]): { orders: number; value: string } {
  const sold = rows.filter((r) => pick(r)[0] > 0)
  const orders = sold.reduce((n, r) => n + pick(r)[0], 0)
  if (sold.length === 0) return { orders, value: '-' }
  const [first, ...rest] = sold
  return { orders, value: formatCents(pick(first)[1], first.currencyCode) + (rest.length > 0 ? ` +${rest.length} more` : '') }
}

const share = (n: number, of: number) => `${Math.round((100 * n) / Math.max(of, 1))}%`

// unanswered links the count to the conversations it came from, where the store keeps them
export function AssistantTiles({ report: r, unanswered }: { report: AssistantNumbers; unanswered?: string }) {
  const all = r.orders.reduce((n, o) => n + o.orderCount, 0)
  const assisted = revenue(r.orders, (o) => [o.assistantOrders, o.assistantRevenue])
  const agents = revenue(r.orders, (o) => [o.agentOrders, o.agentRevenue])
  const u = r.usage
  const tokens = u.inputTokens + u.outputTokens + u.cacheReadTokens + u.cacheWriteTokens
  const cached = tokens > 0 ? `${share(u.cacheReadTokens, tokens)} from cache` : ''
  const cost = u.estimatedCostUsd
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Tile
        label="Messages answered"
        value={r.turns.toLocaleString()}
        sub={r.failedTurns > 0 ? `${r.failedTurns.toLocaleString()} cut short` : 'none cut short'}
      />
      <Tile
        label="Couldn't answer"
        value={r.unansweredTurns.toLocaleString()}
        sub={r.turns > 0 ? `${share(r.unansweredTurns, r.turns)} of messages` : 'nothing asked yet'}
        tone={r.turns > 0 && r.unansweredTurns / r.turns > 0.1 ? 'warn' : undefined}
        to={r.unansweredTurns > 0 ? unanswered : undefined}
      />
      <Tile label="Filled a cart" value={r.cartTurns.toLocaleString()} sub={`${r.checkoutTurns.toLocaleString()} sent to checkout`} />
      <Tile
        label="Assisted orders"
        value={assisted.orders.toLocaleString()}
        sub={assisted.orders > 0 ? `${assisted.value} · ${share(assisted.orders, all)} of orders` : 'none yet'}
      />
      {cost == null ? (
        <Tile label="Model use" value={`${compact.format(tokens)} tokens`} sub={cached ? `${cached} · add prices for a cost` : 'add prices for a cost'} to="/settings/ai/usage" />
      ) : (
        <Tile
          label="Model cost"
          value={usd(cost)}
          sub={[assisted.orders > 0 ? `${usd(cost / assisted.orders)} per assisted order` : '', `${compact.format(tokens)} tokens`, cached].filter(Boolean).join(' · ')}
        />
      )}
      <Tile label="Shoppers' own agents" value={agents.orders.toLocaleString()} sub={agents.orders > 0 ? `orders · ${agents.value}` : 'orders from a handoff link'} />
    </div>
  )
}
