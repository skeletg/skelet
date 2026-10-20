import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef, type Row } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CheckIcon,
  CopyIcon,
  SendIcon,
  WifiOffIcon,
  XCircleIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { cn } from '@/lib/utils'
import { usePermissions } from '@/shared/auth/usePermissions'
import { RedeliverWebhookDeliveryDocument, WebhookDeliveriesDocument } from './Webhooks.gql'
import { useRefetchWhenShown, useVisiblePollInterval } from '@/lib/visible-poll'

// Headers the operator will almost always want at a glance - Rendered first
// and bolded in the key/value list
const NOTABLE_HEADERS = new Set([
  'content-type',
  'content-length',
  'server',
  'www-authenticate',
  'retry-after',
  'cf-ray',
  'x-webhook-signature-256',
])

// Reason phrases for the handful of codes that actually show up in webhook
// debugging - Anything not in the map falls back to just the numeric code
const HTTP_REASON: Record<number, string> = {
  200: 'OK',
  201: 'Created',
  202: 'Accepted',
  204: 'No Content',
  301: 'Moved Permanently',
  302: 'Found',
  307: 'Temporary Redirect',
  308: 'Permanent Redirect',
  400: 'Bad Request',
  401: 'Unauthorized',
  403: 'Forbidden',
  404: 'Not Found',
  405: 'Method Not Allowed',
  408: 'Request Timeout',
  409: 'Conflict',
  410: 'Gone',
  413: 'Payload Too Large',
  415: 'Unsupported Media Type',
  418: "I'm a Teapot",
  422: 'Unprocessable Entity',
  429: 'Too Many Requests',
  500: 'Internal Server Error',
  501: 'Not Implemented',
  502: 'Bad Gateway',
  503: 'Service Unavailable',
  504: 'Gateway Timeout',
}

type HeadersJson = Record<string, string | string[]> | null | undefined

type DeliveryRow = {
  id: string
  eventType: string
  url: string
  requestBody: string
  requestHeaders?: HeadersJson
  responseStatus: number | null
  responseBody: string | null
  responseHeaders?: HeadersJson
  durationMs: number
  success: boolean
  errorMessage: string | null
  attempt: number
  createdAt: string
}

type DeliveryKind = 'success' | 'http-error' | 'network'

function kindOf(row: DeliveryRow): DeliveryKind {
  if (row.success) return 'success'
  if (row.responseStatus != null) return 'http-error'
  return 'network'
}

function statusLabel(row: DeliveryRow): string {
  if (row.responseStatus != null) {
    const reason = HTTP_REASON[row.responseStatus]
    return reason ? `${row.responseStatus} ${reason}` : String(row.responseStatus)
  }
  return 'Network'
}

// Normalise header-like values (string or string[]) to a single line for
// display - Arrays are comma-joined - same thing browsers show in devtools
function formatHeaderValue(v: string | string[]): string {
  return Array.isArray(v) ? v.join(', ') : v
}

// Sort notable headers first, then everything else alphabetically
function sortedHeaderEntries(headers: HeadersJson): Array<[string, string]> {
  if (!headers) return []
  const entries = Object.entries(headers).map(
    ([k, v]) => [k, formatHeaderValue(v)] as [string, string],
  )
  return entries.sort(([a], [b]) => {
    const aNotable = NOTABLE_HEADERS.has(a.toLowerCase())
    const bNotable = NOTABLE_HEADERS.has(b.toLowerCase())
    if (aNotable !== bNotable) return aNotable ? -1 : 1
    return a.localeCompare(b)
  })
}

function headerValue(headers: HeadersJson, name: string): string | null {
  if (!headers) return null
  const target = name.toLowerCase()
  for (const [k, v] of Object.entries(headers)) {
    if (k.toLowerCase() === target) return formatHeaderValue(v)
  }
  return null
}

function isJsonContentType(headers: HeadersJson): boolean {
  const ct = headerValue(headers, 'content-type')
  return !!ct && ct.toLowerCase().includes('json')
}

