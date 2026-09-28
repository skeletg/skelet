import { ExternalLinkIcon } from 'lucide-react'
import { JAEGER_URL } from '@/lib/app-origins'
import {
  formatBytes,
  formatDuration,
  responseTone,
  type ResponseDetails,
  type ResponseTone,
} from './responseDetails'

// GraphiQL themes itself apart from the dashboard, so its footer takes GraphiQL's colours - the dashboard's tokens put dark text on its dark theme
const MUTED = 'text-[hsla(var(--color-neutral),var(--alpha-secondary))]'
const STRONG = 'text-[hsl(var(--color-neutral))]'
const TONE_CLASS: Record<ResponseTone, string> = {
  success: 'bg-[hsla(var(--color-success),var(--alpha-background-medium))] text-[hsl(var(--color-success))]',
  warning: 'bg-[hsla(var(--color-warning),var(--alpha-background-medium))] text-[hsl(var(--color-warning))]',
  destructive: 'bg-[hsla(var(--color-error),var(--alpha-background-medium))] text-[hsl(var(--color-error))]',
}

export function ResponseDetailsBar({ details }: { details: ResponseDetails | null }) {
  if (!details) {
    return <p className={`px-3 py-1.5 text-xs ${MUTED}`}>Run an operation to see its timing</p>
  }
  const tone = responseTone(details)
  return (
    <dl
      aria-label="Response details"
      className={`flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-1.5 text-xs ${MUTED}`}
    >
      <div className="flex items-center gap-1.5">
        <dt className="sr-only">Status</dt>
        <dd className={`rounded-sm px-1.5 py-0.5 font-mono font-medium ${TONE_CLASS[tone]}`}>
          {details.status}
          {details.statusText ? ` ${details.statusText}` : ''}
        </dd>
      </div>
      <Item label="Time" value={formatDuration(details.durationMs)} />
      <Item label="Size" value={formatBytes(details.sizeBytes)} />
      {details.errorCount > 0 && (
        <Item label="Errors" value={String(details.errorCount)} />
      )}
      {details.dbQueries !== null && <Item label="DB queries" value={String(details.dbQueries)} />}
      {details.requestId && <Item label="Request" value={details.requestId} mono />}
      {details.traceId && (
        <a
          href={`${JAEGER_URL}/trace/${details.traceId}`}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 hover:text-[hsl(var(--color-neutral))]"
        >
          Trace <ExternalLinkIcon className="size-3" aria-hidden />
        </a>
      )}
    </dl>
  )
}

function Item({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center gap-1">
      <dt>{label}</dt>
      <dd className={`font-medium ${STRONG} ${mono ? 'font-mono' : 'tabular-nums'}`}>{value}</dd>
    </div>
  )
}
