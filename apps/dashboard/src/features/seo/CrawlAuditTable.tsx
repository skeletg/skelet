import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import { Input } from '@/components/ui/input'
import { CrawlAuditDocument, type CrawlAuditQuery } from './LinkMap.gql'
import type { CrawlPageKind, SeoIssue } from '@/graphql/schema.types'

type Row = NonNullable<CrawlAuditQuery['crawlAudit']>['pages'][number]

const ISSUE_LABELS: Record<string, string> = {
  BROKEN: 'Error page',
  REDIRECT: 'Redirects',
  TITLE_MISSING: 'No title',
  TITLE_LONG: 'Title too long',
  TITLE_SHORT: 'Title too short',
  TITLE_DUPLICATE: 'Duplicate title',
  META_MISSING: 'No meta description',
  META_LONG: 'Meta too long',
  META_SHORT: 'Meta too short',
  META_DUPLICATE: 'Duplicate meta',
  H1_MISSING: 'No H1',
  H1_MULTIPLE: 'Multiple H1',
  HEADING_SKIPPED: 'Skipped heading level',
  NOINDEX: 'noindex',
  NOFOLLOW: 'nofollow',
  CANONICAL_NONE: 'No canonical',
  CANONICAL_OTHER: 'Canonical points elsewhere',
  ORPHAN: 'Nothing links here',
  DEEP: '4+ clicks deep',
  THIN: 'Thin content',
  NO_LINKS_OUT: 'No internal links out',
}

const KINDS: Array<{ value: CrawlPageKind | 'ALL'; label: string }> = [
  { value: 'ALL', label: 'All URLs' },
  { value: 'PAGE', label: 'Pages' },
  { value: 'FILTER', label: 'Facet URLs' },
  { value: 'PARAM', label: 'With parameters' },
]

// Green inside the range Google will actually render, amber outside it
function lengthTone(length: number, min: number, max: number): string {
  if (length === 0) return 'text-destructive'
  return length < min || length > max ? 'text-warning' : 'text-muted-foreground'
}