function tryPrettyJson(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

// Shell-quote a single argument - wrap in single quotes, escape embedded quotes
function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function buildCurl(row: DeliveryRow): string {
  const parts = ['curl', '-X', 'POST', shellQuote(row.url)]
  if (row.requestHeaders) {
    for (const [k, v] of Object.entries(row.requestHeaders)) {
      parts.push('-H', shellQuote(`${k}: ${formatHeaderValue(v)}`))
    }
  }
  parts.push('--data-raw', shellQuote(row.requestBody))
  return parts.join(' ')
}

export function DeliveryLog({ webhookId }: { webhookId: string }) {
  const { data, loading, error, refetch } = useQuery(WebhookDeliveriesDocument, {
    variables: { webhookId, limit: 50 },
    pollInterval: useVisiblePollInterval(10000), // Refresh every 10s for debugging
  })
  useRefetchWhenShown(refetch)

  const columns = useMemo<ColumnDef<DeliveryRow>[]>(
    () => [
      {
        accessorKey: 'eventType',
        header: 'Event',
        cell: ({ row }) => (
          <Badge variant="outline" className="font-mono text-[10px] uppercase">
            {row.original.eventType.replace(/\./g, ' ')}
          </Badge>
        ),
      },
      {
        accessorKey: 'success',
        header: 'Status',
        cell: ({ row }) => <StatusCell row={row.original} />,
      },
      {
        accessorKey: 'durationMs',
        header: 'Duration',
        cell: ({ row }) => `${row.original.durationMs}ms`,
      },
      {
        accessorKey: 'attempt',
        header: 'Attempt',
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.attempt === 1 ? 'Initial' : `Retry ${row.original.attempt - 1}/3`}
          </span>
        ),
      },
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Time" />,
        cell: ({ row }) => new Date(row.original.createdAt).toLocaleString(),
      },
    ],
    [],
  )

  const rows = (data?.webhookDeliveries ?? []) as DeliveryRow[]

  return (
    <DataTable
      columns={columns}
      data={rows}
      loading={loading}
      error={error ? { message: error.message } : null}
      getRowId={(row) => row.id}
      emptyState="No deliveries yet."
      searchPlaceholder="Filter events…"
      renderSubComponent={({ row }: { row: Row<DeliveryRow> }) => (
        <DeliveryDetail row={row.original} />
      )}
    />
  )
}

function StatusCell({ row }: { row: DeliveryRow }) {
  const kind = kindOf(row)
  const label = statusLabel(row)

  const badge = (
    <Badge
      variant={kind === 'success' ? 'outline' : 'destructive'}
      className={cn(
        'gap-1 font-mono text-[11px]',
        kind === 'success' && 'border-success/30 text-success',
      )}
    >
      {kind === 'success' ? (
        <CheckCircle2Icon className="size-3" />
      ) : kind === 'network' ? (
        <WifiOffIcon className="size-3" />
      ) : (
        <XCircleIcon className="size-3" />
      )}
      {label}
    </Badge>
  )

  if (!row.errorMessage) return badge

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{badge}</span>
      </TooltipTrigger>
      <TooltipContent className="max-w-md font-mono text-[11px]" side="top">
        {row.errorMessage}
      </TooltipContent>
    </Tooltip>
  )
}

function DeliveryDetail({ row }: { row: DeliveryRow }) {
  const kind = kindOf(row)
  const hasResponse = row.responseStatus != null
  const responseIsJson = isJsonContentType(row.responseHeaders)
  const prettyRequest = useMemo(() => tryPrettyJson(row.requestBody), [row.requestBody])
  const prettyResponse = useMemo(
    () => (row.responseBody ? (responseIsJson ? tryPrettyJson(row.responseBody) : row.responseBody) : ''),
    [row.responseBody, responseIsJson],
  )

  return (
    <div
      className={cn(
        'grid gap-4 border-l-2 p-4',
        kind === 'success' ? 'border-l-green-500/60' : 'border-l-destructive/60',
      )}
      onClick={(e) => e.stopPropagation()}
    >
      {/* Request */}
      <section className="grid gap-2">
        <div className="flex items-center justify-between">
          <SectionTitle>Request</SectionTitle>
          <div className="flex items-center gap-1.5">
            <RedeliverButton deliveryId={row.id} />
            <CopyButton
              label="Copy as curl"
              value={buildCurl(row)}
              disabledWhenEmpty={!row.requestHeaders}
            />
          </div>
        </div>
        <KeyValue label="URL" value={row.url} mono />
        <HeadersList headers={row.requestHeaders} emptyLabel="No request headers captured." />
        <BodyBlock title="Body" body={prettyRequest} />
      </section>

      {/* Response */}
      <section className="grid gap-2">
        <SectionTitle>Response</SectionTitle>
        {hasResponse ? (
          <>
            <KeyValue label="Status" value={statusLabel(row)} mono />
            <HeadersList headers={row.responseHeaders} emptyLabel="No response headers." />
            {row.responseBody ? (
              <BodyBlock
                title="Body"
                body={prettyResponse}
                note={
                  row.responseBody.length >= 4000
                    ? 'Response body is truncated at 4 KB.'
                    : undefined
                }
              />
            ) : (
              <p className="text-xs text-muted-foreground">Empty response body.</p>
            )}
          </>
        ) : (
          <p className="text-xs text-muted-foreground">
            No response received - the request never reached a responding server.
          </p>
        )}
      </section>

      {/* Error */}
      {row.errorMessage && (
        <section className="grid gap-2">
          <SectionTitle>Error</SectionTitle>
          <pre className="whitespace-pre-wrap rounded border border-destructive/20 bg-destructive/5 p-2 font-mono text-xs text-destructive">
            {row.errorMessage}
          </pre>
        </section>
      )}

      {/* Timing */}
      <section className="grid gap-1 text-xs text-muted-foreground">
        <SectionTitle>Timing</SectionTitle>
        <div className="flex flex-wrap gap-x-6 gap-y-1 font-mono">
          <span>Duration: {row.durationMs}ms</span>
          <span>
            Attempt {row.attempt} of 4 ({row.attempt === 1 ? 'initial' : `retry ${row.attempt - 1}/3`})
          </span>
          <span>At: {new Date(row.createdAt).toLocaleString()}</span>
        </div>
      </section>
    </div>
  )
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </span>
  )
}

