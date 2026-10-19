import { useEffect, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { PlusIcon, XIcon } from 'lucide-react'
import { CRON_PRESETS, SERVER_TZ, TIMEZONES } from './scheduler-options'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { PreviewScheduleDocument } from './Scheduler.gql'

// Radix Select reserves "" for the placeholder, so empty (= server time / custom
// cron) is mapped onto these sentinels
const CUSTOM_PRESET = '__custom__'

function fmtPreview(s: string): string {
  return new Date(s).toLocaleString()
}

/**
 * SchedulePicker renders a preset dropdown + free-text cron input + timezone
 * select, with a live "next runs" preview driven by the backend's
 * previewSchedule query (debounced so we don't query on every keystroke)
 * Timezone only affects cron expressions; @every/@descriptor schedules are
 * zone-independent and the preview reflects that
 */
export function SchedulePicker({
  schedule,
  onScheduleChange,
  timezone,
  onTimezoneChange,
}: {
  schedule: string
  onScheduleChange: (v: string) => void
  timezone: string
  onTimezoneChange: (v: string) => void
}) {
  const presetMatch = CRON_PRESETS.find((p) => p.value === schedule)
  const presetValue = presetMatch ? presetMatch.value : CUSTOM_PRESET

  // Debounce the schedule string feeding the preview query
  const [debounced, setDebounced] = useState(schedule)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(schedule), 350)
    return () => clearTimeout(t)
  }, [schedule])

  const { data, loading } = useQuery(PreviewScheduleDocument, {
    variables: { schedule: debounced, timezone, count: 5 },
    skip: debounced.trim() === '',
    fetchPolicy: 'cache-first',
  })
  const preview = data?.previewSchedule

  const isCron = schedule.trim() !== '' && !schedule.trim().startsWith('@')

  return (
    <div className="grid gap-3">
      <div className="grid gap-2">
        <Label>Schedule</Label>
        <Select
          value={presetValue}
          onValueChange={(v) => {
            if (v !== CUSTOM_PRESET) onScheduleChange(v)
          }}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CRON_PRESETS.map((p) => (
              <SelectItem key={p.value} value={p.value}>
                {p.label} <span className="text-muted-foreground">· {p.value}</span>
              </SelectItem>
            ))}
            <SelectItem value={CUSTOM_PRESET}>Custom…</SelectItem>
          </SelectContent>
        </Select>
        <Input
          value={schedule}
          onChange={(e) => onScheduleChange(e.target.value)}
          placeholder='Cron ("0 2 * * *") or interval ("@every 15m", "@daily")'
          className="font-mono text-sm"
        />
      </div>

      <div className="grid gap-2">
        <Label>Timezone</Label>
        <Select
          value={timezone === '' ? SERVER_TZ : timezone}
          onValueChange={(v) => onTimezoneChange(v === SERVER_TZ ? '' : v)}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={SERVER_TZ}>Server time</SelectItem>
            {TIMEZONES.map((tz) => (
              <SelectItem key={tz} value={tz}>
                {tz}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {!isCron && timezone !== '' && (
          <p className="text-[11px] leading-tight text-muted-foreground">
            Interval schedules ignore the timezone - it only affects cron expressions.
          </p>
        )}
      </div>

      <div className="rounded-md border bg-muted/30 px-3 py-2">
        <div className="text-xs font-medium text-muted-foreground">Next runs</div>
        {debounced.trim() === '' ? (
          <p className="mt-1 text-xs text-muted-foreground">Enter a schedule to preview.</p>
        ) : loading ? (
          <p className="mt-1 text-xs text-muted-foreground">Computing…</p>
        ) : preview && !preview.valid ? (
          <p className="mt-1 text-xs text-destructive">{preview.error || 'Invalid schedule.'}</p>
        ) : preview && preview.nextRuns.length > 0 ? (
          <ul className="mt-1 space-y-0.5">
            {preview.nextRuns.map((r: string, i: number) => (
              <li key={i} className="font-mono text-xs text-foreground">
                {fmtPreview(r)}
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-1 text-xs text-muted-foreground">No upcoming runs.</p>
        )}
      </div>
    </div>
  )
}

export type Header = { name: string; value: string }

/** HeadersEditor edits a list of HTTP header name/value rows */
export function HeadersEditor({
  headers,
  onChange,
}: {
  headers: Header[]
  onChange: (h: Header[]) => void
}) {
  function update(i: number, patch: Partial<Header>) {
    onChange(headers.map((h, idx) => (idx === i ? { ...h, ...patch } : h)))
  }
  return (
    <div className="grid gap-2">
      <Label>Headers (optional)</Label>
      {headers.length === 0 && (
        <p className="text-xs text-muted-foreground">No custom headers.</p>
      )}
      {headers.map((h, i) => (
        <div key={i} className="grid grid-cols-[1fr_1fr_auto] gap-2">
          <Input
            value={h.name}
            onChange={(e) => update(i, { name: e.target.value })}
            placeholder="X-Header"
            className="text-sm"
          />
          <Input
            value={h.value}
            onChange={(e) => update(i, { value: e.target.value })}
            placeholder="value"
            className="text-sm"
          />
          <Button aria-label="Remove header"
            type="button"
            variant="ghost"
            size="icon"
            className="size-9"
            onClick={() => onChange(headers.filter((_, idx) => idx !== i))}
          >
            <XIcon className="size-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="justify-self-start"
        onClick={() => onChange([...headers, { name: '', value: '' }])}
      >
        <PlusIcon className="size-3.5" /> Add header
      </Button>
    </div>
  )
}

/** RetryAlertFields edits the per-job retry/backoff + failure-alert config */
export function RetryAlertFields({
  maxAttempts,
  onMaxAttempts,
  retryBackoffSeconds,
  onRetryBackoffSeconds,
  alertAfterFailures,
  onAlertAfterFailures,
}: {
  maxAttempts: number
  onMaxAttempts: (n: number) => void
  retryBackoffSeconds: number
  onRetryBackoffSeconds: (n: number) => void
  alertAfterFailures: number
  onAlertAfterFailures: (n: number) => void
}) {
  const numField = (v: string, min: number, set: (n: number) => void) => {
    const n = Number(v)
    set(Number.isFinite(n) && n >= min ? n : min)
  }
  return (
    <div className="grid gap-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-1.5">
          <Label htmlFor="max-attempts" className="text-xs">Max attempts</Label>
          <Input
            id="max-attempts"
            type="number"
            min={1}
            value={maxAttempts}
            onChange={(e) => numField(e.target.value, 1, onMaxAttempts)}
          />
          <p className="text-[11px] leading-tight text-muted-foreground">1 = no retry.</p>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="backoff" className="text-xs">Retry backoff (s)</Label>
          <Input
            id="backoff"
            type="number"
            min={0}
            value={retryBackoffSeconds}
            onChange={(e) => numField(e.target.value, 0, onRetryBackoffSeconds)}
            disabled={maxAttempts <= 1}
          />
          <p className="text-[11px] leading-tight text-muted-foreground">Doubles each attempt, capped 6h.</p>
        </div>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="alert-after" className="text-xs">Alert after consecutive failures</Label>
        <Input
          id="alert-after"
          type="number"
          min={0}
          value={alertAfterFailures}
          onChange={(e) => numField(e.target.value, 0, onAlertAfterFailures)}
        />
        <p className="text-[11px] leading-tight text-muted-foreground">
          0 = off. Fires the global alert webhook once per failure streak (set the URL in scheduler settings).
        </p>
      </div>
    </div>
  )
}
