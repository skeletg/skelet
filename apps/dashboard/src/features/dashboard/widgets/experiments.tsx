import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { useConfigScope } from '@/lib/config-scope'
import { ExperimentsOverviewDocument } from '@/features/prism/Prism.gql'
import { Empty, NeedsChannel, Tile, WidgetSection } from './primitives'

const DAYS = 30
const ROWS = 4

export function ExperimentsWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(ExperimentsOverviewDocument, {
    variables: { channelId: effectiveChannelId ?? '', sinceDays: DAYS },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const overview = data?.experimentsOverview
  const rows = (overview?.experiments ?? []).slice(0, ROWS)

  return (
    <WidgetSection
      title="Experiments"
      description={`Running tests and whether any of them has decided, over ${DAYS} days.`}
      to="/prism"
      loading={!!effectiveChannelId && loading && !overview}
      error={error ?? null}
    >
      {!effectiveChannelId ? (
        <NeedsChannel what="experiments" />
      ) : (
        <div className="grid gap-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <Tile label="Running" value={String(overview?.running ?? 0)} sub={`${overview?.draft ?? 0} in draft`} />
            <Tile label="Decided" value={String(overview?.decided ?? 0)} sub="significant winner" />
            <Tile label="Exposed" value={(overview?.totalExposed ?? 0).toLocaleString()} sub="visitors bucketed" />
          </div>
          {rows.length === 0 ? (
            <Empty>Nothing running right now.</Empty>
          ) : (
            <div className="divide-y rounded-md border">
              {rows.map((x) => (
                <Link
                  key={x.id}
                  to={`/prism/${x.id}`}
                  className="flex items-center gap-3 px-3 py-2 transition hover:bg-muted/40"
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{x.name}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {x.leader ? `${x.leader} leading` : 'no leader yet'}
                      {x.sampleRatioMismatch ? ' · traffic split looks off' : ''}
                    </span>
                  </span>
                  <span className="shrink-0 text-right text-xs text-muted-foreground">
                    <span className="block tabular-nums">{x.totalExposed.toLocaleString()} exposed</span>
                    {x.winner && <span className="block text-primary">winner: {x.winner}</span>}
                  </span>
                </Link>
              ))}
            </div>
          )}
        </div>
      )}
    </WidgetSection>
  )
}
