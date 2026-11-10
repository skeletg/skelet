import { useEffect, type ReactNode } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { AlertTriangleIcon, ArrowRightIcon, ChevronDownIcon, FilterIcon, PackageIcon, PanelRightIcon, ShoppingCartIcon } from 'lucide-react'
import { Presence } from '@/features/agentic/aurora'
import { DEFAULT_NAME, useAssistantName } from '@/features/agentic/useAssistant'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { formatDateTime } from '@/lib/format'
import { customerLabel, useCustomerNames } from '@/shared/signals/useCustomerNames'
import { AssistantConversationDocument, type AssistantConversationQuery } from './Conversations.gql'
import { ConversationTrace } from './ConversationTrace'
import { Outcome } from './Outcome'
import { ReplyText } from './ReplyText'
import { shownFrom, unanswered, type Shown } from './transcript'

type Detail = NonNullable<AssistantConversationQuery['assistantConversation']>
type Turn = Detail['turns'][number]

const chip = 'inline-flex max-w-full items-center gap-1.5 rounded-md border bg-background px-2 py-1 text-xs'

function ShownChip({ item }: { item: Shown }) {
  switch (item.kind) {
    case 'card':
    case 'panel':
      return (
        <Link to={`/products/${item.id}`} className={`${chip} hover:bg-muted`} title={item.kind === 'panel' ? 'Explained in a panel' : 'Shown as a card'}>
          {item.kind === 'panel' ? <PanelRightIcon className="size-3.5 shrink-0 text-muted-foreground" /> : <PackageIcon className="size-3.5 shrink-0 text-muted-foreground" />}
          <span className="truncate">{item.name}</span>
        </Link>
      )
    case 'button':
      return (
        <span className={chip} title={item.path}>
          <ArrowRightIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate">{item.label}</span>
          <code className="truncate text-muted-foreground">{item.path}</code>
        </span>
      )
    case 'moved':
      return (
        <span className={chip}>
          <FilterIcon className="size-3.5 shrink-0 text-muted-foreground" />
          Refiltered the list to <code className="truncate text-muted-foreground">{item.path}</code>
        </span>
      )
    case 'cart':
      return (
        <span className={chip}>
          <ShoppingCartIcon className="size-3.5 shrink-0 text-muted-foreground" />
          Review cart button
        </span>
      )
    default:
      return <span className="text-xs text-muted-foreground">{item.text}</span>
  }
}

function TurnView({ turn, channelId, focused, name }: { turn: Turn; channelId: string; focused: boolean; name: string }) {
  const shown = shownFrom(turn.showed)
  const next = turn.followUps
  const lost = unanswered(turn)
  return (
    <article id={`turn-${turn.id}`} className={`scroll-mt-6 space-y-3 py-6 ${focused ? '-mx-3 rounded-md px-3 ring-2 ring-warning/50' : ''}`}>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <span>{formatDateTime(turn.at)}</span>
        {turn.page ? (
          <>
            <span>·</span>
            <span>
              on <code>{turn.page}</code>
            </span>
          </>
        ) : null}
        <span className="ml-auto">
          <Outcome {...turn} />
        </span>
      </div>
      <div className="flex justify-end">
        <p className="max-w-[80%] whitespace-pre-wrap rounded-2xl rounded-br-sm bg-muted px-4 py-2.5 text-sm">{turn.question}</p>
      </div>
      <div className="flex gap-3">
        <Presence size="sm" still className="mt-0.5" />
        <div className="min-w-0 flex-1 space-y-3">
          <p className="-mb-1 text-xs font-medium text-muted-foreground">{name}</p>
          {turn.reply ? (
            <ReplyText text={turn.reply} className="text-sm leading-relaxed" />
          ) : lost ? (
            <p className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning-soft px-3 py-2 text-xs text-warning">
              <AlertTriangleIcon className="mt-px size-3.5 shrink-0" />
              The shopper got no answer here - nothing said and nothing shown, so the storefront apologised instead. The steps below show why.
            </p>
          ) : turn.incomplete ? null : (
            <p className="text-sm italic text-muted-foreground">No words in this reply - only what it showed below.</p>
          )}
          {shown.length > 0 ? (
            <div className="space-y-1.5">
              <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Showed</p>
              <div className="flex flex-wrap gap-1.5">
                {shown.map((s, i) => (
                  <ShownChip key={i} item={s} />
                ))}
              </div>
            </div>
          ) : null}
          {next.length > 0 ? (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Suggested</span>
              {next.map((s) => (
                <span key={s} className="rounded-full border px-2.5 py-0.5 text-xs text-muted-foreground">
                  {s}
                </span>
              ))}
            </div>
          ) : null}
          <ConversationTrace turn={turn} channelId={channelId} />
        </div>
      </div>
    </article>
  )
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 px-4 py-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-sm">{children}</dd>
    </div>
  )
}

