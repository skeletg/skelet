import { Fragment, useEffect, useState, type ComponentProps, type ReactNode } from 'react'
import { TriangleAlertIcon } from 'lucide-react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { EmailFrame, InboxRow, type Rendered } from '@/features/emails/PreviewDialog'
import { dateParts, fromNow, timeZone, type PlanStep, type Split } from './sentence'

export function Stage({ head, aside, children }: { head: ReactNode; aside: ReactNode; children: ReactNode }) {
  return (
    <div className="send-sheet @container overflow-hidden rounded-xl border bg-background">
      <div className="send-sheet-edge" aria-hidden />
      <div className="grid gap-4 px-6 pb-7 pt-6 @3xl:px-10 @3xl:pb-9 @3xl:pt-8">
        <div className="mb-2 flex flex-wrap items-start justify-between gap-x-10 gap-y-3">
          <div className="min-w-48 flex-1">{head}</div>
          <div className="shrink-0">{aside}</div>
        </div>
        {children}
      </div>
    </div>
  )
}

export function Sentence({ children, minor }: { children: ReactNode; minor?: boolean }) {
  return (
    <p
      className={cn(
        'text-pretty font-light tracking-tight text-muted-foreground',
        minor ? 'text-lg leading-[1.6] @3xl:text-xl' : 'text-[clamp(1.5rem,1rem_+_1.7cqi,2.125rem)] leading-[1.35]',
      )}
    >
      {children}
    </p>
  )
}

export function Said({ children }: { children: ReactNode }) {
  return <span className="font-medium text-foreground">{children}</span>
}

// Never clipped: an inline-block that hides its overflow loses its baseline and rides above the words around it
function Slot({ filled = true, className, ...props }: ComponentProps<'button'> & { filled?: boolean }) {
  return (
    <button
      type="button"
      className={cn(
        'inline-block max-w-full rounded-t-sm border-b-2 px-1 text-left align-baseline font-medium break-words outline-none transition-colors',
        'hover:border-foreground hover:bg-muted focus-visible:border-foreground focus-visible:bg-muted aria-expanded:border-foreground aria-expanded:bg-muted',
        filled ? 'border-foreground/25 text-foreground' : 'border-dashed border-foreground/45 text-foreground/55',
        className,
      )}
      {...props}
    />
  )
}

export function Pick({
  what,
  value,
  filled = true,
  className,
  onOpen,
  children,
}: {
  what: string
  value: string
  filled?: boolean
  className?: string
  onOpen?: () => void
  children: (close: () => void) => ReactNode
}) {
  const [open, setOpen] = useState(false)
  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (o) onOpen?.()
      }}
    >
      <PopoverTrigger asChild>
        <Slot filled={filled} aria-label={`${what}: ${value}`}>
          {value}
        </Slot>
      </PopoverTrigger>
      <PopoverContent align="start" sideOffset={8} className={cn('w-80 gap-0 p-0', className)}>
        {children(() => setOpen(false))}
      </PopoverContent>
    </Popover>
  )
}

export function Choice<T extends string>({ label, value, onChange, options }: { label: string; value: T; onChange: (v: T) => void; options: Array<[T, string]> }) {
  return (
    <div role="radiogroup" aria-label={label} className="grid auto-cols-fr grid-flow-col gap-1 rounded-md bg-muted p-1">
      {options.map(([v, text]) => (
        <button
          key={v}
          type="button"
          role="radio"
          aria-checked={value === v}
          onClick={() => onChange(v)}
          className={cn(
            'rounded px-2 py-1.5 text-xs transition-colors',
            value === v ? 'bg-background font-medium text-foreground shadow-xs' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {text}
        </button>
      ))}
    </div>
  )
}

// Keyed by place and value, so only what changed rolls in - words whole, since letters set apart lose their kerning
function Roll({ text, digits }: { text: string; digits?: boolean }) {
  const pieces = digits ? [...text] : text.split(' ')
  return (
    <span>
      <span className="sr-only">{text}</span>
      <span aria-hidden>
        {pieces.map((piece, i) => (
          <Fragment key={`${i}${piece}`}>
            {i > 0 && !digits ? ' ' : null}
            <span className="send-sheet-roll" style={{ animationDelay: `${i * (digits ? 28 : 60)}ms` }}>
              {piece}
            </span>
          </Fragment>
        ))}
      </span>
    </span>
  )
}

// Keeps its own clock, so the page around it doesn't re-render as time passes
function FromNow({ at }: { at: Date }) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const tick = setInterval(() => setNow(new Date()), 30_000)
    return () => clearInterval(tick)
  }, [])
  return <Roll text={fromNow(at, now)} />
}