export function CrawlAuditTable({ channelId }: { channelId: string }) {
  const [issue, setIssue] = useState<SeoIssue | null>(null)
  const [kind, setKind] = useState<CrawlPageKind | 'ALL'>('ALL')
  const [search, setSearch] = useState('')

  const { data, loading } = useQuery(CrawlAuditDocument, {
    variables: {
      channelId,
      issue,
      kind: kind === 'ALL' ? null : kind,
      search: search.trim() || null,
      limit: 500,
    },
    fetchPolicy: 'cache-and-network',
  })
  const audit = data?.crawlAudit

  const counts = useMemo(() => {
    const rows = [...(audit?.issueCounts ?? [])]
    rows.sort((a, b) => b.count - a.count)
    return rows
  }, [audit])

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'path',
        header: ({ column }) => <DataTableColumnHeader column={column} title="URL" />,
        cell: ({ row }) => (
          <div className="max-w-[320px]">
            <div className="truncate font-mono text-xs">{row.original.path}</div>
            <div className="truncate text-xs text-muted-foreground">{row.original.title || '(no title)'}</div>
          </div>
        ),
      },
      {
        accessorKey: 'statusCode',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        cell: ({ row }) => {
          const code = row.original.statusCode
          const tone = code >= 400 || code === 0 ? 'destructive' : code >= 300 ? 'secondary' : 'outline'
          return <Badge variant={tone}>{code === 0 ? 'no response' : code}</Badge>
        },
      },
      {
        accessorKey: 'kind',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Kind" />,
        cell: ({ row }) =>
          row.original.kind === 'PAGE' ? (
            <span className="text-xs text-muted-foreground">page</span>
          ) : (
            <Badge variant="outline">{row.original.kind === 'FILTER' ? 'facet' : 'params'}</Badge>
          ),
      },
      {
        accessorKey: 'titleLength',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Title len" />,
        cell: ({ row }) => (
          <span className={`tabular-nums text-xs ${lengthTone(row.original.titleLength, 15, 60)}`}>
            {row.original.titleLength}
          </span>
        ),
      },
      {
        accessorKey: 'metaLength',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Meta len" />,
        cell: ({ row }) => (
          <span className={`tabular-nums text-xs ${lengthTone(row.original.metaLength, 50, 160)}`}>
            {row.original.metaLength}
          </span>
        ),
      },
      {
        id: 'headings',
        header: ({ column }) => <DataTableColumnHeader column={column} title="H1 / outline" />,
        cell: ({ row }) => {
          const counts = row.original.headingCounts
          const outline = counts.map((c, i) => (c > 0 ? `H${i + 1}×${c}` : null)).filter(Boolean).join(' ')
          return (
            <div className="max-w-[180px]">
              <div className={`text-xs ${row.original.h1Count === 1 ? 'text-muted-foreground' : 'text-destructive'}`}>
                {row.original.h1Count} H1
              </div>
              <div className="truncate text-xs text-muted-foreground">{outline || 'no headings'}</div>
            </div>
          )
        },
      },
      {
        accessorKey: 'internalLinks',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Links out / in" />,
        cell: ({ row }) => (
          <span className="tabular-nums text-xs text-muted-foreground">
            {row.original.internalLinks} / {row.original.inLinks}
          </span>
        ),
      },
      {
        accessorKey: 'canonical',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Canonical" />,
        cell: ({ row }) => (
          <div className="max-w-[180px] truncate font-mono text-xs text-muted-foreground">
            {row.original.canonical || '-'}
          </div>
        ),
      },
      {
        accessorKey: 'robotsMeta',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Robots" />,
        cell: ({ row }) => (
          <span className={`text-xs ${row.original.noindex ? 'text-destructive' : 'text-muted-foreground'}`}>
            {row.original.robotsMeta || '-'}
          </span>
        ),
      },
      {
        id: 'issues',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Findings" />,
        cell: ({ row }) => (
          <div className="flex max-w-[280px] flex-wrap gap-1">
            {row.original.issues.map((i) => (
              <Badge key={i} variant={i === 'BROKEN' ? 'destructive' : 'secondary'} className="text-[10px]">
                {ISSUE_LABELS[i] ?? i}
              </Badge>
            ))}
          </div>
        ),
      },
    ],
    [],
  )

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Filter by URL or title"
          className="h-8 w-64"
        />
        <div className="flex rounded-md border p-0.5">
          {KINDS.map((k) => (
            <button
              key={k.value}
              type="button"
              onClick={() => setKind(k.value)}
              className={`rounded-sm px-2 py-1 text-xs ${kind === k.value ? 'bg-accent font-medium' : 'text-muted-foreground hover:bg-accent/50'}`}
            >
              {k.label}
            </button>
          ))}
        </div>
        {issue ? (
          <button type="button" onClick={() => setIssue(null)} className="rounded-md border px-2 py-1 text-xs">
            Clear “{ISSUE_LABELS[issue] ?? issue}”
          </button>
        ) : null}
      </div>

      <div className="mb-3 flex flex-wrap gap-1.5">
        {counts.map((c) => (
          <button
            key={c.issue}
            type="button"
            onClick={() => setIssue(issue === c.issue ? null : (c.issue as SeoIssue))}
            className={`flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs ${issue === c.issue ? 'bg-accent font-medium' : ''}`}
          >
            {ISSUE_LABELS[c.issue] ?? c.issue}
            <span className="tabular-nums text-muted-foreground">{c.count}</span>
          </button>
        ))}
      </div>

      <DataTable
        columns={columns}
        data={audit?.pages ?? []}
        loading={loading && !audit}
        prefsKey="seo-crawl-audit"
        enableColumnControls
        exportCsv={{ filename: 'seo-audit.csv' }}
        pageSize={25}
      />
    </div>
  )
}
