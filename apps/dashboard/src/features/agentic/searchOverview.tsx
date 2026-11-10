import { Fragment, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { SparklesIcon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { usePermissions } from '@/shared/auth/usePermissions'
import { AuroraStage, Presence } from './aurora'
import { SearchOverviewPreviewDocument, type SearchOverviewPreviewQuery, type SearchOverviewPreviewQueryVariables } from './HybridSearch.gql'
import { useStreamed } from './useAssistant'

type Product = SearchOverviewPreviewQuery['searchOverviewPreview']['products'][number]

const OVERVIEW_ON = 'meaning'
const MAX_QUERY = 200

const linkClass = 'text-foreground underline underline-offset-2'

function Plain({ children }: { children: ReactNode }) {
  return <p className="text-sm text-muted-foreground">{children}</p>
}

const SAID: Record<string, ReactNode> = {
  KEYWORDS: 'The words answered this search, so the search page writes nothing above it.',
  NOTHING: "Nothing written for these results - the model had nothing to say about them, or the store's AI budget for today is spent.",
  UNAVAILABLE: (
    <>
      Nothing's written while the shopping assistant isn't answering on this store - it's the one that writes it. Switch it on under{' '}
      <Link to="/settings/sales/agents" className={linkClass}>
        Settings, Sales, AI shopping agents
      </Link>
      , with an AI provider that calls tools under{' '}
      <Link to="/settings/ai/llm" className={linkClass}>
        Settings, AI, LLM provider
      </Link>
      .
    </>
  ),
}

// [[n]] is the nth product, shown as its name linking to its page - a number past the list drops out
function Linked({ text, products }: { text: string; products: Product[] }) {
  return text.split(/\[\[(\d+)\]\]/).map((part, i) => {
    if (i % 2 === 0) return <Fragment key={i}>{part}</Fragment>
    const p = products[Number(part) - 1]
    return p ? (
      <a key={i} href={p.url} target="_blank" rel="noreferrer" className="font-medium underline decoration-foreground/30 underline-offset-2 hover:decoration-foreground">
        {p.name}
      </a>
    ) : null
  })
}

// Writes on its own while the setting stands at on, saved or not; off, it waits to be asked for this search, since each one spends the AI budget
export function OverviewPreview({ variables, setting }: { variables: SearchOverviewPreviewQueryVariables; setting?: string }) {
  const on = setting === OVERVIEW_ON
  const key = JSON.stringify(variables)
  const [asked, setAsked] = useState('')
  const { canAccessChannelScope, loading } = usePermissions()
  // the backend refuses anyone else, since a preview spends the store's AI allowance
  const allowed = canAccessChannelScope('settings.manage', String(variables.channelId))
  const long = [...variables.query].length > MAX_QUERY
  const run = allowed && !long && (on || asked === key)
  const q = useQuery(SearchOverviewPreviewDocument, { variables, skip: !run, fetchPolicy: 'network-only' })
  const shown = run && !q.loading ? q.data?.searchOverviewPreview : undefined
  const written = shown?.status === 'WRITTEN' ? shown.text : ''
  const line = useStreamed(written, written)

  const description = on
    ? "Written above these results while it's on - a few sentences from the products listed, in the assistant's voice."
    : setting === undefined
      ? "What the search page would write above these results where it's on. Writing one spends the store's AI budget."
      : 'Off for this store, so the search page writes nothing above these results.'

  return (
    <DetailPageSection title="AI overview" description={description}>
      {!allowed ? (
        loading ? null : <Plain>Previewing an overview needs permission to manage settings on this store, since writing one spends its AI budget.</Plain>
      ) : long ? (
        <Plain>Searches longer than {MAX_QUERY} characters get no overview.</Plain>
      ) : !run ? (
        <div className="flex flex-wrap items-center gap-3 rounded-md border px-4 py-3">
          <p className="min-w-60 flex-1 text-sm text-muted-foreground">See what it would write for this search. Writing one spends the store's AI budget, the way a shopper's search would.</p>
          <Button type="button" variant="outline" size="sm" onClick={() => setAsked(key)}>
            <SparklesIcon className="size-4" />
            Preview the AI overview
          </Button>
        </div>
      ) : q.error ? (
        <Alert variant="destructive">
          <AlertTitle>Couldn't write the overview</AlertTitle>
          <AlertDescription>{q.error.message}</AlertDescription>
        </Alert>
      ) : (
        <AuroraStage>
          <div className="flex items-start gap-4 px-5 py-4" aria-live="polite" aria-busy={!shown || line.streaming}>
            <Presence size="sm" speaking={!shown || line.streaming} className="mt-0.5" />
            <div className="min-w-0 flex-1">
              {!shown ? (
                <Plain>Writing the overview…</Plain>
              ) : shown.status === 'WRITTEN' ? (
                <>
                  <p className="max-w-3xl text-base leading-relaxed">
                    <Linked text={line.text} products={shown.products} />
                    {line.streaming ? <span className="aurora-caret" aria-hidden /> : null}
                  </p>
                  <p className="mt-3 text-xs text-muted-foreground">Below it, shoppers get a button to carry on in the assistant's chat.</p>
                </>
              ) : (
                <Plain>{SAID[shown.status]}</Plain>
              )}
            </div>
          </div>
        </AuroraStage>
      )}
    </DetailPageSection>
  )
}
