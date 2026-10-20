import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { type LensSearchAnalyticsQuery, LensSearchAnalyticsDocument } from './Lens.gql'
import { Empty, Tile } from '@/components/layout/panels'
import { formatNumber } from '@/lib/format'
import { LoadError } from '@/shared/errors/LoadError'

type TermRow = LensSearchAnalyticsQuery['lensSearchAnalytics']['top'][number]

// LensSearch reports on-site search: headline totals, a daily volume trend, the most-searched terms, and zero-result
// searches (the merchandising-gap list) Rollup-backed (dim='search' / 'search_zero'), so it scales with traffic
export function LensSearch() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { data, loading, error, refetch } = useQuery(LensSearchAnalyticsDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const a = data?.lensSearchAnalytics
  const top = a?.top ?? []
  const zero = a?.zeroResult ?? []
  const daily = a?.daily ?? []
  const zeroRate = a && a.totalSearches > 0 ? Math.round((a.zeroResultSearches / a.totalSearches) * 100) : 0
  const maxTop = Math.max(1, ...top.map((t) => t.count))

  const topColumns = useMemo<ColumnDef<TermRow>[]>(
    () => [
      {
        accessorKey: 'term',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Search term" />,
        cell: ({ row }) => <span className="block max-w-[360px] truncate">{row.original.term}</span>,
      },
      {
        accessorKey: 'count',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Searches" />,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <span className="w-12 shrink-0 tabular-nums">{formatNumber(row.original.count)}</span>
            <Bar value={Math.round((row.original.count / maxTop) * 100)} />
          </div>
        ),
      },
    ],
    [maxTop],
  )

  const zeroColumns = useMemo<ColumnDef<TermRow>[]>(
    () => [
      {
        accessorKey: 'term',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Search term" />,
        cell: ({ row }) => (
          <span className="block max-w-[360px] truncate font-medium text-warning">
            {row.original.term}
          </span>
        ),
      },
      {
        accessorKey: 'count',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Searches" />,
        cell: ({ row }) => <span className="tabular-nums">{formatNumber(row.original.count)}</span>,
      },
    ],
    [],
  )

  if (error && !data) {
    return <LoadError title="Couldn't load search analytics" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Search analytics"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            <Link to="/lens" className="underline hover:text-foreground">
              Lens
            </Link>
            {' · '}on-site search terms &amp; zero-result searches - last 30 days
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a store</AlertTitle>
              <AlertDescription>Search analytics are per store - select one from the channel switcher.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection title="At a glance">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Tile label="Total searches" value={formatNumber(a?.totalSearches ?? 0)} sub="last 30 days" />
                <Tile label="Distinct terms" value={formatNumber(a?.distinctTerms ?? 0)} sub="unique queries" />
                <Tile label="Zero-result searches" value={formatNumber(a?.zeroResultSearches ?? 0)} sub="returned nothing" />
                <Tile
                  label="Zero-result rate"
                  value={`${zeroRate}%`}
                  sub="of all searches"
                  tone={zeroRate >= 20 ? 'warn' : undefined}
                />
              </div>
            </DetailPageSection>

            <DetailPageSection title="Search volume" description="Daily searches; the amber portion of each bar is zero-result searches.">
              {daily.length === 0 ? (
                <Empty>No searches captured yet.</Empty>
              ) : (
                <TrendBars daily={daily} />
              )}
            </DetailPageSection>

            <DetailPageSection title="Top searches" description="The most-searched terms (normalized: lower-cased, whitespace-collapsed).">
              <DataTable
                columns={topColumns}
                data={top}
                loading={loading}
                getRowId={(t) => t.term}
                searchPlaceholder="Search terms…"
                initialSorting={[{ id: 'count', desc: true }]}
                emptyState={<p className="py-6 text-sm text-muted-foreground">No searches captured yet.</p>}
              />
            </DetailPageSection>

            <DetailPageSection
              title="Zero-result searches"
              description="Searches that returned no products - demand your catalog or search synonyms aren't meeting."
            >
              <DataTable
                columns={zeroColumns}
                data={zero}
                loading={loading}
                getRowId={(t) => t.term}
                searchPlaceholder="Search terms…"
                initialSorting={[{ id: 'count', desc: true }]}
                emptyState={<p className="py-6 text-sm text-muted-foreground">No zero-result searches 🎉</p>}
              />
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// TrendBars draws a compact daily search-volume bar chart (no pie - per the dashboard charting convention) - Each day
// is a column whose height is the day's search count; the amber base shows the zero-result share within that day
function TrendBars({ daily }: { daily: { date: string; searches: number; zeroResults: number }[] }) {
  const max = Math.max(1, ...daily.map((d) => d.searches))
  return (
    <div>
      <div className="flex h-32 items-end gap-0.5">
        {daily.map((d) => {
          const h = (d.searches / max) * 100
          const zh = d.searches > 0 ? (d.zeroResults / d.searches) * 100 : 0
          return (
            <div
              key={d.date}
              className="flex flex-1 flex-col justify-end"
              title={`${d.date}: ${d.searches} searches, ${d.zeroResults} zero-result`}
            >
              <div className="relative w-full rounded-t bg-primary/70" style={{ height: `${Math.max(h, d.searches > 0 ? 4 : 0)}%` }}>
                {zh > 0 && <div className="absolute inset-x-0 bottom-0 bg-warning" style={{ height: `${zh}%` }} />}
              </div>
            </div>
          )
        })}
      </div>
      <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
        <span>{daily[0]?.date}</span>
        <span className="flex items-center gap-3">
          <span className="flex items-center gap-1">
            <span className="inline-block h-2 w-2 rounded-sm bg-primary/70" /> searches
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block h-2 w-2 rounded-sm bg-warning" /> zero-result
          </span>
        </span>
        <span>{daily[daily.length - 1]?.date}</span>
      </div>
    </div>
  )
}

function Bar({ value }: { value: number }) {
  return (
    <span className="h-1.5 min-w-16 flex-1 overflow-hidden rounded-full bg-muted">
      <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
    </span>
  )
}
