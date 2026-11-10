import { useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef, type PaginationState } from '@/components/data-table/tanstack'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { AssistantGapReason, AssistantUnansweredFilter } from '@/graphql/schema.types'
import { DataTable } from '@/components/data-table/DataTable'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { PickStore } from '@/components/layout/panels'
import { useConfigScope } from '@/lib/config-scope'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import { customerLabel, useCustomerNames } from '@/shared/signals/useCustomerNames'
import { AssistantConversationsDocument, type AssistantConversationsQuery } from './Conversations.gql'
import { Outcome } from './Outcome'
import { AuroraStage, Presence } from '@/features/agentic/aurora'
import { DEFAULT_NAME, useAssistantName, useTyping } from '@/features/agentic/useAssistant'

type Row = AssistantConversationsQuery['assistantConversations']['items'][number]
type Counts = AssistantConversationsQuery['assistantConversations']['counts']

const PAGE = 30

// Cursors belong to the store, the filter and the search they came from - any of them changing starts over
type Paging = { key: string; cursors: (string | null)[]; page: number }
const firstPage = (key: string): Paging => ({ key, cursors: [null], page: 0 })
const NO_FILTERS: never[] = []

const REASONS: { reason: AssistantGapReason; label: string; count: keyof Omit<Counts, '__typename'>; badge: 'warning' | 'info'; hint: string }[] = [
  { reason: 'NO_ANSWER', label: 'No answer', count: 'noAnswer', badge: 'warning', hint: 'Nothing said and nothing shown - the storefront apologised' },
  { reason: 'CUT_SHORT', label: 'Cut short', count: 'cutShort', badge: 'warning', hint: 'The turn stopped before it finished' },
  { reason: 'FOUND_NOTHING', label: 'Found nothing', count: 'foundNothing', badge: 'info', hint: 'Every search of the catalog and pages came back empty' },
  { reason: 'UNSURE', label: "Didn't know", count: 'unsure', badge: 'info', hint: "The assistant said the store's information didn't cover it" },
]

const FILTERS: { filter: AssistantUnansweredFilter; param: string; label: string; count: keyof Omit<Counts, '__typename'>; hint: string }[] = [
  { filter: 'ANY', param: 'any', label: 'Unanswered', count: 'any', hint: 'With at least one message the store couldn\'t answer' },
  ...REASONS.map((r) => ({ filter: r.reason, param: r.reason.toLowerCase(), label: r.label, count: r.count, hint: r.hint })),
]

