import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { BrainIcon, CheckIcon, ChevronRightIcon, CircleDashedIcon, EyeIcon, EyeOffIcon, ListTreeIcon, XIcon } from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { AssistantTurnRoundsDocument, type AssistantConversationQuery, type AssistantTurnRoundsQuery } from './Conversations.gql'
import { argChips, firstLine, toolLabel, wordsOf } from './transcript'
import { ReplyText } from './ReplyText'

type Turn = NonNullable<AssistantConversationQuery['assistantConversation']>['turns'][number]
type Round = AssistantTurnRoundsQuery['assistantTurnRounds'][number]
type Call = Round['calls'][number]

function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function CallRow({ call }: { call: Call }) {
  const chips = argChips(call.input)
  return (
    <Collapsible>
      <CollapsibleTrigger className="group flex w-full items-start gap-2.5 px-3 py-2 text-left transition hover:bg-muted/40">
        {call.skipped ? (
          <CircleDashedIcon className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" aria-label="Never ran" />
        ) : call.isError ? (
          <XIcon className="mt-0.5 size-3.5 shrink-0 text-destructive" aria-label="Refused" />
        ) : (
          <CheckIcon className="mt-0.5 size-3.5 shrink-0 text-success" aria-label="Answered" />
        )}
        <span className="min-w-0 flex-1 space-y-1">
          <span className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-sm font-medium">{toolLabel(call.tool)}</span>
            <code className="text-xs text-muted-foreground">{call.tool}</code>
          </span>
          {chips.length > 0 ? (
            <span className="flex flex-wrap gap-1">
              {chips.map((c) => (
                <span key={c} className="rounded border bg-background px-1.5 py-px font-mono text-[11px] text-muted-foreground">
                  {c}
                </span>
              ))}
            </span>
          ) : null}
          <span className={cn('block truncate text-xs', call.isError ? 'text-destructive' : 'text-muted-foreground')}>
            {call.skipped ? 'Never ran - the turn had used its last step' : `→ ${firstLine(call.output) || 'nothing'}`}
          </span>
        </span>
        <ChevronRightIcon className="mt-0.5 size-3.5 shrink-0 text-muted-foreground transition group-data-[state=open]:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent className="grid gap-3 border-t bg-muted/20 px-3 py-3 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className="min-w-0">
          <p className="mb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Arguments</p>
          <pre className="max-h-80 overflow-auto rounded-md border bg-background p-2.5 text-xs">{prettyJSON(call.input) || '{}'}</pre>
        </div>
        <div className="min-w-0">
          <p className="mb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">What the store answered</p>
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-background p-2.5 text-xs">{call.output || '(nothing)'}</pre>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function RoundWords({ round }: { round: Round }) {
  const words = wordsOf(round.text)
  if (!words) return null
  return round.shown ? (
    <div className="rounded-md border border-success/30 bg-success-soft px-3 py-2">
      <p className="mb-1 flex items-center gap-1.5 text-xs font-medium text-success">
        <EyeIcon className="size-3.5" /> Sent to the shopper
      </p>
      <ReplyText text={words} className="text-sm" />
    </div>
  ) : (
    <div className="rounded-md bg-muted/40 px-3 py-2">
      <p className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
        <EyeOffIcon className="size-3.5" /> Written along the way, not shown to the shopper
      </p>
      <p className="whitespace-pre-wrap font-mono text-xs text-muted-foreground">{words}</p>
    </div>
  )
}

// The rounds are read when the trace opens - a conversation's turns come without them, so a long one still loads
export function ConversationTrace({ turn, channelId }: { turn: Turn; channelId: string }) {
  const [open, setOpen] = useState(turn.refused || turn.incomplete)
  const { data, loading, error } = useQuery(AssistantTurnRoundsDocument, { variables: { channelId, turnId: turn.id }, skip: !open })
  const rounds = data?.assistantTurnRounds ?? []
  const last = rounds.length - 1
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-md border">
      <CollapsibleTrigger className="group flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition hover:bg-muted/40">
        <ListTreeIcon className="size-4 text-muted-foreground" />
        <span className="font-medium">How it got there</span>
        <span className="text-xs text-muted-foreground">
          {turn.steps} {turn.steps === 1 ? 'step' : 'steps'} · {turn.toolCalls} {turn.toolCalls === 1 ? 'tool call' : 'tool calls'}
          {turn.thought ? ' · with thinking' : ''}
        </span>
        <ChevronRightIcon className="ml-auto size-4 text-muted-foreground transition group-data-[state=open]:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t px-3 py-4">
        {error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : loading && rounds.length === 0 ? (
          <p className="text-sm text-muted-foreground">Loading the steps…</p>
        ) : (
          <ol className="relative ml-2.5 space-y-5 border-l pl-6">
            {rounds.map((r, i) => (
              <li key={i} className="relative space-y-2">
                <span className="absolute -left-[35px] top-0 flex size-5 items-center justify-center rounded-full border bg-background text-[10px] font-medium tabular-nums text-muted-foreground">
                  {i + 1}
                </span>
                <p className="text-xs font-medium text-muted-foreground">{i === last ? 'Answered' : `Step ${i + 1}`}</p>
                {r.thinking.trim() ? (
                  <div className="flex gap-2 rounded-md bg-info-soft px-3 py-2">
                    <BrainIcon className="mt-0.5 size-3.5 shrink-0 text-info" aria-label="Thinking" />
                    <ReplyText text={r.thinking} className="min-w-0 text-sm text-muted-foreground" />
                  </div>
                ) : null}
                <RoundWords round={r} />
                {r.calls.length > 0 ? (
                  <div className="divide-y rounded-md border">
                    {r.calls.map((c, j) => (
                      <CallRow key={j} call={c} />
                    ))}
                  </div>
                ) : null}
              </li>
            ))}
          </ol>
        )}
        <p className="mt-4 text-xs text-muted-foreground">
          {turn.model || 'Unknown model'} · {turn.inputTokens.toLocaleString()} in · {turn.outputTokens.toLocaleString()} out ·{' '}
          {turn.cacheReadTokens.toLocaleString()} read from cache · {turn.cacheWriteTokens.toLocaleString()} written to cache
          {turn.thought ? '' : ' · no thinking summaries from this model'}
        </p>
      </CollapsibleContent>
    </Collapsible>
  )
}
