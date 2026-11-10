import { ChevronDownIcon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Empty, Panel } from '@/components/layout/panels'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Distance, MeaningFailed, MeaningPanel, RankedList, WordsPanel } from './searchParts'
import type { SearchPreviewQuery } from './HybridSearch.gql'

type Result = SearchPreviewQuery['searchPreview']

export function AssistantOutcome({ r, unavailable }: { r: Result; unavailable?: string }) {
  const languages = r.locales.join(', ')
  const missing = r.meaningFailed ? 'Search by meaning failed.' : unavailable
  return (
    <>
      <MeaningFailed message={r.meaningFailed} answers="the assistant" />
      <DetailPageSection
        title="What the assistant is shown"
        description={`search_products' answer, in its order${languages ? `, searching in ${languages}` : ''}. Both lists are ranked together - high in either comes high, found both ways higher still.`}
      >
        <RankedList rows={r.shown} empty="Nothing found - the assistant would tell the shopper the store doesn't seem to have it." />
      </DetailPageSection>
      <DetailPageSection title="How it got there" description="Each search on its own. A product by meaning counts when it's about as close as the nearest one - the rest are too far to trust.">
        <div className="grid gap-4 lg:grid-cols-2">
          <WordsPanel total={r.wordsTotal} products={r.byWords} />
          <MeaningPanel near={r.byMeaning} unavailable={missing} empty="Nothing embedded is near this question yet." />
          <Panel title="Pages by words">
            {r.pagesByWords.length === 0 ? (
              <Empty>No page mentions these words.</Empty>
            ) : (
              <ul className="divide-y">
                {r.pagesByWords.map((p) => (
                  <li key={p.url} className="py-2">
                    <PageLine title={p.title} url={p.url} />
                  </li>
                ))}
              </ul>
            )}
          </Panel>
          <Panel title="Passages by meaning">
            {missing ? (
              <Empty>{missing}</Empty>
            ) : r.passages.length === 0 ? (
              <Empty>No embedded passage is near this question yet.</Empty>
            ) : (
              <ul className="divide-y">
                {r.passages.map((p, i) => (
                  <li key={`${p.url}-${i}`} className="space-y-1 py-2">
                    <div className="flex items-center gap-3">
                      <PageLine title={p.title} url={p.url} />
                      <Distance d={p.distance} counted={p.counted} />
                    </div>
                    <p className="line-clamp-3 text-xs text-muted-foreground">{p.text}</p>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>
      </DetailPageSection>
      <DetailPageSection title="Word for word" description="Exactly what the two tools hand the model.">
        <div className="space-y-2">
          <Verbatim title="search_products" text={r.productsText} />
          <Verbatim title="search_store_pages" text={r.pagesText} />
        </div>
      </DetailPageSection>
    </>
  )
}

function PageLine({ title, url }: { title: string; url: string }) {
  return (
    <a href={url} target="_blank" rel="noreferrer" className="min-w-0 flex-1 truncate text-sm hover:underline">
      {title || url}
    </a>
  )
}

function Verbatim({ title, text }: { title: string; text: string }) {
  return (
    <Collapsible className="rounded-md border">
      <CollapsibleTrigger className="group flex w-full items-center justify-between px-3 py-2 text-left text-sm">
        <code className="text-xs">{title}</code>
        <ChevronDownIcon className="size-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t p-2">
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/40 p-2.5 text-xs">{text || '(nothing)'}</pre>
      </CollapsibleContent>
    </Collapsible>
  )
}
