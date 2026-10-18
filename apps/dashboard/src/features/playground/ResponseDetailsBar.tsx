import { GQ } from './graphiqlTheme'
import {
  formatBytes,
  formatDuration,
  responseTone,
  type ResponseDetails,
  type ResponseTone,
} from './responseDetails'

const MUTED = GQ.muted
const STRONG = GQ.strong
const TONE_CLASS: Record<ResponseTone, string> = {
  success: GQ.success,
  warning: GQ.warning,
  destructive: GQ.errorBadge,
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
      {details.traceId && <Item label="Trace" value={details.traceId} mono />}
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
