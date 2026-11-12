import { needsEmail, type FlowNodeData } from './flowTypes'

// The walk is the engine's (internal/flow): a split leaves by "yes" or "no", any other step by its first plain connection

export type Step = { id: string; type?: string; position: { x: number; y: number }; data: Record<string, unknown> }
export type Link = { id: string; source: string; target: string; sourceHandle?: string | null }
export type Check = { level: 'error' | 'warning'; step: string | null; text: string }

export const GAP_X = 240
export const GAP_Y = 130
export const STEP_WIDTH = 192 // a step on the canvas is w-48

const plain = (l: Link) => !l.sourceHandle || l.sourceHandle === 'out'

export function nextOf(links: Link[], id: string): string | null {
  return links.find((l) => l.source === id && plain(l))?.target ?? null
}

export function branchOf(links: Link[], id: string, handle: 'yes' | 'no'): string | null {
  return links.find((l) => l.source === id && l.sourceHandle === handle)?.target ?? null
}

function targets(step: Step, links: Link[]): string[] {
  const out = step.type === 'split' ? [branchOf(links, step.id, 'yes'), branchOf(links, step.id, 'no')] : [nextOf(links, step.id)]
  return out.filter((t): t is string => t != null)
}

function walk(steps: Step[], links: Link[], from: string[]): Set<string> {
  const byId = new Map(steps.map((s) => [s.id, s]))
  const seen = new Set<string>()
  const queue = [...from]
  while (queue.length > 0) {
    const id = queue.shift()!
    const step = byId.get(id)
    if (!step || seen.has(id)) continue
    seen.add(id)
    queue.push(...targets(step, links))
  }
  return seen
}

export function reachable(steps: Step[], links: Link[]): Set<string> {
  return walk(
    steps,
    links,
    steps.filter((s) => s.type === 'trigger').map((s) => s.id),
  )
}

export function loops(steps: Step[], links: Link[]): boolean {
  const byId = new Map(steps.map((s) => [s.id, s]))
  const done = new Set<string>()
  const open = new Set<string>()
  const visit = (id: string): boolean => {
    if (open.has(id)) return true
    const step = byId.get(id)
    if (!step || done.has(id)) return false
    open.add(id)
    const found = targets(step, links).some(visit)
    open.delete(id)
    done.add(id)
    return found
  }
  return steps.filter((s) => s.type === 'trigger').some((s) => visit(s.id))
}

export function tailOf(steps: Step[], links: Link[]): Step | null {
  const byId = new Map(steps.map((s) => [s.id, s]))
  let at = steps.find((s) => s.type === 'trigger') ?? null
  const seen = new Set<string>()
  while (at) {
    seen.add(at.id)
    const next = targets(at, links)[0]
    const step: Step | undefined = next && !seen.has(next) ? byId.get(next) : undefined
    if (!step) return at
    at = step
  }
  return at
}

// Hangs a new step under `anchor`, spliced in before whatever followed. A split gives it its first free dot, if any
export function attach(steps: Step[], links: Link[], anchorId: string, added: { id: string; type: string }, linkId: () => string) {
  const anchor = steps.find((s) => s.id === anchorId)
  if (!anchor) return { links, handle: null as string | null, pushed: [] as string[], attached: false }
  if (anchor.type === 'split') {
    const handle = !branchOf(links, anchor.id, 'yes') ? 'yes' : !branchOf(links, anchor.id, 'no') ? 'no' : null
    if (!handle) return { links, handle, pushed: [], attached: false }
    return { links: [...links, { id: linkId(), source: anchor.id, target: added.id, sourceHandle: handle }], handle, pushed: [], attached: true }
  }
  const old = links.find((l) => l.source === anchor.id && plain(l))
  const kept = old ? links.filter((l) => l !== old) : links
  const out: Link[] = [...kept, { id: linkId(), source: anchor.id, target: added.id }]
  if (!old) return { links: out, handle: '', pushed: [], attached: true }
  out.push({ id: linkId(), source: added.id, target: old.target, ...(added.type === 'split' ? { sourceHandle: 'yes' } : {}) })
  return { links: out, handle: '', pushed: [...walk(steps, kept, [old.target])], attached: true }
}

// Takes a step out. One that sat in a line - a single way in, a single way on - leaves the two ends joined
export function detach(steps: Step[], links: Link[], id: string, linkId: () => string): Link[] {
  const into = links.filter((l) => l.target === id)
  const onward = steps.find((s) => s.id === id)?.type === 'split' ? null : nextOf(links, id)
  const rest = links.filter((l) => l.source !== id && l.target !== id)
  return into.length === 1 && onward ? [...rest, { ...into[0]!, id: linkId(), target: onward }] : rest
}

