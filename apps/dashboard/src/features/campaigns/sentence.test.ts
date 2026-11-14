import { describe, expect, it } from 'vitest'
import { formatNumber } from '@/lib/format'
import { abSplit, audiencePhrase, dateParts, dayPhrase, fromNow, localInput, nextMorning, sendPlan, sendTime, whenPhrase, windowPhrase } from './sentence'

const now = new Date(2026, 10, 10, 15, 30)

describe('the campaign sentence', () => {
  it('names the audience the way it is picked', () => {
    expect(audiencePhrase('all', null, 'Vetrina Demo')).toBe('everyone on Vetrina Demo')
    expect(audiencePhrase('group', 'VIP', 'Vetrina Demo')).toBe('the VIP group')
    expect(audiencePhrase('segment', null, 'Vetrina Demo')).toBe('a segment')
  })

  it('says now, today, tomorrow or the day', () => {
    expect(whenPhrase(null, now)).toBe('now')
    expect(dayPhrase(new Date(2026, 10, 10, 18, 0), now)).toBe('today')
    expect(dayPhrase(new Date(2026, 10, 11, 9, 0), now)).toBe('tomorrow')
    expect(dayPhrase(new Date(2026, 10, 14, 9, 0), now)).toMatch(/^on .*14/)
    expect(dayPhrase(new Date(2027, 0, 4, 9, 0), now)).toContain('2027')
  })

  it('reads a test window in its largest whole unit', () => {
    expect(windowPhrase(240)).toBe('4 hours')
    expect(windowPhrase(60)).toBe('1 hour')
    expect(windowPhrase(90)).toBe('90 minutes')
    expect(windowPhrase(2880)).toBe('2 days')
  })

  it('reads a later time only once one is set', () => {
    expect(sendTime({ mode: 'now' })).toBeNull()
    expect(sendTime({ mode: 'later', at: '' })).toBeNull()
    expect(sendTime({ mode: 'later', at: '2026-11-14T09:00' })?.getHours()).toBe(9)
  })

  it('offers the next morning, or the next given weekday', () => {
    expect(localInput(nextMorning(now))).toBe('2026-11-11T09:00')
    expect(localInput(nextMorning(now, 1))).toBe('2026-11-16T09:00')
  })
})

describe('the send date', () => {
  it('splits into the figures the page sets large', () => {
    const d = dateParts(new Date(2026, 10, 11, 9, 5))
    expect(d.day).toBe('11')
    expect(d.month).toContain('2026')
    expect(d.clock).toMatch(/^0?9.05$/)
  })

  it('says how far off it is in the largest unit that fits', () => {
    const minutes = (n: number) => new Date(now.getTime() + n * 60_000)
    // In whatever language the runner speaks - the unit and the count are what's pinned
    const say = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
    expect(fromNow(minutes(30), now)).toBe(say.format(30, 'minute'))
    expect(fromNow(minutes(17 * 60 + 20), now)).toBe(say.format(17, 'hour'))
    expect(fromNow(minutes(3 * 1440), now)).toBe(say.format(3, 'day'))
    expect(fromNow(minutes(-5), now)).toBe(say.format(-5, 'minute'))
  })
})

describe('the A/B split', () => {
  // The server's rule: floor(total * percent / 100), at least 2, dealt A B A B
  it('scales the test slice to the people who are opted in', () => {
    expect(abSplit(1000, 1000, 20)).toEqual({ a: 100, b: 100, rest: 800 })
    expect(abSplit(1000, 400, 20)).toEqual({ a: 40, b: 40, rest: 320 })
    expect(abSplit(11, 11, 30)).toEqual({ a: 2, b: 1, rest: 8 })
  })

  it('never tests fewer than two people, and has nothing to split below that', () => {
    expect(abSplit(10, 10, 1)).toEqual({ a: 1, b: 1, rest: 8 })
    expect(abSplit(1, 1, 50)).toEqual({ a: 1, b: 0, rest: 0 })
    expect(abSplit(139, 0, 20)).toEqual({ a: 0, b: 0, rest: 0 })
  })

  it('never hands out more people than are opted in', () => {
    const s = abSplit(139, 2, 90)
    expect(s.a + s.b + s.rest).toBe(2)
  })
})

describe('the send plan', () => {
  it('is one step without a test', () => {
    expect(sendPlan({ at: null, eligible: 1204, ab: null })).toEqual([{ when: 'When you send', what: `${formatNumber(1204)} emails go out.` }])
    expect(sendPlan({ at: null, eligible: 1, ab: null })[0]!.what).toBe('1 email goes out.')
  })

  it('sends the winner after the test window', () => {
    const at = new Date(2026, 10, 11, 9, 0)
    const plan = sendPlan({ at, eligible: 1000, ab: { minutes: 240, metric: 'click', split: { a: 100, b: 100, rest: 800 } }, now })
    expect(plan).toHaveLength(2)
    expect(plan[0]!.when).toMatch(/^Tomorrow at/)
    expect(plan[0]!.what).toBe('Subject A goes to about 100 and subject B to about 100.')
    expect(plan[1]!.when).toMatch(/^Tomorrow at .*, or soon after$/)
    expect(plan[1]!.what).toBe('The subject with more clicks goes to the other 800. A tie goes to subject A.')
    expect(sendPlan({ at: null, eligible: null, ab: { minutes: 240, metric: 'open', split: null } })[1]!.when).toBe('4 hours later')
  })
})
