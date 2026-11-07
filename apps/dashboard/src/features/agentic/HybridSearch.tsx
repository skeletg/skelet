import { useState, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { PickStore } from '@/components/layout/panels'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/ui/loading-state'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { SearchByMeaningMode, SearchByMeaningStrictness } from '@/graphql/schema.types'
import { useConfigScope } from '@/lib/config-scope'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { useSaveConfirm } from '@/shared/saving/useSaveConfirm'
import { AssistantOutcome } from './assistantOutcome'
import { AuroraStage, Presence } from './aurora'
import { FieldState, SegmentedRadio, type Field } from './FieldState'
import { CoverageLine, MeaningFailed, MeaningPanel, QueryForm, RankedList, WordsPanel } from './searchParts'
import { OverviewPreview } from './searchOverview'
import { EmbeddingCoverageDocument, SearchPreviewDocument, StorefrontSearchPreviewDocument, type StorefrontSearchPreviewQuery } from './HybridSearch.gql'
import { useConfigGroupFields } from './useConfigGroupFields'
import { useTyping } from './useAssistant'

const SECTION = 'search'
const GROUP = 'products'
const MODE = 'search/products/meaning'
const STRICTNESS = 'search/products/meaning_strictness'
const OVERVIEW = 'search/products/ai_overview'
const PATHS = [MODE, STRICTNESS, OVERVIEW]
const MODES: SearchByMeaningMode[] = ['OFF', 'NATURAL', 'ALWAYS']
const REACHES: SearchByMeaningStrictness[] = ['STRICT', 'BALANCED', 'BROAD']

type Result = StorefrontSearchPreviewQuery['storefrontSearchPreview']

const SAYS: Record<string, string> = {
  natural:
    'A query that reads like a sentence lists what it means, the words helping to order it. Keywords stay on their words, and turn to meaning only when they match nothing.',
  always: 'Every search ranks what its words match together with what is closest in meaning - the most recall, though a keyword can pick up loose matches.',
  off: 'The search page and the assistant list only what the words match.',
  strict: 'Only products nearly as close as the best match - fewer, tighter results.',
  balanced: 'Products reasonably close to the best match.',
  broad: 'Reaches further for related products - more results, some only loosely related.',
}

const OVERVIEW_SAYS: Record<string, string> = {
  off: 'The search page shows its results with nothing written above them.',
  meaning: "When meaning answers a search, a few sentences from the products listed sit above them, with a button into the assistant's chat.",
}

const OVERVIEW_NEEDS = (
  <>
    {' '}
    The shopping assistant writes it in{' '}
    <Link to="/agentic/voice" className="text-foreground underline underline-offset-2">
      its voice
    </Link>
    , so it shows only while the assistant is on under{' '}
    <Link to="/settings/sales/agents" className="text-foreground underline underline-offset-2">
      Settings, Sales, AI shopping agents
    </Link>
    .
  </>
)

const HEADLINE: Record<string, string> = {
  natural: 'Keywords by their words, sentences by their meaning',
  always: 'Every search by words and meaning together',
  off: 'Every search by its words alone',
}

const known = <T extends string>(all: readonly T[], value?: string): T | null => all.find((v) => v === value?.toUpperCase()) ?? null

export function HybridSearch() {
  const registry = useSaveRegistry('Hybrid search')
  const confirm = useSaveConfirm(registry.saveAll)
  const { isChannel, selectedChannel } = useConfigScope()
  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <HybridBody isDirty={registry.isDirty} saving={confirm.saving} onSave={confirm.open} />
        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={isChannel ? (selectedChannel?.name ?? 'This store') : 'Every store without its own'} sections={registry.dirtySections} />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

function HybridBody({ isDirty, saving, onSave }: { isDirty: boolean; saving: boolean; onSave: () => void }) {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const s = useConfigGroupFields(SECTION, GROUP, PATHS)
  const mode = s.fields.get(MODE)
  const strictness = s.fields.get(STRICTNESS)
  const overview = s.fields.get(OVERVIEW)
  const localeLock = s.localeLocked ? `Search isn't set per language - clear the language in the header to change it for ${selectedChannel?.name ?? 'this store'}.` : undefined
  const readOnly = !s.canEdit || !!localeLock
  const store = isChannel ? 'on this store' : 'for every store'
  const restricted = !s.permissionsLoading && !s.canView && !s.noStore
  const note = restricted
    ? `The search below runs with the store's saved settings. Seeing or changing them needs access to settings ${store}.`
    : !s.canEdit && !localeLock
      ? `You can see these, but changing them needs permission to manage settings ${store}.`
      : localeLock

  return (
    <>
      <DetailPageHeader
        title="Hybrid search"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : <Badge variant="outline">Every store</Badge>}
        meta={<span className="text-xs text-muted-foreground">How the store's search page and its assistant mix the words a shopper types with what they mean</span>}
        actions={
          <Button type="button" size="sm" disabled={!isDirty || saving} onClick={onSave}>
            {saving ? 'Saving…' : 'Save changes'}
          </Button>
        }
      />
      <DetailPageBody>
        {s.error ? (
          <DetailPageSection>
            <Alert variant="destructive">
              <AlertTitle>Couldn't load the search settings</AlertTitle>
              <AlertDescription>{s.error.message}</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : s.noStore ? (
          <DetailPageSection>
            <PickStore why="Search is set per store" />
          </DetailPageSection>
        ) : !restricted && (!mode || !strictness) ? (
          <DetailPageSection>
            <LoadingState />
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection>
              <AuroraStage>
                <div className="grid gap-8 p-6 md:p-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:gap-12">
                  <div className="flex items-start gap-5">
                    <Presence />
                    <div className="min-w-0 pt-1">
                      <p className="text-balance text-2xl font-semibold tracking-tight md:text-3xl">
                        {mode ? (HEADLINE[mode.value] ?? mode.data.label) : 'Words and meaning, as the store has them set'}
                      </p>
                      <p className="mt-2 text-sm text-muted-foreground">
                        Words find exactly what a shopper types - a SKU, a brand, "linen shirt". Meaning finds what they're after when they say it in their own words. These settings decide
                        when each one answers on the search page. The assistant always weighs both unless meaning is off, and reaches as far as set here.
                      </p>
                    </div>
                  </div>
                  {mode && strictness ? (
                    <div className="grid gap-6">
                      <Picker id="hybrid-mode" label="Use meaning" field={mode} locked={readOnly} readOnly={readOnly} here={s.here} from={s.from(mode.data)} />
                      <Picker
                        id="hybrid-reach"
                        label="How far meaning reaches"
                        field={strictness}
                        locked={readOnly || mode.value === 'off'}
                        readOnly={readOnly}
                        here={s.here}
                        from={s.from(strictness.data)}
                      />
                      {overview ? (
                        <Picker
                          id="hybrid-overview"
                          label="AI overview"
                          field={overview}
                          says={OVERVIEW_SAYS}
                          needs={OVERVIEW_NEEDS}
                          locked={readOnly || mode.value === 'off'}
                          readOnly={readOnly}
                          here={s.here}
                          from={s.from(overview.data)}
                        />
                      ) : null}
                    </div>
                  ) : null}
                </div>
                {note || s.resetError ? (
                  <div className="border-t bg-background/50 px-6 py-3 text-sm md:px-8">
                    {note ? <p className="text-muted-foreground">{note}</p> : null}
                    {s.resetError ? <p className="text-destructive">{s.resetError}</p> : null}
                  </div>
                ) : null}
              </AuroraStage>
            </DetailPageSection>
            {isChannel && effectiveChannelId ? (
              <TryIt key={effectiveChannelId} channelId={effectiveChannelId} mode={mode?.value} strictness={strictness?.value} overview={overview?.value} />
            ) : (
              <DetailPageSection title="Try a search">
                <PickStore title="Pick a store to try a search" why="The search page and the assistant search one store at a time" />
              </DetailPageSection>
            )}
          </>
        )}
      </DetailPageBody>
    </>
  )
}

function Picker({
  id,
  label,
  field,
  says = SAYS,
  needs,
  locked,
  readOnly,
  here,
  from,
}: {
  id: string
  label: string
  field: Field
  says?: Record<string, string>
  needs?: ReactNode
  locked: boolean
  readOnly: boolean
  here: string
  from: string
}) {
  return (
    <div>
      <p id={id} className="text-sm font-medium">
        {label}
      </p>
      <SegmentedRadio labelledBy={id} options={field.data.options ?? []} value={field.value} onChange={field.set} disabled={locked} className="mt-2" />
      <p className="mt-2 min-h-10 text-sm text-muted-foreground">
        {says[field.value] ?? field.data.description}
        {needs}
      </p>
      <FieldState field={field} here={here} from={from} locked={readOnly} />
    </div>
  )
}

type Tab = 'page' | 'assistant'

// Both run the same two searches - they differ in how they put them together, and why
const PAGE_READS: Record<string, string> = {
  natural:
    "What a shopper's search page lists. A sentence lists the products closest in meaning, the words only helping to order them; keywords stay on their words, and meaning steps in only when they match nothing. A shopper scans a grid, so it keeps to close matches.",
  always:
    "What a shopper's search page lists. Every search ranks what the words match together with what's closest in meaning - more found, though a keyword can pick up loose matches.",
  off: "What a shopper's search page lists - only what the words match.",
}

const ASSISTANT_READS =
  "What the assistant gets back when it searches. Words and meaning are always ranked together, as equals, on one short page, and the store's pages are searched too. The model reads every result and drops the weak ones, so it's given more to choose from than a shopper's page would show. Only Off and the reach change it."

// The choices above as they stand, saved or not - without them the store's saved settings answer. A tab runs its search
// once it's been opened, and from then on with every search and every change
function TryIt({ channelId, mode, strictness, overview }: { channelId: string; mode?: string; strictness?: string; overview?: string }) {
  const { effectiveLocale } = useConfigScope()
  const [params, setParams] = useSearchParams()
  const tab: Tab = params.get('tab') === 'assistant' ? 'assistant' : 'page'
  const [opened, setOpened] = useState<Record<Tab, boolean>>({ page: tab === 'page', assistant: tab === 'assistant' })
  const [query, setQuery] = useState('')
  const [locale, setLocale] = useState(effectiveLocale)
  const [asked, setAsked] = useState('')
  const coverage = useQuery(EmbeddingCoverageDocument, { variables: { channelId }, fetchPolicy: 'cache-and-network' })
  const variables = { channelId, query: asked, locale: locale || null, mode: known(MODES, mode), strictness: known(REACHES, strictness) }
  const page = useQuery(StorefrontSearchPreviewDocument, { variables, skip: !asked || !opened.page, fetchPolicy: 'network-only' })
  const assistant = useQuery(SearchPreviewDocument, { variables, skip: !asked || !opened.assistant, fetchPolicy: 'network-only' })
  const pageShown = page.data ?? page.previousData
  const assistantShown = assistant.data ?? assistant.previousData
  const [typing, ping] = useTyping()

  const show = (next: string) => {
    const t: Tab = next === 'assistant' ? 'assistant' : 'page'
    setOpened((o) => ({ ...o, [t]: true }))
    setParams(
      (prev) => {
        const p = new URLSearchParams(prev)
        if (t === 'page') p.delete('tab')
        else p.set('tab', t)
        return p
      },
      { replace: true },
    )
  }
  const submit = () => {
    const next = query.trim()
    if (next !== asked) {
      setAsked(next)
      return
    }
    if (opened.page) page.refetch().catch(() => {})
    if (opened.assistant) assistant.refetch().catch(() => {})
  }

  return (
    <Tabs value={tab} onValueChange={show} className="divide-y">
      <DetailPageSection title="Try a search" description="Runs the store's search page and the assistant's searches with the choices above, saved or not - change one and the results follow.">
        <QueryForm
          value={query}
          onChange={(v) => {
            setQuery(v)
            ping()
          }}
          locale={locale}
          onLocale={setLocale}
          onSubmit={submit}
          busy={page.loading || assistant.loading}
          label="What a shopper types"
          placeholder="something warm for winter hikes"
        >
          <Presence size="sm" speaking={page.loading || assistant.loading || typing} />
        </QueryForm>
        <div className="mt-3 text-sm">
          {coverage.error ? (
            <span className="text-destructive">Couldn't load how much is embedded: {coverage.error.message}</span>
          ) : coverage.data ? (
            <CoverageLine c={coverage.data.embeddingCoverage} />
          ) : null}
        </div>
        <TabsList className="mt-5 w-fit">
          <TabsTrigger value="page">Search page</TabsTrigger>
          <TabsTrigger value="assistant">Assistant</TabsTrigger>
        </TabsList>
        <p className="mt-3 max-w-3xl text-sm text-muted-foreground">{tab === 'page' ? PAGE_READS[mode ?? ''] ?? PAGE_READS.natural : ASSISTANT_READS}</p>
      </DetailPageSection>
      <TabsContent value="page" className="mt-0 divide-y empty:hidden">
        {page.error ? (
          <Failed message={page.error.message} />
        ) : pageShown ? (
          <Outcome r={pageShown.storefrontSearchPreview} overview={<OverviewPreview variables={variables} setting={overview} />} />
        ) : null}
      </TabsContent>
      <TabsContent value="assistant" className="mt-0 divide-y empty:hidden">
        {assistant.error ? (
          <Failed message={assistant.error.message} />
        ) : assistantShown ? (
          <AssistantOutcome
            r={assistantShown.searchPreview}
            unavailable={!coverage.data?.embeddingCoverage.model ? 'Search by meaning is off.' : mode === 'off' ? 'Not asked - search by meaning is off.' : undefined}
          />
        ) : null}
      </TabsContent>
    </Tabs>
  )
}

function Failed({ message }: { message: string }) {
  return (
    <DetailPageSection>
      <Alert variant="destructive">
        <AlertTitle>The search failed</AlertTitle>
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    </DetailPageSection>
  )
}

function summary(r: Result): string {
  const n = `${r.total.toLocaleString()} product${r.total === 1 ? '' : 's'}`
  if (r.total === 0) return 'Nothing found - the shopper sees an empty search page.'
  switch (`${r.reads}/${r.answeredBy}`) {
    case 'SENTENCE/MEANING':
      return `Read as a sentence, so it lists the ${n} closest in meaning, the words helping to order them.`
    case 'KEYWORDS/MEANING':
      return `Read as keywords, but nothing matches them - so it lists the ${n} closest in meaning instead.`
    case 'BLENDED/BLENDED':
      return `Every search blends both, so it lists ${n} the words match or that are close in meaning, ranked together.`
    case 'KEYWORDS/WORDS':
      return `Read as keywords, and they match ${n} - so meaning isn't asked.`
    case 'WORDS/WORDS':
      return `Search by meaning is off, so it lists the ${n} the words match.`
  }
  return r.meaningFailed ? `Meaning didn't answer, so it lists the ${n} the words match.` : `It lists the ${n} the words match - nothing is close enough in meaning.`
}

function Outcome({ r, overview }: { r: Result; overview: ReactNode }) {
  const asked = r.byMeaning.length > 0 || !!r.meaningFailed
  return (
    <>
      <MeaningFailed message={r.meaningFailed} answers="the search page" />
      {r.answeredBy === 'MEANING' ? overview : null}
      <DetailPageSection title="What the shopper sees" description={summary(r)}>
        <RankedList rows={r.listed} empty="No products." />
      </DetailPageSection>
      <DetailPageSection title="How it got there" description="Each search on its own, as this query ran them.">
        <div className="grid gap-4 lg:grid-cols-2">
          <WordsPanel total={r.wordsTotal} products={r.byWords} />
          <MeaningPanel
            title={asked ? `By meaning - counts within ${r.margin.toFixed(2)} of the closest` : 'By meaning'}
            near={r.byMeaning}
            unavailable={
              r.meaningFailed ? 'Search by meaning failed.' : !asked ? `Not asked - ${r.reads === 'WORDS' ? 'search by meaning is off.' : 'keywords that match stay on their words.'}` : undefined
            }
            empty="Nothing is close enough in meaning."
          />
        </div>
      </DetailPageSection>
    </>
  )
}
