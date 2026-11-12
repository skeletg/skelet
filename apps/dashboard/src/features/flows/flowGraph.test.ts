import { describe, expect, it } from 'vitest'
import { attach, detach, flowChecks, GAP_X, GAP_Y, loops, reachable, tailOf, tidy, type Link, type Step } from './flowGraph'

const step = (id: string, type: string, data: Record<string, unknown> = {}, x = 0, y = 0): Step => ({ id, type, position: { x, y }, data })
const link = (source: string, target: string, sourceHandle?: string): Link => ({ id: `${source}>${target}`, source, target, sourceHandle })
const ids = () => {
  let n = 0
  return () => `l${++n}`
}
const ctx = { oneTimeLink: false, emails: new Set(['e1']), segments: new Set(['s1']) }

const t = step('t', 'trigger')
const mail = step('mail', 'send_email', { emailId: 'e1' })
const wait = step('wait', 'wait', { delaySeconds: 3600 })
const split = step('split', 'split', { condition: { kind: 'segment', segmentId: 's1' } })

describe('walking a flow', () => {
  it('follows a split by its two dots and every other step by its first plain connection', () => {
    const steps = [t, mail, split, wait, step('lost', 'webhook')]
    const links = [link('t', 'mail'), link('mail', 'split'), link('split', 'wait', 'no')]
    expect([...reachable(steps, links)].sort()).toEqual(['mail', 'split', 't', 'wait'])
    expect(tailOf(steps, links)?.id).toBe('wait')
    expect(tailOf([t], [])?.id).toBe('t')
  })

  it('sees a loop, and stops walking one', () => {
    const steps = [t, mail, wait]
    const round = [link('t', 'mail'), link('mail', 'wait'), link('wait', 'mail')]
    expect(loops(steps, round)).toBe(true)
    expect(loops(steps, round.slice(0, 2))).toBe(false)
    expect(tailOf(steps, round)?.id).toBe('wait')
  })
})

describe('attaching a new step', () => {
  it('hangs it under a step that led nowhere', () => {
    const r = attach([t], [], 't', { id: 'n', type: 'wait' }, ids())
    expect(r.attached).toBe(true)
    expect(r.links.map((l) => [l.source, l.target])).toEqual([['t', 'n']])
    expect(r.pushed).toEqual([])
  })

  it('splices it in before what followed, and says what moves down', () => {
    const r = attach([t, mail, wait], [link('t', 'mail'), link('mail', 'wait')], 't', { id: 'n', type: 'send_sms' }, ids())
    expect(r.links.map((l) => `${l.source}>${l.target}`).sort()).toEqual(['mail>wait', 'n>mail', 't>n'])
    expect(r.pushed.sort()).toEqual(['mail', 'wait'])
  })

  it('carries on from a spliced-in split by its yes dot', () => {
    const r = attach([t, mail], [link('t', 'mail')], 't', { id: 'n', type: 'split' }, ids())
    expect(r.links.find((l) => l.source === 'n')).toMatchObject({ target: 'mail', sourceHandle: 'yes' })
  })

  it('gives a split its yes dot, then its no dot, then nothing', () => {
    const first = attach([t, split], [link('t', 'split')], 'split', { id: 'a', type: 'wait' }, ids())
    expect(first.handle).toBe('yes')
    const second = attach([t, split], first.links, 'split', { id: 'b', type: 'wait' }, ids())
    expect(second.handle).toBe('no')
    const third = attach([t, split], second.links, 'split', { id: 'c', type: 'wait' }, ids())
    expect(third.attached).toBe(false)
    expect(third.links).toBe(second.links)
  })
})

describe('taking a step out', () => {
  it('joins the two ends of the line it sat in, keeping the dot it hung from', () => {
    const steps = [t, split, mail, wait]
    const links = [link('t', 'split'), link('split', 'mail', 'yes'), link('mail', 'wait')]
    const left = detach(steps, links, 'mail', ids())
    expect(left.map((l) => [l.source, l.target, l.sourceHandle])).toEqual([
      ['t', 'split', undefined],
      ['split', 'wait', 'yes'],
    ])
  })

  it('just drops the connections of a split, an end, or a step two others led to', () => {
    expect(detach([t, split, mail], [link('t', 'split'), link('split', 'mail', 'yes')], 'split', ids())).toEqual([])
    expect(detach([t, mail], [link('t', 'mail')], 'mail', ids())).toEqual([])
    const merged = [link('t', 'mail'), link('wait', 'mail'), link('mail', 'split')]
    expect(detach([t, wait, mail, split], merged, 'mail', ids())).toEqual([])
  })
})

