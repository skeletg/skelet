import { describe, expect, it } from 'vitest'
import { BLOCK_TYPES, type EmailBlock } from './blocks'
import { STARTER_GROUPS, STARTER_TEMPLATES } from './starters'

// What this pins: a starter only uses block types the server renders, and every group shows at least one
describe('the starter layouts', () => {
  it('use only known block types, under unique ids, in every group', () => {
    const known = new Set(BLOCK_TYPES.map((b) => b.type))
    const types = (blocks: EmailBlock[]): string[] => blocks.flatMap((b) => [b.type, ...(b.columns ?? []).flatMap((c) => types(c.blocks ?? []))])
    for (const s of STARTER_TEMPLATES) {
      expect(types(s.blocks).filter((t) => !known.has(t)), s.id).toEqual([])
    }
    expect(new Set(STARTER_TEMPLATES.map((s) => s.id)).size).toBe(STARTER_TEMPLATES.length)
    for (const g of STARTER_GROUPS) expect(STARTER_TEMPLATES.some((s) => s.group === g.id), g.id).toBe(true)
  })
})
