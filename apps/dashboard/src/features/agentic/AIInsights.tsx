import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { PickStore, Tile } from '@/components/layout/panels'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { LoadingState } from '@/components/ui/loading-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useConfigScope } from '@/lib/config-scope'
import { formatCents, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { usePermissions } from '@/shared/auth/usePermissions'
import { AiInsightsDocument, type AiInsightsQuery } from './AIInsights.gql'

type Insights = AiInsightsQuery['aiInsights']
type Cohort = Insights['chatted']
type ProductRow = Insights['products'][number]
type AgentRow = Insights['agents'][number]

const WINDOWS = ['7', '30', '90'] as const

const rate = (part: number, whole: number) => (whole > 0 ? part / whole : null)
const percent = (r: number | null) => (r === null ? '-' : `${(r * 100).toFixed(r > 0 && r < 0.1 ? 1 : 0)}%`)

export function AIInsights() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { canAccessChannelScope } = usePermissions()
  const [days, setDays] = useState<(typeof WINDOWS)[number]>('30')
  const channelId = effectiveChannelId ?? ''
  const allowed = !!channelId && canAccessChannelScope('assistant.view', channelId)
  const { data, error, loading } = useQuery(AiInsightsDocument, {
    variables: { channelId, days: Number(days) },
    skip: !allowed,
    fetchPolicy: 'cache-and-network',
  })
  const ins = data?.aiInsights

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="AI insights"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">How the AI does as a seller - from Signals, so only shoppers it has seen count</span>}
        actions={
          <Select value={days} onValueChange={(v) => setDays(v as (typeof WINDOWS)[number])}>
            <SelectTrigger className="h-8 w-36" aria-label="Time range">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {WINDOWS.map((w) => (
                <SelectItem key={w} value={w}>
                  Last {w} days
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <PickStore why="AI insights are per store" />
          </DetailPageSection>
        ) : !allowed ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Not on this store</AlertTitle>
              <AlertDescription>Reading AI insights needs permission to view the shopping assistant on this store.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : error && !ins ? (
          <DetailPageSection>
            <Alert variant="destructive">
              <AlertTitle>Couldn't load the insights</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : !ins ? (
          <DetailPageSection>
            <LoadingState />
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection
              title="Chatted against didn't"
              description={`People seen on the storefront over the last ${days} days, split by whether they chatted with the shopping assistant. A chatter's order counts only after their first chat.`}
            >
              <Comparison chatted={ins.chatted} didNotChat={ins.didNotChat} currency={ins.currencyCode} />
            </DetailPageSection>
            <DetailPageSection
              title="Products the assistant shows"
              description={`What it put in front of shoppers, and how many of them added it to their cart within ${ins.cartWindowDays} days or ordered it within ${ins.orderWindowDays}.`}
            >
              <DataTable
                columns={productColumns}
                data={ins.products}
                loading={loading}
                getRowId={(p) => p.productId}
                searchPlaceholder="Search products…"
                initialSorting={[{ id: 'shown', desc: true }]}
                emptyState={<p className="py-6 text-sm text-muted-foreground">The assistant hasn't shown any products in this window.</p>}
              />
            </DetailPageSection>
            <DetailPageSection
              title="Shoppers' agents"
              description="Shoppers' own AI agents using the store's MCP endpoint, by the name each gives itself."
            >
              <div className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <Tile label="Orders from agent carts" value={formatNumber(ins.agentOrders)} sub="carts a shopper's agent filled" />
                  <Tile label="Their revenue" value={formatCents(ins.agentRevenue, ins.currencyCode)} sub={`in ${ins.currencyCode}`} />
                </div>
                <DataTable
                  columns={agentColumns}
                  data={ins.agents}
                  loading={loading}
                  getRowId={(a) => a.agent}
                  searchPlaceholder="Search agents…"
                  initialSorting={[{ id: 'calls', desc: true }]}
                  emptyState={<p className="py-6 text-sm text-muted-foreground">No shopper's agent has used the store in this window.</p>}
                />
              </div>
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

type Metric = {
  label: string
  chatted: string
  didNot: string
  diff: string | null
  // whether the chatters' side came out better, worse, or no call to make
  better: boolean | null
}

function metrics(a: Cohort, b: Cohort, currency: string): Metric[] {
  const conv = [rate(a.buyers, a.people), rate(b.buyers, b.people)] as const
  const aov = [a.revenueOrders > 0 ? a.revenue / a.revenueOrders : null, b.revenueOrders > 0 ? b.revenue / b.revenueOrders : null] as const
  const ret = [rate(a.returnedOrders, a.orders), rate(b.returnedOrders, b.orders)] as const
  const points = (x: number | null, y: number | null) => (x === null || y === null ? null : (x - y) * 100)
  const pp = (d: number | null) => (d === null ? null : `${d > 0 ? '+' : ''}${d.toFixed(1)} pts`)
  const relative = (x: number | null, y: number | null) => (x === null || y === null || y === 0 ? null : ((x - y) / y) * 100)
  const convDiff = points(conv[0], conv[1])
  const aovDiff = relative(aov[0], aov[1])
  const retDiff = points(ret[0], ret[1])
  return [
    { label: 'People', chatted: formatNumber(a.people), didNot: formatNumber(b.people), diff: null, better: null },
    { label: 'Converted', chatted: percent(conv[0]), didNot: percent(conv[1]), diff: pp(convDiff), better: convDiff === null || convDiff === 0 ? null : convDiff > 0 },
    { label: 'Orders', chatted: formatNumber(a.orders), didNot: formatNumber(b.orders), diff: null, better: null },
    {
      label: 'Average order',
      chatted: aov[0] === null ? '-' : formatCents(Math.round(aov[0]), currency),
      didNot: aov[1] === null ? '-' : formatCents(Math.round(aov[1]), currency),
      diff: aovDiff === null ? null : `${aovDiff > 0 ? '+' : ''}${aovDiff.toFixed(0)}%`,
      better: aovDiff === null || aovDiff === 0 ? null : aovDiff > 0,
    },
    // fewer returns is the better side here
    { label: 'Returned', chatted: percent(ret[0]), didNot: percent(ret[1]), diff: pp(retDiff), better: retDiff === null || retDiff === 0 ? null : retDiff < 0 },
  ]
}

function Comparison({ chatted, didNotChat, currency }: { chatted: Cohort; didNotChat: Cohort; currency: string }) {
  if (chatted.people === 0) {
    return <p className="py-6 text-sm text-muted-foreground">Nobody chatted with the assistant in this window, so there's nothing to compare yet.</p>
  }
  const rows = metrics(chatted, didNotChat, currency)
  const conv = rows[1]
  return (
    <div className="space-y-3">
      <p className="text-sm">
        Shoppers who chatted converted at <span className="font-medium tabular-nums">{conv.chatted}</span>, against{' '}
        <span className="font-medium tabular-nums">{conv.didNot}</span> for those who didn't.
      </p>
      <div className="overflow-x-auto rounded-md border">
        <table className="w-full text-sm">
          <thead className="border-b bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 text-left font-medium" scope="col" />
              <th className="px-3 py-2 text-right font-medium" scope="col">Chatted</th>
              <th className="px-3 py-2 text-right font-medium" scope="col">Didn't chat</th>
              <th className="px-3 py-2 text-right font-medium" scope="col">Difference</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {rows.map((m) => (
              <tr key={m.label}>
                <th className="px-3 py-2 text-left font-normal text-muted-foreground" scope="row">{m.label}</th>
                <td className="px-3 py-2 text-right tabular-nums">{m.chatted}</td>
                <td className="px-3 py-2 text-right tabular-nums">{m.didNot}</td>
                <td className={cn('px-3 py-2 text-right tabular-nums', m.better === true && 'text-success', m.better === false && 'text-destructive')}>
                  {m.diff ?? ''}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        It compares, it doesn't prove cause - shoppers who start a chat may already be keener to buy.
      </p>
    </div>
  )
}

function num<T extends ProductRow | AgentRow>(id: string, title: string, value: (r: T) => number): ColumnDef<T> {
  return {
    id,
    accessorFn: value,
    header: ({ column }) => <DataTableColumnHeader column={column} title={title} />,
    cell: ({ row }) => <span className="tabular-nums">{formatNumber(value(row.original))}</span>,
  }
}

function share(id: string, title: string, part: (r: ProductRow) => number): ColumnDef<ProductRow> {
  return {
    id,
    accessorFn: (r) => rate(part(r), r.people) ?? 0,
    header: ({ column }) => <DataTableColumnHeader column={column} title={title} />,
    cell: ({ row }) => (
      <span className="tabular-nums">
        {percent(rate(part(row.original), row.original.people))}
        <span className="ml-1 text-xs text-muted-foreground">({formatNumber(part(row.original))})</span>
      </span>
    ),
  }
}

const productColumns: ColumnDef<ProductRow>[] = [
  {
    id: 'name',
    accessorFn: (p) => p.name ?? '',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Product" />,
    cell: ({ row }) =>
      row.original.name ? (
        <Link to={`/products/${row.original.productId}`} className="block max-w-[320px] truncate font-medium hover:underline">
          {row.original.name}
        </Link>
      ) : (
        <span className="text-muted-foreground">Removed product</span>
      ),
  },
  num<ProductRow>('shown', 'Shown', (p) => p.shown),
  num<ProductRow>('people', 'People', (p) => p.people),
  share('carted', 'Added to cart', (p) => p.carted),
  share('ordered', 'Ordered', (p) => p.ordered),
]

const agentColumns: ColumnDef<AgentRow>[] = [
  {
    accessorKey: 'agent',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Agent" />,
    cell: ({ row }) => <span className="block max-w-[260px] truncate font-medium">{row.original.agent || 'Unnamed'}</span>,
  },
  num<AgentRow>('calls', 'Calls', (a) => a.calls),
  num<AgentRow>('searches', 'Searches', (a) => a.searches),
  {
    id: 'emptySearches',
    accessorFn: (a) => a.emptySearches,
    header: ({ column }) => <DataTableColumnHeader column={column} title="Found nothing" />,
    cell: ({ row }) => (
      <span className={cn('tabular-nums', row.original.emptySearches > 0 && 'text-warning')}>{formatNumber(row.original.emptySearches)}</span>
    ),
  },
  {
    id: 'tools',
    enableSorting: false,
    header: 'Most used',
    cell: ({ row }) => (
      <span className="text-xs text-muted-foreground">
        {[...row.original.tools]
          .sort((x, y) => y.calls - x.calls)
          .slice(0, 3)
          .map((t) => `${t.tool} (${formatNumber(t.calls)})`)
          .join(', ') || '-'}
      </span>
    ),
  },
]
