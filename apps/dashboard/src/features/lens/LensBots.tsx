import { useState, type ReactNode } from 'react'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { useQuery } from '@apollo/client/react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { DataTable } from '@/components/data-table'
import { useConfigScope } from '@/lib/config-scope'
import { BotTrafficDocument } from './Lens.signals.gql'

const nfmt = (n: number) => n.toLocaleString()
const fmtShort = (v: string) => {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
const fmtWhen = (v?: string | null) => {
  if (!v) return '-'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

// Category → friendly label + colour, used for the per-agent badges and the category roll-up. "ai" is first because
// it's the headline use case (which LLMs crawl me)
const CAT: Record<string, { label: string; color: string }> = {
  ai: { label: 'AI / LLM', color: '#8b5cf6' },
  search: { label: 'Search', color: '#0ea5e9' },
  social: { label: 'Social', color: '#ec4899' },
  seo: { label: 'SEO tool', color: '#f59e0b' },
  monitoring: { label: 'Monitoring', color: '#10b981' },
  scraper: { label: 'Scraper', color: '#ef4444' },
  other: { label: 'Other', color: '#64748b' },
}
const catOf = (c: string) => CAT[c] ?? CAT.other
const HUMAN = '#10b981'
const BOT = '#8b5cf6'

export function LensBots() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [bot, setBot] = useState('')

  const { data, loading } = useQuery(BotTrafficDocument, {
    variables: { channelId, sinceDays: 30, bot },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const bt = data?.botTraffic

  const total = bt ? bt.botHits + bt.humanViews : 0

  type BotRow = NonNullable<typeof bt>['bots'][number]
  type PageRow = NonNullable<typeof bt>['topPages'][number]

  const botColumns: ColumnDef<BotRow>[] = [
    { accessorKey: 'name', header: 'Agent', cell: ({ row }) => <span className="font-medium">{row.original.name}</span> },
    {
      accessorKey: 'category',
      header: 'Category',
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="size-2 rounded-full" style={{ background: catOf(row.original.category).color }} />
          {catOf(row.original.category).label}
        </span>
      ),
    },
    { accessorKey: 'hits', header: () => <div className="text-right">Requests</div>, cell: ({ row }) => <div className="text-right tabular-nums">{nfmt(row.original.hits)}</div> },
    { accessorKey: 'pages', header: () => <div className="text-right">Pages</div>, cell: ({ row }) => <div className="text-right tabular-nums">{nfmt(row.original.pages)}</div> },
    { accessorKey: 'lastSeen', header: () => <div className="text-right">Last seen</div>, cell: ({ row }) => <div className="text-right text-muted-foreground">{fmtWhen(row.original.lastSeen)}</div> },
  ]

  const pageColumns: ColumnDef<PageRow>[] = [
    { accessorKey: 'pageKey', header: 'Page', cell: ({ row }) => <span className="block max-w-[420px] truncate font-mono">{row.original.pageKey || '/'}</span> },
    { accessorKey: 'hits', header: () => <div className="text-right">Crawls</div>, cell: ({ row }) => <div className="text-right tabular-nums">{nfmt(row.original.hits)}</div> },
    // The bot column is meaningless once the list is filtered to one agent
    ...(bot ? [] : [{ accessorKey: 'bots', header: () => <div className="text-right">Bots</div>, cell: ({ row }) => <div className="text-right tabular-nums">{nfmt(row.original.bots)}</div> } as ColumnDef<PageRow>]),
  ]
  const botShare = total > 0 ? Math.round((bt!.botHits / total) * 100) : 0
  const series = (bt?.daily ?? []).map((d) => ({ day: fmtShort(d.day), Humans: d.humanViews, Bots: d.botHits }))

  // Category roll-up from the per-agent rows
  const byCat = new Map<string, number>()
  for (const b of bt?.bots ?? []) byCat.set(b.category, (byCat.get(b.category) ?? 0) + b.hits)
  const cats = [...byCat.entries()].sort((a, b) => b[1] - a[1])

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Bot traffic"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Crawlers &amp; AI agents hitting your storefront - server-observed, so even bots that never run page
            scripts (GPTBot, ClaudeBot, scrapers) are counted. Last 30 days.
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Bot traffic is per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : !bt ? (
          <DetailPageSection>
            <p className="py-6 text-sm text-muted-foreground">{loading ? 'Loading…' : 'No data.'}</p>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection title="At a glance">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Tile label="Bot requests" value={nfmt(bt.botHits)} sub="server-observed crawls" />
                <Tile label="Human page views" value={nfmt(bt.humanViews)} sub="bots excluded" />
                <Tile label="Distinct bots" value={nfmt(bt.botCount)} sub="named agents seen" />
                <Tile label="Pages crawled" value={nfmt(bt.pageCount)} sub="distinct URLs" />
              </div>
              {total > 0 && (
                <div className="mt-3 rounded border p-3">
                  <div className="mb-1.5 flex items-center justify-between text-xs text-muted-foreground">
                    <span>Bot vs human traffic</span>
                    <span>{botShare}% bot</span>
                  </div>
                  <div className="flex h-2.5 overflow-hidden rounded-full bg-muted">
                    <div style={{ width: `${100 - botShare}%`, background: HUMAN }} title={`Human: ${nfmt(bt.humanViews)}`} />
                    <div style={{ width: `${botShare}%`, background: BOT }} title={`Bot: ${nfmt(bt.botHits)}`} />
                  </div>
                  <div className="mt-1.5 flex gap-4 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: HUMAN }} /> Human</span>
                    <span className="flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: BOT }} /> Bot</span>
                  </div>
                </div>
              )}
            </DetailPageSection>

            <DetailPageSection title="Trend" description="Daily bot crawls vs human page views.">
              <Panel title="Bot vs human - last 30 days">
                {series.length === 0 ? (
                  <Empty>No traffic yet.</Empty>
                ) : (
                  <div className="h-60">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={series} margin={{ top: 8, right: 12, bottom: 0, left: -16 }}>
                        <defs>
                          <linearGradient id="bot-h" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="0%" stopColor={HUMAN} stopOpacity={0.3} />
                            <stop offset="100%" stopColor={HUMAN} stopOpacity={0} />
                          </linearGradient>
                          <linearGradient id="bot-b" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="0%" stopColor={BOT} stopOpacity={0.3} />
                            <stop offset="100%" stopColor={BOT} stopOpacity={0} />
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                        <XAxis dataKey="day" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={24} />
                        <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={32} />
                        <Tooltip content={<ChartTooltipContent />} />
                        <Area type="monotone" dataKey="Humans" stroke={HUMAN} strokeWidth={2} fill="url(#bot-h)" />
                        <Area type="monotone" dataKey="Bots" stroke={BOT} strokeWidth={2} fill="url(#bot-b)" />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                )}
              </Panel>
            </DetailPageSection>

            {cats.length > 0 && (
              <DetailPageSection title="By category" description="Where the crawler traffic comes from.">
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  {cats.map(([c, hits]) => (
                    <div key={c} className="flex items-center gap-3 rounded border p-3">
                      <span className="size-2.5 shrink-0 rounded-full" style={{ background: catOf(c).color }} />
                      <span className="flex-1 text-sm">{catOf(c).label}</span>
                      <span className="text-sm font-semibold tabular-nums">{nfmt(hits)}</span>
                    </div>
                  ))}
                </div>
              </DetailPageSection>
            )}

            <DetailPageSection title="Bots" description="Every crawler seen, by volume. Click one to filter the crawled-pages list.">
              <Panel title="Agents">
                <DataTable
                  columns={botColumns}
                  data={bt.bots}
                  getRowId={(row) => row.name}
                  emptyState="No bots seen yet."
                  compact
                  onRowClick={(row) => setBot((cur) => (cur === row.name ? '' : row.name))}
                  rowIsActive={(row) => row.name === bot}
                />
              </Panel>
            </DetailPageSection>

            <DetailPageSection
              title="Crawled pages"
              description={bot ? `Pages crawled by ${bot}.` : 'Most-crawled pages across all bots.'}
            >
              <Panel title="Pages">
                {bot && (
                  <button
                    type="button"
                    onClick={() => setBot('')}
                    className="mb-2 inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs hover:bg-muted"
                  >
                    {bot} <span className="text-muted-foreground">✕</span>
                  </button>
                )}
                <DataTable
                  columns={pageColumns}
                  data={bt.topPages}
                  getRowId={(row) => row.pageKey}
                  emptyState={`No crawled pages${bot ? ` for ${bot}` : ''} yet.`}
                  compact
                />
              </Panel>
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function Tile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
      <div className="p-3">{children}</div>
    </div>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>
}
