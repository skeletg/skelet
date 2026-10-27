import type { Link, LinkKind, OptionsModel } from './options-graph'

// Lanes g0, rules, g1, path, g2, branches, g3 - square lines in tracks of their own, so they cross but never overlap

export type Point = { x: number; y: number }
export type Box = { w: number; h: number; ports: Record<string, number> }
export type Route = { id: string; kind: LinkKind; from: string; to: string; points: Point[]; dot: boolean }
export type MapLayout = { positions: Map<string, Point>; routes: Route[]; width: number; height: number }

type Side = 'left' | 'right'
type Gutter = 'g0' | 'g1' | 'g2' | 'g3'

export const SLOT = 12
export const SLOT_TOP = 12
const TRACK = 10
const GUTTER_PAD = 18
const CHANNEL_MIN = 44
const STACK_GAP = 16
const CLEAR = 4

export function layoutOptionsMap(model: OptionsModel, box: (id: string) => Box): MapLayout {
  const options = new Map(model.options.map((o) => [o.id, o]))
  const rules = new Map(model.rules.map((r) => [r.id, r]))
  const rowOf = (id: string) => options.get(id)?.row ?? rules.get(id)?.row ?? -1
  const branch = (id: string) => !!options.get(id)?.branch

  // Channel c (between row c and c+1, -1 above the first) sits at index c+1
  const channels: Link[][] = Array.from({ length: model.rows + 1 }, () => [])
  for (const l of model.links) {
    if (l.kind === 'rejoin' || (l.kind === 'read' && branch(l.from))) channels[rowOf(l.from) + 1].push(l)
    if (l.kind === 'act' && branch(l.to)) channels[rowOf(l.to)].push(l)
  }
  const channelH = channels.map((ls) => Math.max(CHANNEL_MIN, 2 * GUTTER_PAD + (ls.length - 1) * TRACK))

  const rowH: number[] = new Array(model.rows).fill(0)
  for (const o of model.options) rowH[o.row] = Math.max(rowH[o.row], box(o.id).h)
  const stackTop = new Map<string, number>()
  const stacked: number[] = new Array(model.rows).fill(0)
  for (const r of model.rules) {
    stackTop.set(r.id, stacked[r.row])
    stacked[r.row] += box(r.id).h + STACK_GAP
    rowH[r.row] = Math.max(rowH[r.row], stacked[r.row] - STACK_GAP)
  }

  const rowTop: number[] = []
  const channelTop: number[] = []
  let y = box('start').h
  for (let c = -1; c < model.rows; c++) {
    if (c >= 0) {
      rowTop[c] = y
      y += rowH[c]
    }
    channelTop[c + 1] = y
    y += channelH[c + 1]
  }
  const cartTop = y
  const top = (id: string) =>
    id === 'start' ? 0 : id === 'cart' ? cartTop : rowTop[rowOf(id)] + (stackTop.get(id) ?? 0)
  const channelY = (index: number, l: Link) => {
    const ls = channels[index]
    return channelTop[index] + channelH[index] / 2 + (ls.indexOf(l) - (ls.length - 1) / 2) * TRACK
  }

  const asks = portEnds(model).map((e) => ({ ...e, key: `${e.link.id}:${e.end}`, far: top(e.end === 'from' ? e.link.to : e.link.from) }))
  const portY = new Map<string, number>()
  const facing: number[] = []
  const groups = new Map<string, typeof asks>()
  for (const a of asks) groups.set(`${a.node}|${a.side}`, [...(groups.get(`${a.node}|${a.side}`) ?? []), a])
  const order = [...groups.keys()].sort((a, b) => Number(a.startsWith('rule:')) - Number(b.startsWith('rule:')))
  for (const g of order) {
    const list = groups.get(g) ?? []
    const { node, side } = list[0]
    const b = box(node)
    const lines = Object.values(b.ports)
    const end = lines.length > 0 ? Math.min(...lines) - 12 : b.h - 6
    const fixed = list.filter((a) => a.choice && b.ports[a.choice] !== undefined)
    for (const a of fixed) portY.set(a.key, top(node) + b.ports[a.choice ?? ''])
    // A rule's ports dodge the option ports across g1, so their stubs never meet end to end
    const dodge = node.startsWith('rule:') && side === 'right' ? facing : []
    let next = SLOT_TOP
    for (const a of list.filter((x) => !fixed.includes(x)).sort((p, q) => p.far - q.far)) {
      while (next < end && dodge.some((f) => Math.abs(f - (top(node) + next)) < CLEAR)) next += SLOT / 3
      portY.set(a.key, top(node) + Math.min(next, Math.max(SLOT_TOP, end)))
      next += SLOT
    }
    if (node.startsWith('opt:') && !branch(node) && side === 'left') for (const a of list) facing.push(portY.get(a.key) ?? 0)
  }
  const py = (l: Link, end: 'from' | 'to') => portY.get(`${l.id}:${end}`) ?? top(l[end])

  type Run = { link: Link; gutter: Gutter; y1: number; y2: number; track: number }
  const runs: Run[] = []
  const run = (link: Link, gutter: Gutter, a: number, b: number) =>
    runs.push({ link, gutter, y1: Math.min(a, b), y2: Math.max(a, b), track: 0 })
  for (const l of model.links) {
    const [from, to] = [py(l, 'from'), py(l, 'to')]
    if (l.kind === 'show') run(l, branch(l.from) ? 'g3' : 'g2', from, to)
    else if (l.kind === 'read' && branch(l.from)) {
      const c = channelY(rowOf(l.from) + 1, l)
      run(l, 'g2', from, c)
      run(l, 'g1', c, to)
    } else if (l.kind === 'read') run(l, 'g1', from, to)
    else if (l.kind === 'act' && branch(l.to)) {
      const c = channelY(rowOf(l.to), l)
      run(l, 'g1', from, c)
      run(l, 'g2', c, to)
    } else if (l.kind === 'act') run(l, 'g1', from, to)
  }
  const tracks: Record<Gutter, number> = { g0: 0, g1: 0, g2: 0, g3: 0 }
  for (const gutter of ['g1', 'g2', 'g3'] as const) {
    const ends: number[] = []
    for (const r of runs.filter((x) => x.gutter === gutter).sort((a, b) => a.y1 - b.y1)) {
      let t = ends.findIndex((e) => e + 2 * CLEAR < r.y1)
      if (t === -1) t = ends.push(0) - 1
      ends[t] = r.y2
      r.track = t
    }
    tracks[gutter] = ends.length
  }

  const hasFx = model.links.some((l) => l.kind === 'fx')
  const hasBranch = model.options.some((o) => o.branch)
  const gutterW = (g: Gutter, present: boolean) => (tracks[g] > 0 ? 2 * GUTTER_PAD + (tracks[g] - 1) * TRACK : present ? 2 * GUTTER_PAD : 0)
  const ruleW = Math.max(0, ...model.rules.map((r) => box(r.id).w))
  const mainW = Math.max(box('start').w, box('cart').w, ...model.options.filter((o) => !o.branch).map((o) => box(o.id).w))
  const branchW = Math.max(0, ...model.options.filter((o) => o.branch).map((o) => box(o.id).w))
  const left: Record<Gutter, number> = { g0: 0, g1: 0, g2: 0, g3: 0 }
  left.g1 = gutterW('g0', hasFx) + ruleW
  const mainLeft = left.g1 + gutterW('g1', model.rules.length > 0)
  left.g2 = mainLeft + mainW
  const branchLeft = left.g2 + gutterW('g2', hasBranch)
  left.g3 = branchLeft + branchW
  const width = left.g3 + gutterW('g3', false)
  const mainX = mainLeft + mainW / 2
  const branchX = branchLeft + branchW / 2
  const trackX = (g: Gutter, t: number) => left[g] + GUTTER_PAD + t * TRACK

  const positions = new Map<string, Point>()
  const place = (id: string, cx: number | null) => positions.set(id, { x: cx === null ? left.g1 - box(id).w : cx - box(id).w / 2, y: top(id) })
  place('start', mainX)
  place('cart', mainX)
  for (const o of model.options) place(o.id, o.branch ? branchX : mainX)
  for (const r of model.rules) place(r.id, null)
  const xOf = (id: string, side: Side) => (positions.get(id)?.x ?? 0) + (side === 'right' ? box(id).w : 0)
  const bottom = (id: string) => top(id) + box(id).h
  const at = (l: Link, g: Gutter) => trackX(g, runs.find((r) => r.link === l && r.gutter === g)?.track ?? 0)

  const routes: Route[] = model.links.map((l) => {
    const [a, b] = [py(l, 'from'), py(l, 'to')]
    const p = (x: number, yy: number) => ({ x, y: yy })
    let points: Point[]
    switch (l.kind) {
      case 'path':
        points = [p(mainX, bottom(l.from)), p(mainX, top(l.to))]
        break
      case 'chain':
        points = [p(branchX, bottom(l.from)), p(branchX, top(l.to))]
        break
      case 'rejoin': {
        const c = channelY(rowOf(l.from) + 1, l)
        points = [p(branchX, bottom(l.from)), p(branchX, c), p(mainX, c), p(mainX, top(l.to))]
        break
      }
      case 'show': {
        const g = branch(l.from) ? 'g3' : 'g2'
        points = [p(xOf(l.from, 'right'), a), p(at(l, g), a), p(at(l, g), b), p(xOf(l.to, g === 'g3' ? 'right' : 'left'), b)]
        break
      }
      case 'read':
        if (branch(l.from)) {
          const c = channelY(rowOf(l.from) + 1, l)
          points = [p(xOf(l.from, 'left'), a), p(at(l, 'g2'), a), p(at(l, 'g2'), c), p(at(l, 'g1'), c), p(at(l, 'g1'), b), p(xOf(l.to, 'right'), b)]
        } else points = [p(xOf(l.from, 'left'), a), p(at(l, 'g1'), a), p(at(l, 'g1'), b), p(xOf(l.to, 'right'), b)]
        break
      case 'act':
        if (branch(l.to)) {
          const c = channelY(rowOf(l.to), l)
          points = [p(xOf(l.from, 'right'), a), p(at(l, 'g1'), a), p(at(l, 'g1'), c), p(at(l, 'g2'), c), p(at(l, 'g2'), b), p(xOf(l.to, 'left'), b)]
        } else points = [p(xOf(l.from, 'right'), a), p(at(l, 'g1'), a), p(at(l, 'g1'), b), p(xOf(l.to, 'left'), b)]
        break
      case 'fx': {
        const mid = top('cart') + box('cart').h / 2
        points = [p(xOf(l.from, 'left'), a), p(GUTTER_PAD, a), p(GUTTER_PAD, mid), p(xOf('cart', 'left'), mid)]
        break
      }
    }
    return { id: l.id, kind: l.kind, from: l.from, to: l.to, points, dot: l.kind === 'show' || l.kind === 'read' }
  })
  return { positions, routes, width, height: cartTop + box('cart').h }
}

