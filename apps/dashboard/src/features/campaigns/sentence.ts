import { formatNumber } from '@/lib/format'

export type When = { mode: 'now' } | { mode: 'later'; at: string }

export type Split = { a: number; b: number; rest: number }

export type PlanStep = { when: string; what: string }

export function sendTime(when: When): Date | null {
  if (when.mode === 'now' || !when.at) return null
  const d = new Date(when.at)
  return Number.isNaN(d.getTime()) ? null : d
}

export function hasPassed(at: Date): boolean {
  return at.getTime() <= Date.now()
}

export function localInput(d: Date): string {
  const two = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}T${two(d.getHours())}:${two(d.getMinutes())}`
}

export function nextMorning(now = new Date(), weekday?: number): Date {
  const d = new Date(now)
  d.setHours(9, 0, 0, 0)
  d.setDate(d.getDate() + 1)
  while (weekday !== undefined && d.getDay() !== weekday) d.setDate(d.getDate() + 1)
  return d
}

export function audiencePhrase(kind: string, picked: string | null, channel: string): string {
  if (kind === 'group') return picked ? `the ${picked} group` : 'a customer group'
  if (kind === 'segment') return picked ? `the ${picked} segment` : 'a segment'
  return channel ? `everyone on ${channel}` : 'everyone'
}

const sameDay = (a: Date, b: Date) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()

export function clock(d: Date): string {
  return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

export function dayPhrase(d: Date, now = new Date()): string {
  if (sameDay(d, now)) return 'today'
  const tomorrow = new Date(now)
  tomorrow.setDate(now.getDate() + 1)
  if (sameDay(d, tomorrow)) return 'tomorrow'
  const year = d.getFullYear() === now.getFullYear() ? {} : ({ year: 'numeric' } as const)
  return `on ${d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', ...year })}`
}

export function whenPhrase(at: Date | null, now = new Date()): string {
  return at ? `${dayPhrase(at, now)} at ${clock(at)}` : 'now'
}

const count = (n: number, unit: string) => `${formatNumber(n)} ${unit}${n === 1 ? '' : 's'}`

export function windowPhrase(minutes: number): string {
  if (minutes > 0 && minutes % 1440 === 0) return count(minutes / 1440, 'day')
  if (minutes > 0 && minutes % 60 === 0) return count(minutes / 60, 'hour')
  return count(minutes, 'minute')
}

// The server deals the test slice (at least two, A B A B) out of the whole audience, opted in or not - so an estimate
export function abSplit(total: number, eligible: number, percent: number): Split {
  if (eligible <= 0) return { a: 0, b: 0, rest: 0 }
  if (total < 2) return { a: eligible, b: 0, rest: 0 }
  const test = Math.min(Math.max(Math.floor((total * percent) / 100), 2), total)
  const share = eligible / total
  const a = Math.min(Math.round(Math.ceil(test / 2) * share), eligible)
  const b = Math.min(Math.round(Math.floor(test / 2) * share), eligible - a)
  return { a, b, rest: eligible - a - b }
}

const capital = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

export function sendPlan(o: {
  at: Date | null
  eligible: number | null
  ab: { minutes: number; metric: string; split: Split | null } | null
  now?: Date
}): PlanStep[] {
  const first = o.at ? capital(whenPhrase(o.at, o.now)) : 'When you send'
  if (!o.ab) {
    const what = o.eligible == null ? 'The email goes out to everyone opted in.' : o.eligible === 1 ? '1 email goes out.' : `${formatNumber(o.eligible)} emails go out.`
    return [{ when: first, what }]
  }
  const s = o.ab.split
  // The window is counted from when the test finishes going out, so the clock time is the earliest it can be
  const second = o.at ? `${capital(whenPhrase(new Date(o.at.getTime() + o.ab.minutes * 60_000), o.now))}, or soon after` : `${capital(windowPhrase(o.ab.minutes))} later`
  const measure = o.ab.metric === 'click' ? 'clicks' : 'opens'
  return [
    {
      when: first,
      what: s ? `Subject A goes to about ${formatNumber(s.a)} and subject B to about ${formatNumber(s.b)}.` : 'Subject A and subject B each go to half of the test group.',
    },
    {
      when: second,
      what: `The subject with more ${measure} goes to ${s ? `the other ${formatNumber(s.rest)}` : 'everyone else'}. A tie goes to subject A.`,
    },
  ]
}

export type DateParts = { day: string; weekday: string; month: string; clock: string; period: string }

export function dateParts(at: Date): DateParts {
  const date = new Intl.DateTimeFormat(undefined, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }).formatToParts(at)
  const time = new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit' }).formatToParts(at)
  const part = (type: string) => date.find((p) => p.type === type)?.value ?? ''
  return {
    day: part('day'),
    weekday: part('weekday'),
    month: `${part('month')} ${part('year')}`,
    clock: time.filter((p) => p.type !== 'dayPeriod').map((p) => p.value).join('').trim(),
    period: time.find((p) => p.type === 'dayPeriod')?.value ?? '',
  }
}

export function fromNow(at: Date, now = new Date()): string {
  const minutes = Math.round((at.getTime() - now.getTime()) / 60_000)
  const say = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  if (Math.abs(minutes) < 60) return say.format(minutes, 'minute')
  if (Math.abs(minutes) < 48 * 60) return say.format(Math.round(minutes / 60), 'hour')
  return say.format(Math.round(minutes / 1440), 'day')
}

export function timeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone.replace(/_/g, ' ')
}