const figure = 'text-[2.75rem] font-extralight leading-none tracking-tight tabular-nums text-foreground'

export function SendDate(props: { at: Date; lead: string } | { word: string; beside: [string, string]; note?: string }) {
  if ('word' in props) {
    return (
      <div className="grid justify-items-start gap-2 @3xl:justify-items-end">
        <div className="flex items-center gap-3">
          <span className={figure}>
            <Roll text={props.word} />
          </span>
          <span className="grid gap-0.5 text-xs leading-tight">
            <span className="font-medium text-foreground">{props.beside[0]}</span>
            <span className="text-muted-foreground">{props.beside[1]}</span>
          </span>
        </div>
        {props.note ? <p className="text-xs text-muted-foreground">{props.note}</p> : null}
      </div>
    )
  }
  const d = dateParts(props.at)
  return (
    <div className="grid justify-items-start gap-2 @3xl:justify-items-end">
      <div className="flex items-center gap-3">
        <span className={figure}>
          <Roll text={d.day} digits />
        </span>
        <span className="grid gap-0.5 text-xs leading-tight">
          <span className="font-medium text-foreground">
            <Roll text={d.weekday} />
          </span>
          <span className="text-muted-foreground">
            <Roll text={d.month} />
          </span>
        </span>
        <span aria-hidden className="mx-1 h-9 w-px bg-border" />
        <span className="flex items-start gap-1">
          <span className={figure}>
            <Roll text={d.clock} digits />
          </span>
          {d.period ? <span className="mt-0.5 text-xs font-medium text-muted-foreground">{d.period}</span> : null}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        {props.lead} <FromNow at={props.at} />, {timeZone()} time
      </p>
    </div>
  )
}

function Heading({ children }: { children: ReactNode }) {
  return <h2 className="text-sm font-semibold text-foreground">{children}</h2>
}

function Tag({ v }: { v: 'a' | 'b' }) {
  return (
    <span className="inline-flex w-7 shrink-0 items-center gap-1.5 text-xs font-semibold text-foreground">
      <span aria-hidden className={cn('size-2 rounded-full', v === 'a' ? 'bg-(--mail-a)' : 'bg-(--mail-b)')} />
      {v.toUpperCase()}
    </span>
  )
}

export function Landing({ r, subjectB, empty, action }: { r: Rendered | null; subjectB: string | null; empty: ReactNode; action?: ReactNode }) {
  return (
    <section aria-label="What lands" className="send-sheet grid min-w-0 grid-cols-1 content-start gap-3">
      <div className="flex items-center justify-between gap-3">
        <Heading>What lands</Heading>
        {r ? action : null}
      </div>
      {!r ? (
        <div className="grid min-h-48 place-items-center rounded-lg border border-dashed px-6 py-10 text-center text-sm text-muted-foreground">{empty}</div>
      ) : (
        <>
          {subjectB == null ? (
            <InboxRow r={r} />
          ) : (
            <div className="grid grid-cols-1 gap-2">
              <div className="flex items-center gap-2">
                <Tag v="a" />
                <div className="min-w-0 flex-1">
                  <InboxRow r={r} />
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Tag v="b" />
                <div className="min-w-0 flex-1">
                  <InboxRow r={{ ...r, subject: subjectB }} />
                </div>
              </div>
            </div>
          )}
          <div className="overflow-hidden rounded-lg border bg-white">
            <div className="max-h-[38rem] overflow-y-auto">
              <EmailFrame html={r.html} dark={false} />
            </div>
          </div>
        </>
      )}
    </section>
  )
}

type Part = { key: string; name: string; amount: number; about?: boolean; fill: string }

