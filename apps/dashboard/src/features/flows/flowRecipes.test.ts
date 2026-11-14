import { describe, expect, it } from 'vitest'
import { flowChecks, reachable, type Step } from './flowGraph'
import { FLOW_RECIPES, buildRecipe } from './flowRecipes'
import { TRIGGER_LABELS } from './flowTypes'

const trigger: Step = { id: 't', type: 'trigger', position: { x: 100, y: 20 }, data: {} }
const ids = () => {
  let n = 0
  return () => `n${++n}`
}

describe('flow recipes', () => {
  it('start on a trigger the builder knows', () => {
    for (const r of FLOW_RECIPES) expect(TRIGGER_LABELS[r.trigger], r.id).toBeTruthy()
    expect(new Set(FLOW_RECIPES.map((r) => r.id)).size).toBe(FLOW_RECIPES.length)
  })

  it('build one connected line, with nothing left to do but pick the emails', () => {
    for (const r of FLOW_RECIPES) {
      const { steps, links } = buildRecipe(r, trigger, ids())
      const all = [trigger, ...steps]
      expect(reachable(all, links).size, r.id).toBe(all.length)
      const left = flowChecks(all, links, { oneTimeLink: false, emails: new Set(), segments: new Set() })
      expect(left.length, r.id).toBe(r.steps.filter((s) => s.type === 'send_email').length)
      expect(left.every((c) => c.text === 'Pick the email to send.'), r.id).toBe(true)
      expect(steps.every((s, i) => s.position.x === trigger.position.x && s.position.y > (steps[i - 1]?.position.y ?? trigger.position.y))).toBe(true)
    }
  })
})