// The trigger stays where it is, and whatever isn't connected goes in a column of its own to the right
export function tidy(steps: Step[], links: Link[]): Map<string, { x: number; y: number }> {
  const byId = new Map(steps.map((s) => [s.id, s]))
  const trigger = steps.find((s) => s.type === 'trigger')
  const placed = new Map<string, { x: number; y: number }>()
  if (!trigger) return placed
  const claimed = new Set<string>()
  const kids = new Map<string, string[]>()
  const claim = (id: string) => {
    claimed.add(id)
    const mine = targets(byId.get(id)!, links).filter((t) => byId.has(t) && !claimed.has(t))
    mine.forEach((t) => claimed.add(t))
    kids.set(id, mine)
    mine.forEach(claim)
  }
  claim(trigger.id)
  const width = (id: string): number => Math.max(1, (kids.get(id) ?? []).reduce((sum, k) => sum + width(k), 0))
  const place = (id: string, left: number, row: number) => {
    placed.set(id, { x: (left + (width(id) - 1) / 2) * GAP_X, y: row * GAP_Y })
    let at = left
    for (const k of kids.get(id) ?? []) {
      place(k, at, row + 1)
      at += width(k)
    }
  }
  place(trigger.id, 0, 0)
  const loose = steps.filter((s) => !placed.has(s.id))
  loose.forEach((s, i) => placed.set(s.id, { x: (width(trigger.id) + 0.5) * GAP_X, y: i * GAP_Y }))
  const origin = placed.get(trigger.id)!
  const dx = trigger.position.x - origin.x
  const dy = trigger.position.y - origin.y
  for (const [id, p] of placed) placed.set(id, { x: Math.round(p.x + dx), y: Math.round(p.y + dy) })
  return placed
}

// `emails` and `segments` are the ids that exist, or null while they're still loading
export function flowChecks(steps: Step[], links: Link[], ctx: { oneTimeLink: boolean; emails: Set<string> | null; segments: Set<string> | null }): Check[] {
  const out: Check[] = []
  const live = reachable(steps, links)
  const trigger = steps.find((s) => s.type === 'trigger')
  if (trigger && !nextOf(links, trigger.id)) out.push({ level: 'error', step: trigger.id, text: 'Nothing follows the trigger, so nobody would be enrolled.' })
  if (loops(steps, links)) out.push({ level: 'error', step: null, text: 'The flow loops back on itself, so a customer would never finish it.' })
  for (const s of steps) {
    if (s.type === 'trigger') continue
    if (!live.has(s.id)) {
      out.push({ level: 'warning', step: s.id, text: 'Not connected to the trigger, so it never runs.' })
      continue
    }
    const d = s.data as FlowNodeData
    const error = (text: string) => out.push({ level: 'error', step: s.id, text })
    if (s.type === 'send_email') {
      if (needsEmail(d)) error('Pick the email to send.')
      else if (d.emailId && ctx.emails && !ctx.emails.has(d.emailId)) error('The email it sends is no longer in the library.')
    } else if (s.type === 'send_sms') {
      if (!d.smsBody?.trim()) error('Write the message.')
    } else if (s.type === 'split') {
      const c = d.condition
      if (!c || ((c.kind === 'segment' || c.kind === 'in_segment') && !c.segmentId)) error('Pick the segment to branch on.')
      else if (c.segmentId && ctx.segments && !ctx.segments.has(c.segmentId)) error('The segment it branches on no longer exists.')
      if (!branchOf(links, s.id, 'yes') && !branchOf(links, s.id, 'no')) out.push({ level: 'warning', step: s.id, text: 'Neither branch leads anywhere.' })
    } else if (s.type === 'add_segment') {
      if (!d.segmentId) error('Pick the segment to add to.')
      else if (ctx.segments && !ctx.segments.has(d.segmentId)) error('The segment it adds to no longer exists.')
    } else if (s.type === 'webhook') {
      if (ctx.oneTimeLink) error("This flow carries a one-time link, so it can't call a webhook.")
      else if (!d.url?.trim()) error('Add the URL to call.')
    }
  }
  for (const s of steps) {
    if (s.type !== 'split' && live.has(s.id) && links.filter((l) => l.source === s.id && plain(l)).length > 1) {
      out.push({ level: 'warning', step: s.id, text: 'Has more than one connection out - only the first is followed.' })
    }
  }
  return out
}