type PortEnd = { link: Link; end: 'from' | 'to'; node: string; side: Side; choice: string | null }

function portEnds(model: OptionsModel): PortEnd[] {
  const branches = new Set(model.options.filter((o) => o.branch).map((o) => o.id))
  const out: PortEnd[] = []
  const add = (link: Link, end: 'from' | 'to', side: Side, choice: string | null = null) => out.push({ link, end, node: link[end], side, choice })
  for (const l of model.links) {
    if (l.kind === 'show') {
      add(l, 'from', 'right', l.choice ?? null)
      add(l, 'to', branches.has(l.from) ? 'right' : 'left')
    } else if (l.kind === 'read') {
      add(l, 'from', 'left', l.choice ?? null)
      add(l, 'to', 'right')
    } else if (l.kind === 'act') {
      add(l, 'from', 'right')
      add(l, 'to', 'left')
    } else if (l.kind === 'fx') add(l, 'from', 'left')
  }
  return out
}

// How many slots a card has to leave room for near its top, on its busier side - a rule gets a few spare to dodge with
export function slotCounts(model: OptionsModel): Map<string, number> {
  const per = new Map<string, number>()
  for (const e of portEnds(model)) if (!e.choice) per.set(`${e.node}|${e.side}`, (per.get(`${e.node}|${e.side}`) ?? 0) + 1)
  const out = new Map<string, number>()
  for (const [key, n] of per) {
    const node = key.slice(0, key.indexOf('|'))
    out.set(node, Math.max(out.get(node) ?? 0, n + (node.startsWith('rule:') ? 3 : 0)))
  }
  return out
}

