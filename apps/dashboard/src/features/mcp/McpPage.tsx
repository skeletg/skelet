import { useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { CheckIcon, ChevronDownIcon, CopyIcon } from 'lucide-react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { PickStore } from '@/components/layout/panels'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { LoadingState } from '@/components/ui/loading-state'
import { AuroraStage, Presence } from '@/features/agentic/aurora'
import { useConfigScope } from '@/lib/config-scope'
import { McpOverviewDocument } from './Mcp.gql'
import { adminMcpUrl, firstSentence, shellQuote, storefrontStatus, type Overview, type Tool } from './mcp'

export function McpPage() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const { data, loading, error } = useQuery(McpOverviewDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const o = data?.mcpOverview

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="MCP"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={<span className="text-xs text-muted-foreground">How AI agents connect to the store: shoppers' agents shop it, your team's read it</span>}
      />
      <DetailPageBody>
        {!isChannel ? (
          <DetailPageSection>
            <PickStore why="An agent connects to one store at a time" />
          </DetailPageSection>
        ) : error ? (
          <DetailPageSection>
            <Alert variant="destructive">
              <AlertTitle>Couldn't load the MCP endpoints</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : !o ? (
          <DetailPageSection>{loading ? <LoadingState /> : null}</DetailPageSection>
        ) : (
          <>
            <DetailPageSection>
              <McpStage s={o.storefront} channelId={effectiveChannelId!} store={selectedChannel?.name ?? 'this store'} />
            </DetailPageSection>
            <StorefrontSection s={o.storefront} />
            <TeamSection a={o.admin} channelId={effectiveChannelId!} />
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// The address shoppers' agents shop the store at, as the page's one big thing - or what keeps them out
function McpStage({ s, channelId, store }: { s: Overview['storefront']; channelId: string; store: string }) {
  const status = storefrontStatus(s)
  const [copied, copy] = useCopy()
  return (
    <AuroraStage>
      <div className="flex items-start gap-5 p-6 md:p-8">
        <Presence speaking={copied} />
        <div className="min-w-0 flex-1 pt-1">
          <p className="text-balance text-2xl font-semibold tracking-tight md:text-3xl">{status.on ? `Any AI agent can shop ${store} here.` : `Shoppers' agents can't reach ${store} yet.`}</p>
          {status.on && s.url ? (
            <>
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <span className="min-w-0 break-all rounded-md border bg-background/80 px-3 py-2 text-lg font-medium">{s.url}</span>
                <Button variant="outline" onClick={() => void copy(s.url!)} aria-label={`Copy ${s.url}`}>
                  {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              </div>
              <p className="mt-3 max-w-2xl text-sm text-muted-foreground">
                No sign-in. A shopper's own assistant searches, compares and fills a cart here, then hands them a link to finish on the store - it never places the order.
              </p>
            </>
          ) : (
            <p className="mt-2 max-w-2xl text-muted-foreground">
              {status.why}{' '}
              {s.available && !s.url ? (
                <Link to={`/channels/${channelId}`} className="text-foreground underline underline-offset-2">
                  Set the store's public URL
                </Link>
              ) : s.available && !s.enabled ? (
                <Link to="/settings/sales/agents" className="text-foreground underline underline-offset-2">
                  Turn it on
                </Link>
              ) : null}
            </p>
          )}
        </div>
      </div>
    </AuroraStage>
  )
}

function StorefrontSection({ s }: { s: Overview['storefront'] }) {
  return (
    <DetailPageSection
      title="Shoppers' agents"
      description="Connect your own agent to see the store the way a shopper's agent does, and every tool it can call."
      trailing={
        <Button asChild variant="outline" size="sm">
          <Link to="/settings/sales/agents">Settings</Link>
        </Button>
      }
    >
      {s.url ? (
        <div className="divide-y rounded-md border">
          <Row label="Try it">
            <Copyable value={`claude mcp add --transport http shop ${shellQuote(s.url)}`} />
            <p className="mt-1.5 text-xs text-muted-foreground">There's no sign-in: a client that asks for authentication should be told none.</p>
          </Row>
        </div>
      ) : null}
      <Tools tools={s.tools} label="What a shopper's agent can do" />
    </DetailPageSection>
  )
}

function TeamSection({ a, channelId }: { a: Overview['admin']; channelId: string }) {
  const url = adminMcpUrl(a.path)
  return (
    <DetailPageSection
      title="Your team's agents"
      description="Lets an agent like Claude Code answer questions from the store's own data - sales, orders, stock, customers. It's read-only, and an agent sees only what its token may."
      trailing={
        <Button asChild variant="outline" size="sm">
          <Link to="/integrations">Integrations</Link>
        </Button>
      }
    >
      <div className="divide-y rounded-md border">
        <Row label="Status">
          <StatusLine
            on={a.available}
            why={a.available ? 'Answers any agent with an integration token.' : 'Turned off on this server (MCP_ENABLED=false).'}
          />
        </Row>
        {a.available ? (
          <>
            <Row label="Address">
              <Copyable value={url} />
            </Row>
            <Row label="Connect">
              <ol className="list-decimal space-y-1.5 pl-4 text-sm">
                <li>
                  Create a token under <Link to="/integrations" className="underline underline-offset-2">Integrations</Link> with
                  the <code className="text-xs">.view</code> permissions the agent should have, pinned to this store.
                </li>
                <li>Add it to the agent, replacing the token:</li>
              </ol>
              <div className="mt-2">
                <Copyable value={`claude mcp add --transport http skelet ${shellQuote(url)} --header 'X-Api-Token: sk_...' --header ${shellQuote(`X-Channel-Id: ${channelId}`)}`} />
              </div>
            </Row>
          </>
        ) : null}
      </div>
      <Tools tools={a.tools} label="What your team's agents can read" />
    </DetailPageSection>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 px-4 py-3 sm:grid-cols-[8rem_1fr] sm:gap-4">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function StatusLine({ on, why }: { on: boolean; why: string }) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <Badge variant={on ? 'success' : 'secondary'}>{on ? 'On' : 'Off'}</Badge>
      <span className="text-muted-foreground">{why}</span>
    </div>
  )
}

function useCopy(): [boolean, (value: string) => Promise<void>] {
  const [copies, setCopies] = useState(0)
  useEffect(() => {
    if (!copies) return
    const id = window.setTimeout(() => setCopies(0), 1500)
    return () => window.clearTimeout(id)
  }, [copies])
  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopies((n) => n + 1)
    } catch {
      setCopies(0)
    }
  }
  return [copies > 0, copy]
}

function Copyable({ value }: { value: string }) {
  const [copied, copy] = useCopy()
  return (
    <div className="flex items-start gap-2">
      <code className="min-w-0 flex-1 rounded-sm bg-muted px-2 py-1.5 font-mono text-xs break-all">{value}</code>
      <Button variant="outline" size="sm" className="h-7 shrink-0 gap-1.5 text-xs" onClick={() => void copy(value)} aria-label={`Copy ${value}`}>
        {copied ? <CheckIcon className="size-3" /> : <CopyIcon className="size-3" />}
        {copied ? 'Copied' : 'Copy'}
      </Button>
    </div>
  )
}

function Tools({ tools, label }: { tools: Tool[]; label: string }) {
  return (
    <Collapsible className="mt-3 rounded-md border">
      <CollapsibleTrigger className="group flex w-full items-center justify-between px-4 py-3 text-left text-sm font-medium">
        <span>
          {label} <span className="font-normal text-muted-foreground">· {tools.length} tools</span>
        </span>
        <ChevronDownIcon className="size-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <ul className="divide-y border-t">
          {tools.map((t) => (
            <li key={t.name} className="grid gap-1 px-4 py-2.5 sm:grid-cols-[14rem_1fr] sm:gap-4">
              <code className="font-mono text-xs">{t.name}</code>
              <span className="text-sm text-muted-foreground" title={t.description}>
                {firstSentence(t.description)}
              </span>
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  )
}