describe('tidying a flow', () => {
  it('stacks a line under the trigger, which stays put', () => {
    const at = tidy([step('t', 'trigger', {}, 300, 40), mail, wait], [link('t', 'mail'), link('mail', 'wait')])
    expect(at.get('t')).toEqual({ x: 300, y: 40 })
    expect(at.get('mail')).toEqual({ x: 300, y: 40 + GAP_Y })
    expect(at.get('wait')).toEqual({ x: 300, y: 40 + 2 * GAP_Y })
  })

  it('sets a split\'s branches side by side under it, and parks loose steps to the right', () => {
    const steps = [t, split, mail, wait, step('lost', 'webhook')]
    const at = tidy(steps, [link('t', 'split'), link('split', 'mail', 'yes'), link('split', 'wait', 'no')])
    expect(at.get('mail')!.y).toBe(at.get('wait')!.y)
    expect(at.get('wait')!.x - at.get('mail')!.x).toBe(GAP_X)
    expect(at.get('split')!.x).toBe((at.get('mail')!.x + at.get('wait')!.x) / 2)
    expect(at.get('lost')!.x).toBeGreaterThan(at.get('wait')!.x)
  })
})

describe('checking a flow', () => {
  it('passes a flow that is wired and filled in', () => {
    expect(flowChecks([t, mail, wait], [link('t', 'mail'), link('mail', 'wait')], ctx)).toEqual([])
  })

  it('stops a trigger with nothing after it', () => {
    expect(flowChecks([t], [], ctx)).toEqual([{ level: 'error', step: 't', text: 'Nothing follows the trigger, so nobody would be enrolled.' }])
  })

  it('names what each step is missing', () => {
    const steps = [
      t,
      step('m1', 'send_email', {}),
      step('m2', 'send_email', { emailId: 'gone' }),
      step('sms', 'send_sms', { smsBody: ' ' }),
      step('sp', 'split', { condition: { kind: 'segment', segmentId: '' } }),
      step('seg', 'add_segment', { segmentId: 'gone' }),
      step('hook', 'webhook', { url: '' }),
    ]
    const links = [link('t', 'm1'), link('m1', 'm2'), link('m2', 'sms'), link('sms', 'sp'), link('sp', 'seg', 'yes'), link('seg', 'hook')]
    const found = flowChecks(steps, links, ctx)
    expect(found.every((c) => c.level === 'error')).toBe(true)
    expect(Object.fromEntries(found.map((c) => [c.step, c.text]))).toEqual({
      m1: 'Pick the email to send.',
      m2: 'The email it sends is no longer in the library.',
      sms: 'Write the message.',
      sp: 'Pick the segment to branch on.',
      seg: 'The segment it adds to no longer exists.',
      hook: 'Add the URL to call.',
    })
  })

  it('leaves alone a send step that carries an email of its own', () => {
    const own = step('m', 'send_email', { subject: 'Your order is on its way', blocks: [{ type: 'heading', text: 'Shipped' }] })
    expect(flowChecks([t, own], [link('t', 'm')], ctx)).toEqual([])
    const fresh = step('m', 'send_email', { subject: 'Hello {{customer.firstName}}', blocks: [{ type: 'heading', text: 'Hi {{customer.firstName}}' }], styles: {} })
    expect(flowChecks([t, fresh], [link('t', 'm')], ctx)[0]!.text).toBe('Pick the email to send.')
  })

  it('only warns about a step that never runs, whatever it is missing', () => {
    expect(flowChecks([t, wait, step('m', 'send_email', {})], [link('t', 'wait')], ctx)).toEqual([
      { level: 'warning', step: 'm', text: 'Not connected to the trigger, so it never runs.' },
    ])
  })

  it('refuses a webhook where the flow carries a one-time link, a loop, and a second way out', () => {
    const hook = step('hook', 'webhook', { url: 'https://example.com' })
    expect(flowChecks([t, hook], [link('t', 'hook')], { ...ctx, oneTimeLink: true })[0]!.text).toMatch(/one-time link/)
    expect(flowChecks([t, wait], [link('t', 'wait'), link('wait', 't')], ctx).some((c) => c.step === null && c.level === 'error')).toBe(true)
    const two = flowChecks([t, mail, wait], [link('t', 'mail'), link('t', 'wait')], ctx)
    expect(two).toContainEqual({ level: 'warning', step: 't', text: 'Has more than one connection out - only the first is followed.' })
  })

  it('does not call an email or a segment missing while the lists are still loading', () => {
    const steps = [t, step('m', 'send_email', { emailId: 'e9' }), step('seg', 'add_segment', { segmentId: 's9' })]
    expect(flowChecks(steps, [link('t', 'm'), link('m', 'seg')], { oneTimeLink: false, emails: null, segments: null })).toEqual([])
  })
})