function KeyValue({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="grid grid-cols-[120px_1fr] items-baseline gap-2 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className={cn('break-all', mono && 'font-mono')}>{value}</span>
    </div>
  )
}

function HeadersList({ headers, emptyLabel }: { headers: HeadersJson; emptyLabel: string }) {
  const entries = sortedHeaderEntries(headers)
  if (entries.length === 0) {
    return <p className="text-xs text-muted-foreground">{emptyLabel}</p>
  }
  return (
    <div className="rounded border bg-background">
      <div className="grid">
        {entries.map(([name, value], i) => {
          const notable = NOTABLE_HEADERS.has(name.toLowerCase())
          return (
            <div
              key={`${name}-${i}`}
              className={cn(
                'grid grid-cols-[minmax(140px,220px)_1fr] gap-3 border-b px-2 py-1.5 font-mono text-[11px] last:border-b-0',
                notable && 'bg-muted/40',
              )}
            >
              <span className={cn('truncate', notable ? 'font-semibold' : 'text-muted-foreground')}>
                {name}
              </span>
              <span className="break-all">{value}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function BodyBlock({ title, body, note }: { title: string; body: string; note?: string }) {
  return (
    <div className="grid gap-1">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          {title}
        </span>
        {note && <span className="text-[10px] text-muted-foreground">{note}</span>}
      </div>
      <pre className="max-h-64 overflow-auto rounded border bg-background p-2 font-mono text-[11px]">
        {body}
      </pre>
    </div>
  )
}

// RedeliverButton re-enqueues this delivery's exact payload through the
// async queue - the receiver gets an identical copy with a fresh signature
function RedeliverButton({ deliveryId }: { deliveryId: string }) {
  const { has } = usePermissions()
  const [redeliver, { loading }] = useMutation(RedeliverWebhookDeliveryDocument)
  const [queued, setQueued] = useState(false)

  if (!has('webhooks.manage')) return null

  async function onClick() {
    try {
      await redeliver({ variables: { deliveryId } })
      setQueued(true)
      setTimeout(() => setQueued(false), 2500)
    } catch {
      // surfaced via the mutation error state below
    }
  }

  return (
    <Button
      variant="outline"
      size="sm"
      className="h-7 gap-1.5 text-xs"
      onClick={onClick}
      disabled={loading || queued}
    >
      {queued ? <CheckIcon className="size-3" /> : <SendIcon className="size-3" />}
      {queued ? 'Queued' : 'Redeliver'}
    </Button>
  )
}

function CopyButton({
  label,
  value,
  disabledWhenEmpty,
}: {
  label: string
  value: string
  disabledWhenEmpty?: boolean
}) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // ignore
    }
  }

  return (
    <Button
      variant="outline"
      size="sm"
      className="h-7 gap-1.5 text-xs"
      onClick={copy}
      disabled={disabledWhenEmpty}
    >
      {copied ? <CheckIcon className="size-3" /> : <CopyIcon className="size-3" />}
      {copied ? 'Copied' : label}
    </Button>
  )
}
