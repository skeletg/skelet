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
import { type LensPerformanceQuery, LensPerformanceDocument } from './Lens.gql'
import { LoadError } from '@/shared/errors/LoadError'

type Row = LensPerformanceQuery['lensPerformance'][number]
type Metric = 'lcp' | 'cls' | 'inp' | 'ttfb'

// Core Web Vitals thresholds (good ≤ / poor >) - LCP/INP/TTFB in ms, CLS unitless
const THRESHOLDS: Record<Metric, [number, number]> = {
  lcp: [2500, 4000],
  inp: [200, 500],
  cls: [0.1, 0.25],
  ttfb: [800, 1800],
}

function rate(metric: Metric, v: number): string {
  const [good, poor] = THRESHOLDS[metric]
  if (v <= good) return 'text-success'
  if (v <= poor) return 'text-warning'
  return 'text-destructive'
}
function fmt(metric: Metric, v: number): string {
  if (metric === 'cls') return v.toFixed(3)
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`
  return `${Math.round(v)}ms`
}

function VitalCell({ metric, value }: { metric: Metric; value?: number | null }) {
  if (value == null) return <span className="text-xs text-muted-foreground">-</span>
  return <span className={`tabular-nums font-medium ${rate(metric, value)}`}>{fmt(metric, value)}</span>
}

// LensPerformance shows p75 Core Web Vitals per page (real-user monitoring) -
// captured by the storefront via the web-vitals lib, aggregated from the spine
export function LensPerformance() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { data, loading, error, refetch } = useQuery(LensPerformanceDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const pages = data?.lensPerformance ?? []

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'pageKey',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Page" />,
        cell: ({ row }) => <span className="block max-w-[320px] truncate font-mono text-sm">{row.original.pageKey || '/'}</span>,
      },
      {
        accessorKey: 'lcp',
        header: ({ column }) => <DataTableColumnHeader column={column} title="LCP" />,
        cell: ({ row }) => <VitalCell metric="lcp" value={row.original.lcp} />,
      },
      {
        accessorKey: 'inp',
        header: ({ column }) => <DataTableColumnHeader column={column} title="INP" />,
        cell: ({ row }) => <VitalCell metric="inp" value={row.original.inp} />,
      },
      {
        accessorKey: 'cls',
        header: ({ column }) => <DataTableColumnHeader column={column} title="CLS" />,
        cell: ({ row }) => <VitalCell metric="cls" value={row.original.cls} />,
      },
      {
        accessorKey: 'ttfb',
        header: ({ column }) => <DataTableColumnHeader column={column} title="TTFB" />,
        cell: ({ row }) => <VitalCell metric="ttfb" value={row.original.ttfb} />,
      },
      {
        accessorKey: 'samples',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Samples" />,
        cell: ({ row }) => <span className="tabular-nums text-muted-foreground">{row.original.samples.toLocaleString()}</span>,
      },
    ],
    [],
  )

  if (error && !data) {
    return <LoadError title="Couldn't load performance" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Performance"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            <Link to="/lens" className="underline hover:text-foreground">
              Lens
            </Link>
            {' · '}p75 Core Web Vitals per page (real-user monitoring) - last 30 days
          </span>
        }
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a store</AlertTitle>
              <AlertDescription>Performance is per store - select one from the channel switcher.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection
            title="Web vitals by page"
            description="p75 of each metric. Green = good, amber = needs improvement, red = poor (Google's Core Web Vitals thresholds)."
          >
            <DataTable
              columns={columns}
              data={pages}
              loading={loading}
              getRowId={(p) => p.pageKey || '/'}
              searchPlaceholder="Search pages…"
              initialSorting={[{ id: 'samples', desc: true }]}
              emptyState={<p className="py-6 text-sm text-muted-foreground">No web-vitals samples captured yet.</p>}
            />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
