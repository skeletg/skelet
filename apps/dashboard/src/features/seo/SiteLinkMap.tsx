import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ExternalLinkIcon,
  EyeOffIcon,
  LayersIcon,
  NetworkIcon,
  RadarIcon,
  TriangleAlertIcon,
  UnlinkIcon,
  UnplugIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import {
  CancelSiteCrawlDocument,
  LinkMapDocument,
  SiteCrawlsDocument,
  StartSiteCrawlDocument,
} from './LinkMap.gql'
import { CrawlAuditTable } from './CrawlAuditTable'
import { CrawlDiffPanel } from './CrawlDiffPanel'
import { LinkMapCanvas, type ColorMode, type MapEdge, type MapNode } from './LinkMapCanvas'
import { HOMEPAGE_COLOR, NODE_COLORS, NODE_LABELS, type MapNodeType } from './link-map-theme'
import { cn } from '@/lib/utils'
import { useRefetchWhenShown, useVisiblePollInterval } from '@/lib/visible-poll'

const DETAIL_ROUTES: Record<MapNodeType, string | null> = {
  PRODUCT: '/products',
  CATEGORY: '/categories',
  COLLECTION: '/collections',
  CMS_PAGE: '/cms/pages',
  BLOG_POST: '/blog/posts',
  BLOG_CATEGORY: '/blog/categories',
  BLOG_TAG: '/blog/tags',
  BLOG_AUTHOR: '/blog/authors',
  OTHER: null,
}

const COLOR_MODES: Array<{ value: ColorMode; label: string; hint: string }> = [
  { value: 'health', label: 'Link health', hint: 'Broken, orphaned and redirecting pages first' },
  { value: 'type', label: 'Page type', hint: 'What each page is' },
  { value: 'depth', label: 'Click depth', hint: 'Green is shallow, red is buried' },
]

const BUDGETS = [400, 1200, 3000, 6000]

function Stat({ label, value, tone, icon: Icon }: { label: string; value: string; tone?: string; icon?: typeof NetworkIcon }) {
  return (
    <div className="flex items-center gap-3 rounded-md border p-3">
      {Icon ? <Icon className={`size-4 shrink-0 ${tone ?? 'text-muted-foreground'}`} /> : null}
      <div className="min-w-0">
        <div className="text-lg leading-tight font-semibold tabular-nums">{value}</div>
        <div className="truncate text-xs text-muted-foreground">{label}</div>
      </div>
    </div>
  )
}