function parts(total: number, eligible: number, split: Split | null): Part[] {
  const out: Part[] = split
    ? [
        { key: 'a', name: 'Subject A', amount: split.a, about: true, fill: 'bg-(--mail-a)' },
        { key: 'b', name: 'Subject B', amount: split.b, about: true, fill: 'bg-(--mail-b)' },
        { key: 'rest', name: 'The winner, after the test', amount: split.rest, about: true, fill: 'send-sheet-either' },
      ]
    : [{ key: 'sent', name: 'Get the email', amount: eligible, fill: 'bg-(--mail-a)' }]
  return [...out, { key: 'out', name: 'Not opted in, skipped', amount: Math.max(total - eligible, 0), fill: 'bg-muted-foreground/25' }]
}

const share = (amount: number, total: number) => (total > 0 ? `${Math.round((amount / total) * 100)}%` : '0%')

export function Reach({ total, eligible, split, pending }: { total: number | null; eligible: number | null; split: Split | null; pending: string }) {
  if (total == null || eligible == null) {
    return (
      <section aria-label="Who gets it" className="grid gap-3">
        <Heading>Who gets it</Heading>
        <p className="text-sm text-muted-foreground">{pending}</p>
      </section>
    )
  }
  const list = parts(total, eligible, split)
  const drawn = list.filter((p) => p.amount > 0)
  const said = list.map((p) => `${p.name}: ${p.about ? 'about ' : ''}${formatNumber(p.amount)}`).join(', ')
  return (
    <section aria-label="Who gets it" className="send-sheet grid gap-3">
      <Heading>Who gets it</Heading>
      <div>
        <p className="text-5xl font-semibold leading-none tracking-tight text-foreground">{formatNumber(eligible)}</p>
        <p className="mt-2 text-sm text-muted-foreground">
          of {formatNumber(total)} {total === 1 ? 'customer' : 'customers'} in this audience {eligible === 1 ? 'is' : 'are'} opted in to marketing email.
        </p>
      </div>
      {eligible === 0 ? (
        <p className="flex items-start gap-1.5 text-sm text-warning">
          <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
          <span>Nobody here is opted in, so nothing would go out.</span>
        </p>
      ) : null}
      {total > 0 ? (
        <div role="img" aria-label={said} className="flex h-2 gap-0.5">
          {drawn.map((p) => (
            <Tooltip key={p.key}>
              <TooltipTrigger asChild>
                <span style={{ flexGrow: p.amount }} className={cn('h-full min-w-1 basis-0 rounded-[2px] first:rounded-l-full last:rounded-r-full', p.fill)} />
              </TooltipTrigger>
              <TooltipContent>
                {p.name}: {p.about ? 'about ' : ''}
                {formatNumber(p.amount)} ({share(p.amount, total)})
              </TooltipContent>
            </Tooltip>
          ))}
        </div>
      ) : null}
      <ul className="grid gap-1.5 text-sm">
        {list.map((p) => (
          <li key={p.key} className="flex items-center gap-2">
            <span aria-hidden className={cn('size-2.5 shrink-0 rounded-[3px]', p.fill)} />
            <span className="min-w-0 flex-1 truncate text-muted-foreground">{p.name}</span>
            <span className="tabular-nums text-foreground">
              {p.about ? '~' : ''}
              {formatNumber(p.amount)}
            </span>
          </li>
        ))}
      </ul>
      {split && (split.a === 0 || split.b === 0) && eligible > 0 ? (
        <p className="text-xs text-muted-foreground">With this few people opted in, a subject may reach no one, and a tie goes to subject A.</p>
      ) : null}
    </section>
  )
}

export function Plan({ steps }: { steps: PlanStep[] }) {
  const sequence = steps.length > 1
  return (
    <section aria-label="How it goes out" className="grid gap-3">
      <Heading>How it goes out</Heading>
      <ol className="grid gap-3">
        {steps.map((s, i) => (
          <li key={i} className={cn('grid gap-x-3', sequence && 'grid-cols-[1.5rem_1fr]')}>
            {sequence ? (
              <span aria-hidden className="mt-0.5 grid size-6 place-items-center rounded-full border text-xs font-medium tabular-nums text-muted-foreground">
                {i + 1}
              </span>
            ) : null}
            <div className="grid gap-0.5 text-sm">
              <p className="font-medium text-foreground">{s.when}</p>
              <p className="text-muted-foreground">{s.what}</p>
            </div>
          </li>
        ))}
      </ol>
      <p className="text-xs text-muted-foreground">Anyone who unsubscribes before their email is sent is skipped.</p>
    </section>
  )
}