export function ConversationsList() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const active = FILTERS.find((f) => f.param === params.get('unanswered')) ?? null
  const setFilter = (next: (typeof FILTERS)[number] | null) =>
    setParams(
      (prev) => {
        const p = new URLSearchParams(prev)
        if (next) p.set('unanswered', next.param)
        else p.delete('unanswered')
        return p
      },
      { replace: true },
    )
  const [search, setSearch] = useState('')
  const term = useDebounced(search.trim(), 300)
  const key = [effectiveChannelId, active?.filter, term].join('|')
  const [stored, setPaging] = useState<Paging>(() => firstPage(key))
  // Another store, filter or settled search reads from its first page - from the very render it changes, so no stale cursor goes out
  const { cursors, page } = stored.key === key ? stored : firstPage(key)
  const { data, loading, error } = useQuery(AssistantConversationsDocument, {
    variables: { channelId: effectiveChannelId ?? '', search: term || null, unanswered: active?.filter ?? null, after: cursors[page] ?? null, limit: PAGE },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const result = data?.assistantConversations
  const name = useAssistantName(effectiveChannelId)
  const [typing, ping] = useTyping()
  const rows = useMemo(() => result?.items ?? [], [result])
  const nameById = useCustomerNames(useMemo(() => [...new Set(rows.flatMap((r) => (r.customerId ? [r.customerId] : [])))], [rows]))

  const pagination: PaginationState = { pageIndex: page, pageSize: PAGE }
  const onPaginationChange = (next: PaginationState) => {
    const onward = next.pageIndex > page && result?.nextCursor ? [...cursors.slice(0, page + 1), result.nextCursor] : cursors
    setPaging({ key, cursors: onward, page: Math.max(0, next.pageIndex) })
  }

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        id: 'shopper',
        header: 'Shopper',
        enableSorting: false,
        cell: ({ row }) => {
          const r = row.original
          return r.customerId ? (
            <Link to={`/synapse/customers/${r.customerId}`} onClick={(e) => e.stopPropagation()} className="hover:underline">
              {customerLabel(nameById.get(r.customerId), r.customerId)}
            </Link>
          ) : (
            <Link to={`/synapse/visitors/${r.anonId}`} onClick={(e) => e.stopPropagation()} className="text-muted-foreground hover:underline">
              Visitor {r.anonId.slice(0, 8)}
            </Link>
          )
        },
      },
      {
        id: 'question',
        header: 'Started with',
        enableSorting: false,
        cell: ({ row }) => {
          const gap = active ? row.original.latestUnanswered : null
          return (
            <div className="max-w-xl space-y-1">
              <p className="line-clamp-2">{row.original.firstQuestion}</p>
              {gap ? (
                <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                  <span className="line-clamp-1">Couldn't answer: {gap.question}</span>
                  {REASONS.filter((r) => gap.reasons.includes(r.reason)).map((r) => (
                    <Badge key={r.reason} variant={r.badge} title={r.hint}>
                      {r.label}
                    </Badge>
                  ))}
                </p>
              ) : null}
            </div>
          )
        },
      },
      {
        id: 'turns',
        header: 'Messages',
        enableSorting: false,
        cell: ({ row }) => <span className="tabular-nums">{row.original.turns}</span>,
      },
      {
        id: 'outcome',
        header: 'Outcome',
        enableSorting: false,
        cell: ({ row }) => {
          const r = row.original
          if (!r.cartChanged && !r.checkout && !r.incomplete && r.unanswered === 0) return <span className="text-xs text-muted-foreground">-</span>
          return (
            <span className="flex flex-wrap items-center gap-1.5">
              {r.cartChanged || r.checkout || r.incomplete ? <Outcome {...r} /> : null}
              {r.unanswered > 0 ? <Badge variant="warning">{r.unanswered} unanswered</Badge> : null}
            </span>
          )
        },
      },
      {
        id: 'lastAt',
        header: 'Last message',
        enableSorting: false,
        cell: ({ row }) => <span className="whitespace-nowrap text-sm">{formatDateTime(row.original.lastAt)}</span>,
      },
    ],
    [nameById, active],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Assistant conversations"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">What shoppers asked the storefront assistant, and how it answered</span>}
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <PickStore why="Conversations are kept per store" />
          </DetailPageSection>
        ) : (
          <>
          <DetailPageSection>
            <AuroraStage>
              <div className="flex items-start gap-5 p-6 md:p-8">
                <Presence speaking={(loading && !result) || typing} />
                <div className="min-w-0 flex-1 pt-1">
                  <p className="text-balance text-2xl font-semibold tracking-tight md:text-3xl">What shoppers asked {name === DEFAULT_NAME ? 'the shopping assistant' : name}</p>
                  <p className="mt-1 text-muted-foreground">
                    {!result
                      ? 'Reading the conversations…'
                      : result.counts.any > 0
                        ? `${result.counts.any.toLocaleString()} ${result.counts.any === 1 ? 'conversation had a message' : 'conversations had a message'} it couldn't answer - those are what to add to the catalog and the store's pages.`
                        : 'It answered every message in the conversations kept here.'}
                  </p>
                  <div className="mt-5 flex flex-wrap gap-2" role="group" aria-label="Show">
                    <FilterButton active={active === null} onClick={() => setFilter(null)} label="All" />
                    {FILTERS.map((f) => (
                      <FilterButton
                        key={f.filter}
                        active={active?.filter === f.filter}
                        onClick={() => setFilter(active?.filter === f.filter ? null : f)}
                        label={f.label}
                        count={result?.counts[f.count]}
                        hint={f.hint}
                      />
                    ))}
                  </div>
                </div>
              </div>
            </AuroraStage>
          </DetailPageSection>
          <DetailPageSection
            title={active ? `Conversations, ${active.label.toLowerCase()}` : 'Conversations'}
            description="Kept while the store's Keep the shopping assistant's conversations setting is on, for shoppers signed in or who accepted analytics. Open one to see each answer and how the model got there."
          >
            <DataTable
              columns={columns}
              data={rows}
              loading={loading && !result}
              error={error ?? null}
              getRowId={(r) => r.id}
              searchPlaceholder="Search messages and replies…"
              onRowClick={(r) => navigate(`/conversations/${r.id}${active && r.latestUnanswered ? `#turn-${r.latestUnanswered.turnId}` : ''}`)}
              emptyState={
                <p className="py-6 text-center text-sm text-muted-foreground">{active ? 'No conversations like that.' : 'No conversations kept yet.'}</p>
              }
              manual={{
                rowCount: page * PAGE + Math.max(rows.length, page > 0 ? 1 : 0) + (result?.nextCursor ? 1 : 0), // never under the page being loaded, or the table clamps back to the first
                pagination,
                onPaginationChange,
                globalFilter: search,
                onGlobalFilterChange: (next: string) => {
                  setSearch(next)
                  ping()
                },
                columnFilters: NO_FILTERS,
                onColumnFiltersChange: () => {},
              }}
            />
          </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function FilterButton({ active, onClick, label, count, hint }: { active: boolean; onClick: () => void; label: string; count?: number; hint?: string }) {
  return (
    <Button variant="outline" size="sm" onClick={onClick} aria-pressed={active} title={hint} className={cn('gap-2 bg-background/80', active && 'border-foreground bg-foreground text-background hover:bg-foreground/90 hover:text-background')}>
      {label}
      {count !== undefined ? <span className={cn('tabular-nums', active ? 'opacity-70' : 'text-muted-foreground')}>{count.toLocaleString()}</span> : null}
    </Button>
  )
}