export function SiteLinkMap() {
  const { effectiveChannelId, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [colorMode, setColorMode] = useState<ColorMode>('health')
  const [budget, setBudget] = useState(1200)
  const [search, setSearch] = useState('')
  const [hidden, setHidden] = useState<Set<MapNodeType>>(new Set())
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [showChrome, setShowChrome] = useState(false)
  const [baseUrl, setBaseUrl] = useState('')
  const [maxPages, setMaxPages] = useState('2000')

  // While a crawl runs the numbers only exist in its progress row, so it polls - pollInterval rather than a hand-rolled
  // timer, which restarted on every render (its deps were the query results). The interval reads the previous render's
  // crawl state, adjusted during render like JobHistory, because the query can't see its own result before it runs
  const [crawling, setCrawling] = useState(false)
  const crawlsQuery = useQuery(SiteCrawlsDocument, {
    variables: { channelId, limit: 5 },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
    pollInterval: useVisiblePollInterval(crawling ? 3000 : 0),
  })
  useRefetchWhenShown(crawlsQuery.refetch)
  const crawls = crawlsQuery.data?.siteCrawls ?? []
  const active = crawls.find((c) => c.status === 'RUNNING') ?? null
  const lastDone = crawls.find((c) => c.status !== 'RUNNING') ?? null
  if (!!active !== crawling) setCrawling(!!active)

  const graphQuery = useQuery(LinkMapDocument, {
    variables: { channelId, maxNodes: budget },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const graph = graphQuery.data?.linkGraph
  const stats = graph?.stats

  // Pull the map once, when a running crawl finishes
  const refetchGraph = graphQuery.refetch
  const runningCrawl = useRef<string | null>(null)
  useEffect(() => {
    if (active) runningCrawl.current = active.id
    else if (runningCrawl.current) {
      runningCrawl.current = null
      void refetchGraph()
    }
  }, [active, refetchGraph])

  const [startCrawl, startState] = useMutation(StartSiteCrawlDocument, {
    onCompleted: () => {
      toast.success('Crawl started')
      void crawlsQuery.refetch()
    },
    onError: (err) => toast.error(err.message),
  })
  const [cancelCrawl] = useMutation(CancelSiteCrawlDocument, {
    onCompleted: () => {
      toast.info('Crawl cancelled')
      void crawlsQuery.refetch()
    },
  })

  const run = useCallback(() => {
    const pages = Number.parseInt(maxPages, 10)
    void startCrawl({
      variables: {
        input: {
          channelId,
          baseUrl: baseUrl.trim() || null,
          maxPages: Number.isFinite(pages) ? pages : 500,
        },
      },
    })
  }, [startCrawl, channelId, baseUrl, maxPages])

  const allPages = useMemo(() => {
    if (!graph) return { nodes: [] as MapNode[], edges: [] as MapEdge[] }
    const keep = new Set<string>()
    const mapped: MapNode[] = []
    for (const n of graph.nodes) {
      const type = n.type as MapNodeType
      if (hidden.has(type)) continue
      keep.add(n.key)
      mapped.push({
        key: n.key,
        entityId: n.entityId ?? null,
        type,
        label: n.label || n.path,
        path: n.path,
        statusCode: n.statusCode,
        noindex: n.noindex,
        isHomepage: n.isHomepage,
        kind: n.kind,
        state: n.state,
        inLinks: n.inLinks,
        outLinks: n.outLinks,
        depth: n.depth,
      })
    }
    const mappedEdges: MapEdge[] = []
    for (const e of graph.edges) {
      if (keep.has(e.from) && keep.has(e.to)) {
        mappedEdges.push({ from: e.from, to: e.to, nofollow: e.nofollow, boilerplate: e.boilerplate })
      }
    }
    return { nodes: mapped, edges: mappedEdges }
  }, [graph, hidden])

  const visibleEdges = useMemo(
    () => (showChrome ? allPages.edges : allPages.edges.filter((e) => !e.boilerplate)),
    [allPages, showChrome],
  )
  const nodes = allPages.nodes
  const edges = visibleEdges

  const typeCounts = useMemo(() => {
    const counts = new Map<MapNodeType, number>()
    for (const n of graph?.nodes ?? []) {
      const t = n.type as MapNodeType
      counts.set(t, (counts.get(t) ?? 0) + 1)
    }
    return counts
  }, [graph])

  const selected = selectedKey ? (nodes.find((n) => n.key === selectedKey) ?? null) : null
  const liveUrl = selected && graph ? graph.crawl.baseUrl.replace(/\/$/, '') + selected.path : null

  // Who links here / where it links out - straight off the loaded edge list, contextual links
  // ranked above chrome so the interesting ones surface in the visible eight
  const neighbors = useMemo(() => {
    if (!selectedKey) return null
    const byKey = new Map(allPages.nodes.map((n) => [n.key, n]))
    type Row = { node: MapNode; nofollow: boolean; boilerplate: boolean }
    const inbound: Row[] = []
    const outbound: Row[] = []
    for (const e of allPages.edges) {
      if (e.to === selectedKey) {
        const n = byKey.get(e.from)
        if (n) inbound.push({ node: n, nofollow: e.nofollow, boilerplate: e.boilerplate })
      } else if (e.from === selectedKey) {
        const n = byKey.get(e.to)
        if (n) outbound.push({ node: n, nofollow: e.nofollow, boilerplate: e.boilerplate })
      }
    }
    const rank = (a: Row, b: Row) =>
      Number(a.boilerplate) - Number(b.boilerplate) || b.node.inLinks - a.node.inLinks
    inbound.sort(rank)
    outbound.sort(rank)
    return { inbound, outbound }
  }, [selectedKey, allPages])

  if (!effectiveChannelId) {
    return (
      <DetailPageLayout toc={false}>
        <DetailPageHeader title="Link map" />
        <DetailPageBody>
          <DetailPageSection>
            <Alert>
              <TriangleAlertIcon />
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>
                A crawl walks one storefront, so the map is per channel. Switch the scope selector out of Global.
              </AlertDescription>
            </Alert>
          </DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    )
  }

  return (
    <DetailPageLayout toc={false}>
      <DetailPageHeader
        title="Link map"
        meta={selectedChannel ? `${selectedChannel.name} storefront` : undefined}
        badges={stats ? <Badge variant="secondary">{stats.totalNodes.toLocaleString()} pages</Badge> : undefined}
        actions={
          active ? (
            <Button variant="outline" onClick={() => void cancelCrawl({ variables: { channelId } })}>
              Stop crawl
            </Button>
          ) : (
            <Button onClick={run} disabled={startState.loading}>
              <RadarIcon /> Run crawl
            </Button>
          )
        }
      />
      <DetailPageBody>
        <DetailPageSection
          title="Crawl"
          description="Skelet fetches the storefront's own HTML and reads the links it actually contains. It never leaves the channel's origin and obeys that site's robots.txt."
        >
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-[16rem] flex-1">
              <Label htmlFor="crawl-base" className="text-xs">
                Start URL
              </Label>
              <Input
                id="crawl-base"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder={selectedChannel ? 'Channel public URL' : 'https://example.com'}
                className="h-9"
              />
            </div>
            <div className="w-32">
              <Label htmlFor="crawl-pages" className="text-xs">
                Page budget
              </Label>
              <Input
                id="crawl-pages"
                value={maxPages}
                onChange={(e) => setMaxPages(e.target.value)}
                inputMode="numeric"
                className="h-9"
              />
            </div>
          </div>

          {active ? (
            <div className="mt-3 rounded-md border border-l-2 border-l-sky-500 p-3">
              <div className="flex items-center gap-2 text-sm font-medium">
                <RadarIcon className="size-4 animate-pulse text-info" /> Crawling {active.baseUrl}
              </div>
              <div className="mt-1 text-xs text-muted-foreground tabular-nums">
                {active.pagesCrawled.toLocaleString()} of {active.maxPages.toLocaleString()} pages ·{' '}
                {active.linksFound.toLocaleString()} links found
              </div>
            </div>
          ) : null}

          {!active && lastDone ? (
            <p className="mt-3 text-xs text-muted-foreground">
              Last crawl {lastDone.status.toLowerCase()} · {lastDone.pagesCrawled.toLocaleString()} pages ·{' '}
              {lastDone.linksFound.toLocaleString()} links · {new Date(lastDone.startedAt).toLocaleString()}
              {lastDone.error ? ` · ${lastDone.error}` : ''}
            </p>
          ) : null}

          {graph && !graph.stats.complete ? (
            <Alert className="mt-3">
              <TriangleAlertIcon />
              <AlertTitle>Partial crawl - orphan findings are not valid yet</AlertTitle>
              <AlertDescription>
                The page budget ran out with links still queued. {graph.stats.discovered.toLocaleString()} pages are
                linked from somewhere but were never fetched, and {graph.stats.unseen.toLocaleString()} more were never
                seen at all - that second number only means "nothing links here" once a crawl finishes its queue. Raise
                the budget past {graph.stats.totalNodes.toLocaleString()} and run it again.
              </AlertDescription>
            </Alert>
          ) : null}
        </DetailPageSection>

        {!graph ? (
          <DetailPageSection title="Structure">
            <Alert>
              <RadarIcon />
              <AlertTitle>No crawl yet</AlertTitle>
              <AlertDescription>
                Run one and the map appears here: which pages link to which, how deep each one sits, and what nothing
                links to at all.
              </AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection title="Structure" description="Measured across the whole crawl, not just the drawn part.">
              <div className="grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-6">
                <Stat icon={NetworkIcon} label="Pages" value={stats!.totalNodes.toLocaleString()} />
                <Stat
                  icon={LayersIcon}
                  label={`Links (${stats!.boilerplate.toLocaleString()} are nav)`}
                  value={stats!.contextual.toLocaleString()}
                />
                <Stat
                  icon={UnplugIcon}
                  label="Broken (4xx / 5xx)"
                  value={stats!.broken.toLocaleString()}
                  tone={stats!.broken > 0 ? 'text-destructive' : undefined}
                />
                <Stat
                  icon={UnlinkIcon}
                  label={stats!.complete ? 'Never linked to' : 'Not reached (budget)'}
                  value={(stats!.complete ? stats!.unseen : stats!.discovered + stats!.unseen).toLocaleString()}
                  tone={stats!.complete && stats!.unseen > 0 ? 'text-warning' : undefined}
                />
                <Stat icon={LayersIcon} label="Deepest page (clicks)" value={String(stats!.maxDepth)} />
                <Stat icon={EyeOffIcon} label="noindex" value={stats!.noindexed.toLocaleString()} />
              </div>
              {stats!.dropped > 0 ? (
                <p className="mt-3 text-xs text-muted-foreground">
                  Drawing {stats!.returnedNodes.toLocaleString()} of {stats!.totalNodes.toLocaleString()} pages -{' '}
                  {stats!.dropped.toLocaleString()} left out. The site skeleton is kept first, then broken and
                  orphaned pages. Raise the budget to see more.
                </p>
              ) : null}
            </DetailPageSection>

            <DetailPageSection
              title="Changes since the last crawl"
              description="Only pages both crawls fetched are compared, so a shifting page budget cannot masquerade as a regression."
            >
              <CrawlDiffPanel channelId={channelId} />
            </DetailPageSection>

            <DetailPageSection
              title="On-page audit"
              description="Every page the crawl fetched, worst-first. Click a finding to filter, or export the table."
            >
              <CrawlAuditTable channelId={channelId} />
            </DetailPageSection>

            <DetailPageSection
              title="Map"
              description="Rings are clicks from the homepage. Links stay hidden until you hover a page - hover to trace what links to it and where it goes."
              trailing={
                <div className="flex flex-wrap items-center gap-2">
                  <Input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Highlight by name or path"
                    className="h-8 w-56"
                  />
                  <div className="flex rounded-md border p-0.5">
                    {COLOR_MODES.map((m) => (
                      <button
                        key={m.value}
                        type="button"
                        title={m.hint}
                        onClick={() => setColorMode(m.value)}
                        className={`rounded-sm px-2 py-1 text-xs ${colorMode === m.value ? 'bg-accent font-medium' : 'text-muted-foreground hover:bg-accent/50'}`}
                      >
                        {m.label}
                      </button>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={() => setShowChrome(!showChrome)}
                    title="Header and footer links repeat on nearly every page - hiding them keeps a hover to the links that actually say something"
                    className={`rounded-md border px-2 py-1 text-xs ${showChrome ? 'bg-accent font-medium' : 'text-muted-foreground hover:bg-accent/50'}`}
                  >
                    {showChrome ? 'Nav links shown' : 'Nav links hidden'}
                  </button>
                  <div className="flex rounded-md border p-0.5">
                    {BUDGETS.map((b) => (
                      <button
                        key={b}
                        type="button"
                        onClick={() => setBudget(b)}
                        className={`rounded-sm px-2 py-1 text-xs tabular-nums ${budget === b ? 'bg-accent font-medium' : 'text-muted-foreground hover:bg-accent/50'}`}
                      >
                        {b}
                      </button>
                    ))}
                  </div>
                </div>
              }
            >
              <div className="mb-3 flex flex-wrap items-center gap-1.5">
                {(Object.keys(NODE_COLORS) as MapNodeType[]).map((t) => {
                  const count = typeCounts.get(t) ?? 0
                  const off = hidden.has(t)
                  return (
                    <button
                      key={t}
                      type="button"
                      disabled={count === 0}
                      onClick={() => {
                        const next = new Set(hidden)
                        if (off) next.delete(t)
                        else next.add(t)
                        setHidden(next)
                      }}
                      className={`flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs transition-opacity disabled:opacity-30 ${off ? 'opacity-40' : ''}`}
                    >
                      <span className="size-2.5 rounded-full" style={{ background: NODE_COLORS[t] }} />
                      {NODE_LABELS[t]}
                      <span className="tabular-nums text-muted-foreground">{count}</span>
                    </button>
                  )
                })}
              </div>

              <LinkMapCanvas
                nodes={nodes}
                edges={edges}
                colorMode={colorMode}
                search={search}
                selectedKey={selectedKey}
                onSelect={setSelectedKey}
                maxDepth={stats!.maxDepth}
              />

              <div className="mt-3 flex flex-wrap gap-4 text-xs text-muted-foreground">
                <span>red ring = broken</span>
                <span>orange ring = nothing links in</span>
                <span>grey = linked but not fetched, so unmeasured</span>
                <span>hover a dot to draw its links</span>
                <span>diagonal slash = noindex</span>
                <span>bigger dot = more links</span>
              </div>

              {selected ? (
                <div className="mt-4 rounded-md border border-l-2 p-3" style={{ borderLeftColor: selected.isHomepage ? HOMEPAGE_COLOR : NODE_COLORS[selected.type] }}>
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{selected.label}</span>
                        <Badge variant="outline">{NODE_LABELS[selected.type]}</Badge>
                        {selected.isHomepage ? <Badge variant="secondary">Homepage</Badge> : null}
                        {selected.kind !== 'PAGE' ? (
                          <Badge variant="outline">{selected.kind === 'FILTER' ? 'Facet URL' : 'Has parameters'}</Badge>
                        ) : null}
                        {selected.noindex ? <Badge variant="destructive">noindex</Badge> : null}
                        {selected.state === 'CRAWLED' ? (
                          <Badge variant={selected.statusCode >= 400 ? 'destructive' : 'outline'}>
                            HTTP {selected.statusCode}
                          </Badge>
                        ) : null}
                      </div>
                      <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{selected.path}</div>
                      <div className="mt-2 text-xs text-muted-foreground">
                        {selected.state === 'CRAWLED'
                          ? `${selected.inLinks} pages link here · links out to ${selected.outLinks} · ${selected.depth} clicks from the homepage`
                          : selected.state === 'DISCOVERED'
                            ? `${selected.inLinks} pages link here, but the crawl hit its page budget before fetching it - raise the budget to measure it`
                            : 'Published with a URL that never turned up as a link on any page the crawl fetched'}
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="ghost" size="sm" onClick={() => setSelectedKey(null)}>
                        Clear
                      </Button>
                      {liveUrl ? (
                        <Button asChild size="sm" variant="outline">
                          <a href={liveUrl} target="_blank" rel="noreferrer">
                            <ExternalLinkIcon /> Visit
                          </a>
                        </Button>
                      ) : null}
                      {selected.entityId && DETAIL_ROUTES[selected.type] ? (
                        <Button asChild size="sm">
                          <Link to={`${DETAIL_ROUTES[selected.type]}/${selected.entityId}`}>Open</Link>
                        </Button>
                      ) : null}
                    </div>
                  </div>

                  {neighbors && (neighbors.inbound.length > 0 || neighbors.outbound.length > 0) ? (
                    <div className="mt-3 grid gap-4 border-t pt-3 sm:grid-cols-2">
                      <NeighborList
                        key={`in-${selected.key}`}
                        title={`Linked from (${neighbors.inbound.length})`}
                        rows={neighbors.inbound}
                        onPick={setSelectedKey}
                      />
                      <NeighborList
                        key={`out-${selected.key}`}
                        title={`Links out to (${neighbors.outbound.length})`}
                        rows={neighbors.outbound}
                        onPick={setSelectedKey}
                      />
                    </div>
                  ) : null}
                </div>
              ) : null}
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}


// One column of the selected page's link neighbourhood - rows are buttons, so clicking walks
// the selection through the graph without touching the canvas
function NeighborList({
  title,
  rows,
  onPick,
}: {
  title: string
  rows: { node: MapNode; nofollow: boolean; boilerplate: boolean }[]
  onPick: (key: string) => void
}) {
  const SHOWN = 8
  const [showAll, setShowAll] = useState(false)
  const visible = showAll ? rows : rows.slice(0, SHOWN)
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</div>
      {rows.length === 0 ? (
        <p className="mt-1.5 text-xs text-muted-foreground">None on the crawled pages.</p>
      ) : (
        <ul className={cn('mt-1.5 grid gap-1', showAll && 'max-h-64 overflow-y-auto pr-1')}>
          {visible.map((r) => (
            <li key={r.node.key} className="flex min-w-0 items-center gap-2">
              <span
                className="size-2 shrink-0 rounded-full"
                style={{ backgroundColor: r.node.isHomepage ? HOMEPAGE_COLOR : NODE_COLORS[r.node.type] }}
              />
              <button
                type="button"
                onClick={() => onPick(r.node.key)}
                className="min-w-0 truncate text-left text-xs hover:underline"
                title={r.node.path}
              >
                <span className="font-medium">{r.node.label}</span>
                <span className="ml-1.5 font-mono text-muted-foreground">{r.node.path}</span>
              </button>
              {r.boilerplate ? (
                <Badge variant="outline" className="shrink-0 px-1 py-0 text-[10px]">chrome</Badge>
              ) : null}
              {r.nofollow ? (
                <Badge variant="outline" className="shrink-0 px-1 py-0 text-[10px]">nofollow</Badge>
              ) : null}
            </li>
          ))}
          {rows.length > SHOWN ? (
            <li>
              <button
                type="button"
                onClick={() => setShowAll((v) => !v)}
                className="text-xs text-muted-foreground underline-offset-2 hover:underline"
              >
                {showAll ? 'Show fewer' : `Show all ${rows.length}`}
              </button>
            </li>
          ) : null}
        </ul>
      )}
    </div>
  )
}