export const NODE_W = 288
export const RULE_W = 256
export const STEP_W = 224
export const CHOICE_H = 20
export const headerRoom = (slots: number) => SLOT_TOP + slots * SLOT + 6

export function estimateBoxes(model: OptionsModel): Map<string, Box> {
  const slots = slotCounts(model)
  const out = new Map<string, Box>([
    ['start', { w: STEP_W, h: 34, ports: {} }],
    ['cart', { w: STEP_W, h: 34, ports: {} }],
  ])
  for (const o of model.options) {
    const header = Math.max((o.data.when ? 68 : 52) + (o.data.template ? 16 : 0), headerRoom(slots.get(o.id) ?? 0))
    const ports = Object.fromEntries(o.data.choices.map((c, i) => [c.id, header + i * CHOICE_H + CHOICE_H / 2]))
    out.set(o.id, { w: NODE_W, h: header + o.data.choices.length * CHOICE_H + (o.data.more > 0 ? CHOICE_H : 0) + 8, ports })
  }
  for (const r of model.rules) out.set(r.id, { w: RULE_W, h: Math.max(96, headerRoom(slots.get(r.id) ?? 0)), ports: {} })
  return out
}

export function roundedPath(points: Point[], radius = 6): string {
  const pts = points.filter((q, i) => i === 0 || q.x !== points[i - 1].x || q.y !== points[i - 1].y)
  let d = `M ${pts[0].x} ${pts[0].y}`
  for (let i = 1; i < pts.length - 1; i++) {
    const [a, b, c] = [pts[i - 1], pts[i], pts[i + 1]]
    const r = Math.min(radius, Math.hypot(b.x - a.x, b.y - a.y) / 2, Math.hypot(c.x - b.x, c.y - b.y) / 2)
    const toward = (from: Point, to: Point) => {
      const len = Math.hypot(to.x - from.x, to.y - from.y) || 1
      return { x: from.x + ((to.x - from.x) / len) * r, y: from.y + ((to.y - from.y) / len) * r }
    }
    const [p1, p2] = [toward(b, a), toward(b, c)]
    d += ` L ${p1.x} ${p1.y} Q ${b.x} ${b.y} ${p2.x} ${p2.y}`
  }
  const last = pts[pts.length - 1]
  return `${d} L ${last.x} ${last.y}`
}
