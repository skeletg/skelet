import { describe, expect, it } from 'vitest'
import type { ProductOption, VisibilityGroup } from '../productOptionsStaging'
import { buildOptionsModel } from './options-graph'
import { estimateBoxes, layoutOptionsMap, roundedPath, type Point } from './options-layout'
import type { OptionRule, RuleAction } from './rules'

// What these pin: on hundreds of made-up products every line turns only at right angles, none passes behind a card or
// hugs one on its way, none runs on top of another unless both start or end at the same spot, and no two cards overlap

function random(seed: number) {
  let a = seed
  return () => {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function scenario(seed: number) {
  const rnd = random(seed)
  const pick = <T,>(xs: T[]) => xs[Math.floor(rnd() * xs.length)]
  const options: ProductOption[] = []
  const count = 1 + Math.floor(rnd() * 12)
  for (let i = 0; i < count; i++) {
    const choice = rnd() < 0.7
    const values = choice
      ? Array.from({ length: 1 + Math.floor(rnd() * 8) }, (_, k) => ({
          id: `o${i}v${k}`, title: `Choice ${k}`, sortOrder: k, priceValue: 0, priceType: 'FIXED' as const, isDefault: false, quantity: 1,
        }))
      : []
    let visibleIf: VisibilityGroup | undefined
    if (i > 0 && rnd() < 0.45) {
      const parent = options[Math.floor(rnd() * i)]
      visibleIf = parent.values.length > 0 && rnd() < 0.8
        ? { rules: [{ option_id: parent.id, op: 'anyOf', value_ids: [pick(parent.values).id] }] }
        : { rules: [{ option_id: parent.id, op: rnd() < 0.5 ? 'filled' : 'noneOf', value_ids: [] }] }
      if (rnd() < 0.3 && options[i - 1].visibleIf) visibleIf = options[i - 1].visibleIf as VisibilityGroup
    }
    options.push({
      id: `o${i}`, title: `Option ${i}`, helpText: '', inputType: choice ? 'DROPDOWN' : 'FIELD', isRequired: false, sortOrder: i, values,
      content: [{ locale: 'en', title: `Option ${i}`, helpText: '' }], visibleIf,
    })
  }
  const rules: OptionRule[] = Array.from({ length: Math.floor(rnd() * 7) }, (_, k) => {
    const reads = Array.from({ length: 1 + Math.floor(rnd() * 2) }, () => {
      const o = pick(options)
      return o.values.length > 0 && rnd() < 0.8
        ? { option_id: o.id, op: 'anyOf' as const, value_ids: [pick(o.values).id] }
        : { option_id: o.id, op: 'filled' as const }
    })
    const actions: RuleAction[] = Array.from({ length: 1 + Math.floor(rnd() * 3) }, () => {
      const roll = rnd()
      if (roll < 0.35) return { type: 'REQUIRE', optionId: pick(options).id }
      if (roll < 0.6) return { type: 'OPTIONAL', optionId: pick(options).id }
      if (roll < 0.8) return { type: 'SET_QUANTITY', quantity: 2 }
      return { type: 'ADJUST_PRICE', priceType: 'FIXED', priceValue: 100, channelPrices: [] }
    })
    return { id: `r${k}`, labels: {}, condition: { logic: 'and', rules: reads }, actions }
  })
  return { options, rules }
}

type Seg = { a: Point; b: Point }
const segments = (points: Point[]): Seg[] => points.slice(1).map((b, i) => ({ a: points[i], b }))
const same = (p: Point, q: Point) => Math.abs(p.x - q.x) < 0.5 && Math.abs(p.y - q.y) < 0.5

function overlap(s: Seg, t: Seg) {
  const flat = (x: Seg) => Math.abs(x.a.y - x.b.y) < 0.5
  if (flat(s) !== flat(t)) return 0
  const [fixed, from, to] = flat(s) ? (['y', 'x', 'x'] as const) : (['x', 'y', 'y'] as const)
  if (Math.abs(s.a[fixed] - t.a[fixed]) > 0.5) return 0
  const lo = Math.max(Math.min(s.a[from], s.b[to]), Math.min(t.a[from], t.b[to]))
  const hi = Math.min(Math.max(s.a[from], s.b[to]), Math.max(t.a[from], t.b[to]))
  return hi - lo
}

function crosses(s: Seg, r: { l: number; t: number; r: number; b: number }) {
  const [x1, x2, y1, y2] = [Math.min(s.a.x, s.b.x), Math.max(s.a.x, s.b.x), Math.min(s.a.y, s.b.y), Math.max(s.a.y, s.b.y)]
  return x1 < r.r && x2 > r.l && y1 < r.b && y2 > r.t
}

describe('options map layout', () => {
  it('keeps every line square, out from behind the cards and off the other lines', () => {
    const problems: string[] = []
    for (let seed = 1; seed <= 400; seed++) {
      const { options, rules } = scenario(seed)
      const model = buildOptionsModel(options, rules, 'en', 'en', 'USD')
      const boxes = estimateBoxes(model)
      const { positions, routes } = layoutOptionsMap(model, (id) => boxes.get(id) ?? { w: 0, h: 0, ports: {} })
      const rects = [...positions].map(([id, p]) => {
        const b = boxes.get(id) ?? { w: 0, h: 0 }
        return { id, l: p.x + 1, t: p.y + 1, r: p.x + b.w - 1, b: p.y + b.h - 1 }
      })
      rects.forEach((a, i) => {
        for (const b of rects.slice(i + 1)) if (a.l < b.r && a.r > b.l && a.t < b.b && a.b > b.t) problems.push(`seed ${seed}: ${a.id} overlaps ${b.id}`)
      })
      for (const route of routes) {
        segments(route.points).forEach((s, i, all) => {
          if (Math.abs(s.a.x - s.b.x) > 0.5 && Math.abs(s.a.y - s.b.y) > 0.5) problems.push(`seed ${seed}: ${route.id} turns at an angle`)
          for (const r of rects) if (crosses(s, r)) problems.push(`seed ${seed}: ${route.id} runs behind ${r.id}`)
          const between = i > 0 && i < all.length - 1
          for (const r of between ? rects : []) {
            if (crosses(s, { l: r.l - 8, t: r.t - 8, r: r.r + 8, b: r.b + 8 })) problems.push(`seed ${seed}: ${route.id} hugs ${r.id}`)
          }
        })
      }
      routes.forEach((p, i) => {
        for (const q of routes.slice(i + 1)) {
          if (same(p.points[0], q.points[0]) || same(p.points[p.points.length - 1], q.points[q.points.length - 1])) continue
          const hit = segments(p.points).some((s) => segments(q.points).some((t) => overlap(s, t) > 0.5))
          if (hit) problems.push(`seed ${seed}: ${p.id} runs on top of ${q.id}`)
        }
      })
    }
    expect(problems.slice(0, 10)).toEqual([])
  })

  it('rounds each corner off without moving the ends', () => {
    expect(roundedPath([{ x: 0, y: 0 }, { x: 0, y: 20 }, { x: 30, y: 20 }])).toBe('M 0 0 L 0 14 Q 0 20 6 20 L 30 20')
  })
})
