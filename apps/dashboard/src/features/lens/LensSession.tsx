import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import type { Replayer as RrwebReplayer } from 'rrweb'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LensSessionEventsDocument, LensSessionSignalsDocument } from './Lens.gql'

type RREvent = {
  type?: number
  timestamp?: number
  data?: { width?: number; height?: number; source?: number; type?: number; href?: string }
}

const SPEEDS = [1, 2, 4, 8]

// Core Web Vitals thresholds (good ≤ / poor >) - LCP/INP/TTFB ms, CLS unitless
const VITAL_THRESHOLDS: Record<string, [number, number]> = {
  LCP: [2500, 4000],
  INP: [200, 500],
  CLS: [0.1, 0.25],
  TTFB: [800, 1800],
}
function rateVital(metric: string, v: number): string {
  const t = VITAL_THRESHOLDS[metric]
  if (!t) return 'text-foreground'
  if (v <= t[0]) return 'text-success'
  if (v <= t[1]) return 'text-warning'
  return 'text-destructive'
}
function fmtVital(metric: string, v: number): string {
  if (metric === 'CLS') return v.toFixed(3)
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`
  return `${Math.round(v)}ms`
}

function fmtClock(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
function pathOf(href: string): string {
  try {
    return new URL(href).pathname || href
  } catch {
    return href
  }
}

type Mark = { kind: 'click' | 'nav' | 'input'; offset: number; pct: number; label: string; count?: number }

// Consecutive input events within this gap collapse into one "Input" entry - rrweb emits an input event per value
// change (per keystroke / per autofilled field), which otherwise floods the event list with dozens of identical rows
const INPUT_COALESCE_MS = 2000

// LensSession plays one recorded browser session via rrweb's Replayer (the
// rrweb-player Svelte wrapper renders blank under Vite/React) - On top of the
// reconstructed page it adds session-replay affordances: click + navigation
// markers on the scrub bar, a jump-to event list, playback speed and
// skip-inactivity - all driven off the same event stream
export function LensSession() {
  const { id = '' } = useParams()
  const { data, loading, error } = useQuery(LensSessionEventsDocument, {
    variables: { id },
    skip: !id,
  })
  // This recording's JS errors + web vitals (spine custom events keyed by sessionId)
  const { data: sigData } = useQuery(LensSessionSignalsDocument, { variables: { id }, skip: !id })
  const errors = useMemo(() => sigData?.lensSessionSignals.errors ?? [], [sigData])
  const vitals = useMemo(() => sigData?.lensSessionSignals.vitals ?? [], [sigData])

  // Parse once (multi-MB), then normalize the head: rrweb requires the initial Meta(4)+FullSnapshot(2) before any
  // IncrementalSnapshot(3) - A live recording can emit a pre-snapshot mutation in the same millisecond as startup
  const events = useMemo<RREvent[]>(() => {
    if (!data?.lensSessionEvents) return []
    let parsed: RREvent[]
    try {
      parsed = JSON.parse(data.lensSessionEvents) as RREvent[]
    } catch {
      return []
    }
    const fsIdx = parsed.findIndex((e) => e.type === 2)
    if (fsIdx <= 0) return parsed
    const metaIdx = parsed.findIndex((e) => e.type === 4)
    const taken = new Set<number>([fsIdx])
    const lead: RREvent[] = []
    if (metaIdx >= 0) {
      lead.push(parsed[metaIdx])
      taken.add(metaIdx)
    }
    lead.push(parsed[fsIdx])
    return [...lead, ...parsed.filter((_, i) => !taken.has(i))]
  }, [data])

  // Notable interactions for the markers + event list: navigations, clicks, inputs
  const marks = useMemo<Mark[]>(() => {
    if (events.length < 2) return []
    let lo = Infinity
    let hi = -Infinity
    for (const e of events) {
      const t = e.timestamp ?? 0
      if (t < lo) lo = t
      if (t > hi) hi = t
    }
    const span = hi - lo
    const out: Mark[] = []
    for (const e of events) {
      const off = (e.timestamp ?? 0) - lo
      const pct = span > 0 ? (off / span) * 100 : 0
      const d = e.data
      if (e.type === 4 && d?.href) {
        out.push({ kind: 'nav', offset: off, pct, label: `Visited ${pathOf(d.href)}` })
      } else if (e.type === 3 && d?.source === 2 && (d?.type === 2 || d?.type === 4)) {
        out.push({ kind: 'click', offset: off, pct, label: d.type === 4 ? 'Double-click' : 'Click' })
      } else if (e.type === 3 && d?.source === 5) {
        // Coalesce a run of inputs into the previous one (typing / autofill burst)
        const prev = out[out.length - 1]
        if (prev && prev.kind === 'input' && off - prev.offset <= INPUT_COALESCE_MS) {
          prev.count = (prev.count ?? 1) + 1
          prev.label = `Input ×${prev.count}`
          continue
        }
        out.push({ kind: 'input', offset: off, pct, label: 'Input' })
      }
    }
    return out
  }, [events])

  // Replay start (epoch ms) - maps error timestamps onto the timeline
  const startMs = useMemo(() => {
    let lo = Infinity
    for (const e of events) {
      const t = e.timestamp ?? 0
      if (t < lo) lo = t
    }
    return Number.isFinite(lo) ? lo : 0
  }, [events])

  // JS errors placed on the scrub bar + jump list (offset from replay start)
  const errorMarks = useMemo(
    () =>
      errors.map((e) => {
        const off = new Date(e.at).getTime() - startMs
        return { offset: off, message: e.message }
      }),
    [errors, startMs],
  )

  const mountRef = useRef<HTMLDivElement>(null)
  const replayerRef = useRef<RrwebReplayer | null>(null)
  const totalRef = useRef(0)
  const [mountErr, setMountErr] = useState('')
  const [playing, setPlaying] = useState(false)
  const [cur, setCur] = useState(0)
  const [totalMs, setTotalMs] = useState(0)
  const [speed, setSpeedState] = useState(1)
  const [skipInactive, setSkipInactive] = useState(false)

  useEffect(() => {
    const el = mountRef.current
    if (events.length < 2 || !el) return
    el.innerHTML = ''
    setMountErr('')
    let raf = 0
    let cancelled = false
    let replayer: RrwebReplayer | undefined

    void (async () => {
      try {
        await import('rrweb/dist/style.css')
        const { Replayer } = await import('rrweb')
        if (cancelled || !mountRef.current) return
        const host = mountRef.current
        host.innerHTML = ''
        replayer = new Replayer(events as never, {
          root: host,
          skipInactive: false,
          // Skip idle gaps longer than 2s (rrweb's default is 10s, which almost
          // never triggers on a real recording → the toggle looked broken)
          inactivePeriodThreshold: 2000,
          showWarning: false,
          mouseTail: { strokeStyle: '#6366f1' },
        })
        replayerRef.current = replayer
        totalRef.current = replayer.getMetaData().totalTime
        setTotalMs(totalRef.current)

        // Size the host to the SCALED recording (transform alone doesn't shrink
        // the layout box, which is what caused the empty gap + horizontal scroll)
        const rec = events.find((e) => e.type === 4)?.data
        const wrapper = host.querySelector('.replayer-wrapper') as HTMLElement | null
        if (rec?.width && rec?.height && wrapper) {
          const scale = Math.min(1, (host.clientWidth || 960) / rec.width)
          wrapper.style.transformOrigin = 'top left'
          wrapper.style.transform = `scale(${scale})`
          host.style.width = `${Math.round(rec.width * scale)}px`
          host.style.height = `${Math.round(rec.height * scale)}px`
          host.style.overflow = 'hidden'
        }

        replayer.play()
        setPlaying(true)
        const tick = () => {
          if (cancelled || !replayer) return
          setCur(replayer.getCurrentTime())
          raf = requestAnimationFrame(tick)
        }
        raf = requestAnimationFrame(tick)
      } catch (e) {
        console.error('[lens] rrweb Replayer failed', e)
        if (!cancelled) setMountErr(e instanceof Error ? e.message : String(e))
      }
    })()

    return () => {
      cancelled = true
      cancelAnimationFrame(raf)
      try {
        replayer?.pause()
      } catch {
        /* noop */
      }
      replayerRef.current = null
    }
  }, [events])

  const seekTo = useCallback((offsetMs: number) => {
    const r = replayerRef.current
    if (!r) return
    r.play(Math.max(0, offsetMs))
    setPlaying(true)
  }, [])

  const toggle = useCallback(() => {
    const r = replayerRef.current
    if (!r) return
    if (playing) {
      r.pause()
      setPlaying(false)
    } else {
      seekTo(r.getCurrentTime() >= totalRef.current ? 0 : r.getCurrentTime())
    }
  }, [playing, seekTo])

  const onScrub = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (totalRef.current <= 0) return
      const rect = e.currentTarget.getBoundingClientRect()
      const frac = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
      seekTo(frac * totalRef.current)
    },
    [seekTo],
  )

  // Stable handler for the event list - reads the offset off the row's data
  // attribute so we don't create a ref-reading closure per item during render
  const onEventClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      const off = Number(e.currentTarget.dataset.offset)
      if (Number.isFinite(off)) seekTo(off)
    },
    [seekTo],
  )

  const changeSpeed = useCallback(
    (sp: number) => {
      setSpeedState(sp)
      const r = replayerRef.current
      if (!r) return
      r.setConfig({ speed: sp })
      if (playing) seekTo(r.getCurrentTime()) // re-arm the timer at the new speed
    },
    [playing, seekTo],
  )

  const toggleSkip = useCallback(() => {
    const next = !skipInactive
    setSkipInactive(next)
    const r = replayerRef.current
    if (!r) return
    r.setConfig({ skipInactive: next })
    // rrweb only evaluates the skip on the next processed event; re-arm playback
    // from the current time so it takes effect immediately
    if (playing) seekTo(r.getCurrentTime())
  }, [playing, skipInactive, seekTo])

  const pct = totalMs > 0 ? Math.min(100, (cur / totalMs) * 100) : 0
  const replayable = events.length >= 2

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Session replay"
        meta={
          <span className="text-xs text-muted-foreground">
            <Link to="/lens/sessions" className="underline hover:text-foreground">
              All recordings
            </Link>
            {' · '}
            {id.slice(0, 8)}
          </span>
        }
      />
      <DetailPageBody>
        <DetailPageSection title="Replay" description="Scrub through the recorded session. Form inputs were masked at capture.">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading recording…</p>
          ) : error ? (
            <p className="text-sm text-destructive">Couldn't load this recording: {error.message}</p>
          ) : !replayable ? (
            <p className="text-sm text-muted-foreground">
              This recording is too short to replay ({events.length} event{events.length === 1 ? '' : 's'}).
            </p>
          ) : (
            <div className="grid gap-4 lg:grid-cols-[1fr_18rem]">
              <div className="min-w-0 space-y-2">
                {mountErr ? <p className="text-sm text-destructive">Player error: {mountErr}</p> : null}
                {/* Player viewport - host is sized to the scaled recording, centered */}
                <div className="flex justify-center overflow-hidden rounded-lg border bg-white">
                  <div ref={mountRef} />
                </div>
                {/* Controls */}
                <div className="flex flex-wrap items-center gap-3">
                  <button
                    type="button"
                    onClick={toggle}
                    className="rounded-md border px-3 py-1.5 text-sm font-medium hover:bg-muted"
                  >
                    {playing ? '❚❚ Pause' : '▶ Play'}
                  </button>
                  <span className="text-xs tabular-nums text-muted-foreground">
                    {fmtClock(cur)} / {fmtClock(totalMs)}
                  </span>
                  {/* Scrub bar with click/navigation markers */}
                  <div
                    onClick={onScrub}
                    className="relative h-2.5 min-w-[8rem] flex-1 cursor-pointer rounded-full bg-muted"
                    role="slider"
                    aria-label="Seek"
                    aria-valuenow={Math.round(pct)}
                    tabIndex={0}
                  >
                    <div className="absolute inset-y-0 left-0 rounded-full bg-primary/70" style={{ width: `${pct}%` }} />
                    {marks.map((m, i) => (
                      <span
                        key={i}
                        className={`pointer-events-none absolute top-1/2 h-3 w-0.5 -translate-y-1/2 ${
                          m.kind === 'click' ? 'bg-info' : m.kind === 'nav' ? 'bg-warning' : 'bg-muted-foreground'
                        }`}
                        style={{ left: `${m.pct}%` }}
                        title={`${fmtClock(m.offset)} · ${m.label}`}
                      />
                    ))}
                    {/* JS error markers - full-height red ticks */}
                    {totalMs > 0 &&
                      errorMarks.map((m, i) => (
                        <span
                          key={`err${i}`}
                          className="pointer-events-none absolute inset-y-0 w-0.5 bg-destructive"
                          style={{ left: `${Math.min(100, Math.max(0, (m.offset / totalMs) * 100))}%` }}
                          title={`${fmtClock(m.offset)} · error: ${m.message}`}
                        />
                      ))}
                  </div>
                  <div className="flex items-center gap-1">
                    {SPEEDS.map((sp) => (
                      <button
                        key={sp}
                        type="button"
                        onClick={() => changeSpeed(sp)}
                        className={`rounded px-2 py-1 text-xs font-medium ${
                          speed === sp ? 'bg-primary text-primary-foreground' : 'border hover:bg-muted'
                        }`}
                      >
                        {sp}×
                      </button>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={toggleSkip}
                    className={`rounded px-2 py-1 text-xs font-medium ${
                      skipInactive ? 'bg-primary text-primary-foreground' : 'border hover:bg-muted'
                    }`}
                    title="Skip periods with no activity"
                  >
                    Skip idle
                  </button>
                </div>
              </div>

              {/* Event list - jump to any click / navigation */}
              <div className="rounded-lg border">
                <div className="border-b px-3 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Events ({marks.length})
                </div>
                <ul className="max-h-[28rem] divide-y overflow-y-auto text-sm">
                  {marks.length === 0 ? (
                    <li className="px-3 py-2 text-muted-foreground">No clicks or navigations recorded.</li>
                  ) : (
                    marks.map((m, i) => (
                      <li key={i}>
                        <button
                          type="button"
                          data-offset={m.offset}
                          onClick={onEventClick}
                          className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-muted"
                        >
                          <span className="w-9 shrink-0 tabular-nums text-xs text-muted-foreground">{fmtClock(m.offset)}</span>
                          <span
                            className={`h-2 w-2 shrink-0 rounded-full ${
                              m.kind === 'click' ? 'bg-info' : m.kind === 'nav' ? 'bg-warning' : 'bg-muted-foreground'
                            }`}
                          />
                          <span className="truncate">{m.label}</span>
                        </button>
                      </li>
                    ))
                  )}
                </ul>
              </div>
            </div>
          )}
        </DetailPageSection>

        {replayable && (
          <DetailPageSection title="This session" description="Core Web Vitals + JS errors captured during this recording.">
            <div className="grid gap-4 lg:grid-cols-2">
              <div className="rounded-lg border p-4">
                <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Web vitals</p>
                {vitals.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No vitals captured.</p>
                ) : (
                  <div className="flex flex-wrap gap-4">
                    {vitals.map((v) => (
                      <div key={v.metric} className="min-w-16">
                        <p className="text-xs text-muted-foreground">{v.metric}</p>
                        <p className={`text-lg font-semibold tabular-nums ${rateVital(v.metric, v.value)}`}>{fmtVital(v.metric, v.value)}</p>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div className="rounded-lg border">
                <div className="border-b px-3 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  JS errors ({errors.length})
                </div>
                {errors.length === 0 ? (
                  <p className="px-3 py-2 text-sm text-muted-foreground">No errors 🎉</p>
                ) : (
                  <ul className="max-h-48 divide-y overflow-y-auto text-sm">
                    {errorMarks.map((m, i) => (
                      <li key={i}>
                        <button
                          type="button"
                          data-offset={m.offset}
                          onClick={onEventClick}
                          className="flex w-full items-start gap-2 px-3 py-1.5 text-left hover:bg-muted"
                          title="Jump to this moment"
                        >
                          <span className="w-9 shrink-0 pt-0.5 text-xs tabular-nums text-destructive">{fmtClock(m.offset)}</span>
                          <span className="truncate font-mono text-xs">{m.message || '(empty)'}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
