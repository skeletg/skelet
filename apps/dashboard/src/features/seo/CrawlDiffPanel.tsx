import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ArrowRightIcon, TriangleAlertIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { CrawlDiffDocument } from './LinkMap.gql'

const CHANGE_LABELS: Record<string, string> = {
  NOW_BROKEN: 'Started erroring',
  FIXED: 'Fixed',
  NOW_REDIRECT: 'Started redirecting',
  NOW_NOINDEX: 'Became noindex',
  NOW_INDEXABLE: 'Became indexable',
  NOW_ORPHAN: 'Lost every inbound link',
  LOST_IN_LINKS: 'Lost inbound links',
  DEEPER: 'Moved deeper',
  TITLE_CHANGED: 'Title changed',
  META_CHANGED: 'Meta description changed',
  CANONICAL_CHANGED: 'Canonical changed',
  H1_CHANGED: 'H1 changed',
  GONE: 'No longer reachable',
  NEW: 'New page',
}

function when(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function CrawlDiffPanel({ channelId }: { channelId: string }) {
  const [regressionsOnly, setRegressionsOnly] = useState(true)
  const { data } = useQuery(CrawlDiffDocument, {
    variables: { channelId, regressionsOnly, limit: 300 },
    fetchPolicy: 'cache-and-network',
  })
  const diff = data?.crawlDiff

  const counts = useMemo(() => {
    const rows = [...(diff?.counts ?? [])]
    rows.sort((a, b) => b.count - a.count)
    return rows
  }, [diff])

  if (!diff) {
    return (
      <p className="text-sm text-muted-foreground">
        Nothing to compare yet - a second completed crawl is what turns this page into a regression watch.
      </p>
    )
  }

  return (
    <div>
      <p className="text-xs text-muted-foreground">
        {when(diff.base.startedAt)} ({diff.base.pagesCrawled.toLocaleString()} pages) →{' '}
        {when(diff.current.startedAt)} ({diff.current.pagesCrawled.toLocaleString()} pages) ·{' '}
        {diff.compared.toLocaleString()} pages in both
      </p>

      {!diff.coverageComparable ? (
        <Alert className="mt-3">
          <TriangleAlertIcon />
          <AlertTitle>Both crawls stopped on their page budget</AlertTitle>
          <AlertDescription>
            So the two runs cover different slices of the site, and a page missing from one is not evidence it went
            away. Appeared and disappeared are left out of this comparison for that reason - raise the budget on both
            runs to get them.
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <button
          type="button"
          onClick={() => setRegressionsOnly(!regressionsOnly)}
          className={`rounded-md border px-2 py-1 text-xs ${regressionsOnly ? 'bg-accent font-medium' : 'text-muted-foreground hover:bg-accent/50'}`}
        >
          {regressionsOnly ? 'Regressions only' : 'Every change'}
        </button>
        {counts.map((c) => (
          <span key={c.kind} className="flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs">
            {CHANGE_LABELS[c.kind] ?? c.kind}
            <span className="tabular-nums text-muted-foreground">{c.count}</span>
          </span>
        ))}
      </div>

      {diff.changes.length === 0 ? (
        <p className="mt-4 text-sm text-muted-foreground">
          {regressionsOnly ? 'No regressions since the last crawl.' : 'Nothing changed since the last crawl.'}
        </p>
      ) : (
        <div className="mt-3 divide-y rounded-md border">
          {diff.changes.map((c) => (
            <div
              key={`${c.kind}:${c.path}`}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 border-l-2 px-3 py-2 text-xs"
              style={{ borderLeftColor: c.isRegression ? 'var(--destructive)' : 'transparent' }}
            >
              <Badge variant={c.isRegression ? 'destructive' : 'secondary'} className="text-[10px]">
                {CHANGE_LABELS[c.kind] ?? c.kind}
              </Badge>
              <span className="min-w-0 flex-1 truncate font-mono">{c.path}</span>
              {c.before || c.after ? (
                <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
                  <span className="max-w-[14rem] truncate line-through">{c.before || '(empty)'}</span>
                  <ArrowRightIcon className="size-3 shrink-0" />
                  <span className="max-w-[14rem] truncate">{c.after || '(empty)'}</span>
                </span>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