function SummaryFacts({ detail, shopper }: { detail: Detail; shopper: ReactNode }) {
  const c = detail.conversation
  const sum = (f: (t: Turn) => number) => detail.turns.reduce((n, t) => n + f(t), 0)
  const models = [...new Set(detail.turns.map((t) => t.model).filter(Boolean))]
  const pages = [...new Set(detail.turns.map((t) => t.page).filter(Boolean))]
  const thought = detail.turns.some((t) => t.thought)
  const lost = detail.turns.filter(unanswered).length
  return (
    <dl className="divide-y">
      <Fact label="Shopper">{shopper}</Fact>
      <Fact label="Started">{formatDateTime(c.startedAt)}</Fact>
      <Fact label="Last message">{formatDateTime(c.lastAt)}</Fact>
      <Fact label="Messages">{c.turns}</Fact>
      {c.cartChanged || c.checkout || c.incomplete ? (
        <Fact label="Outcome">
          <Outcome {...c} />
        </Fact>
      ) : null}
      {lost > 0 ? (
        <Fact label="Needs a look">
          <span className="text-warning">
            {lost} {lost === 1 ? 'message' : 'messages'} the shopper got no answer to
          </span>
        </Fact>
      ) : null}
      <Fact label="Model">{models.join(', ') || '-'}</Fact>
      <Fact label="Tokens">
        <span className="tabular-nums">
          {sum((t) => t.inputTokens).toLocaleString()} in · {sum((t) => t.outputTokens).toLocaleString()} out
        </span>
        <span className="block text-xs text-muted-foreground tabular-nums">
          {sum((t) => t.cacheReadTokens).toLocaleString()} read from cache · {sum((t) => t.cacheWriteTokens).toLocaleString()} written
        </span>
      </Fact>
      <Fact label="Thinking">{thought ? 'Summaries kept' : <span className="text-muted-foreground">None returned by this model</span>}</Fact>
      {pages.length > 0 ? (
        <Fact label="Pages">
          <span className="flex flex-col gap-1">
            {pages.map((p) => (
              <code key={p} className="truncate text-xs">
                {p}
              </code>
            ))}
          </span>
        </Fact>
      ) : null}
    </dl>
  )
}

function Summary({ detail, shopper }: { detail: Detail; shopper: ReactNode }) {
  return (
    <aside className="scrollbar-accent hidden h-full w-[22rem] shrink-0 flex-col overflow-y-auto border-l bg-muted/10 xl:flex 2xl:w-[24rem]">
      <div className="border-b bg-background px-4 py-3">
        <div className="text-sm font-semibold">Summary</div>
      </div>
      <SummaryFacts detail={detail} shopper={shopper} />
    </aside>
  )
}

// Below the width the sidebar needs, the same facts fold above the thread
function InlineSummary({ detail, shopper }: { detail: Detail; shopper: ReactNode }) {
  return (
    <Collapsible className="mx-auto mb-4 w-full max-w-3xl rounded-md border bg-background xl:hidden">
      <CollapsibleTrigger className="group flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold">
        Summary
        <ChevronDownIcon className="size-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t">
        <SummaryFacts detail={detail} shopper={shopper} />
      </CollapsibleContent>
    </Collapsible>
  )
}

export function ConversationDetail() {
  const { id = '' } = useParams()
  const { effectiveChannelId, selectedChannel } = useConfigScope()
  const { data, loading, error } = useQuery(AssistantConversationDocument, {
    variables: { channelId: effectiveChannelId ?? '', id },
    skip: !effectiveChannelId || !id,
  })
  const detail = data?.assistantConversation
  const c = detail?.conversation
  // Opened at one message - from the list filtered to unanswered - it scrolls to that message and marks it
  const { hash } = useLocation()
  const focus = hash.startsWith('#turn-') ? hash.slice('#turn-'.length) : null
  useEffect(() => {
    if (focus && detail) document.getElementById(`turn-${focus}`)?.scrollIntoView?.({ block: 'start' })
  }, [focus, detail])
  const names = useCustomerNames(c?.customerId ? [c.customerId] : [])
  const assistant = useAssistantName(effectiveChannelId)
  const shopper = !c ? null : c.customerId ? (
    <Link to={`/synapse/customers/${c.customerId}`} className="hover:underline">
      {customerLabel(names.get(c.customerId), c.customerId)}
    </Link>
  ) : (
    <Link to={`/synapse/visitors/${c.anonId}`} className="hover:underline">
      Guest visitor {c.anonId.slice(0, 8)}
    </Link>
  )

  return (
    <DetailPageLayout rightSidebar={detail ? <Summary detail={detail} shopper={shopper} /> : undefined}>
      <DetailPageHeader
        backTo="/conversations"
        backLabel="Back to conversations"
        title={<span className="line-clamp-1">{c?.firstQuestion ?? 'Conversation'}</span>}
        badges={
          <>
            {selectedChannel ? <Badge variant="secondary">{selectedChannel.name}</Badge> : null}
            {c ? <Outcome {...c} /> : null}
          </>
        }
        meta={
          c ? (
            <>
              <span className="text-foreground">{shopper}</span>
              <span>·</span>
              <span>{formatDateTime(c.startedAt)}</span>
              <span>·</span>
              <span>
                {c.turns} {c.turns === 1 ? 'message' : 'messages'}
              </span>
            </>
          ) : undefined
        }
      />
      <DetailPageBody className="min-h-full shrink-0 bg-muted">
        <DetailPageSection className="px-4 sm:px-10 lg:px-14">
          {error ? (
            <p className="text-sm text-destructive">{error.message}</p>
          ) : loading && !detail ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : !detail ? (
            <p className="text-sm text-muted-foreground">This store holds no such conversation - it may have aged out, or belong to another store.</p>
          ) : (
            <>
              <InlineSummary detail={detail} shopper={shopper} />
              {detail.turns.length < detail.conversation.turns ? (
                <p className="mx-auto mb-3 w-full max-w-3xl text-xs text-muted-foreground">
                  Showing the latest {detail.turns.length} of {detail.conversation.turns} messages.
                </p>
              ) : null}
              <div className="mx-auto w-full max-w-3xl divide-y rounded-sm border bg-background px-5 py-2 shadow-md sm:px-10 sm:py-6">
                {detail.turns.map((t) => (
                  <TurnView key={t.id} turn={t} channelId={effectiveChannelId ?? ''} focused={t.id === focus} name={assistant === DEFAULT_NAME ? 'Shopping assistant' : assistant} />
                ))}
              </div>
            </>
          )}
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}
