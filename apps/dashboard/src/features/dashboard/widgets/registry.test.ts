import { describe, expect, it } from 'vitest'
import { HOME_WIDGETS, visibleWidgets } from './registry'

const gates = (held: string[], features: string[] = []) => ({
  has: (slug: string) => held.includes(slug),
  featureEnabled: (flag: string) => features.includes(flag),
})

const ids = (list: { id: string }[]) => list.map((w) => w.id)

describe('the section registry', () => {
  // ids are storage keys in every admin's saved arrangement, so a duplicate would have two sections
  // fighting over one entry
  it('has unique ids', () => {
    expect(new Set(ids(HOME_WIDGETS)).size).toBe(HOME_WIDGETS.length)
  })

  it('opens with a curated page, not the whole library', () => {
    const on = HOME_WIDGETS.filter((w) => !w.defaultHidden)
    expect(on.length).toBeLessThan(HOME_WIDGETS.length)
    expect(ids(on)).toContain('revenue')
    expect(ids(on)).toContain('fulfillment')
    // a plugin's section must never be opt-in, or installing a plugin would appear to do nothing
    expect(ids(on)).toContain('plugins')
  })
})

describe('visibleWidgets', () => {
  it('keeps ungated sections for an admin with no grants at all', () => {
    const out = ids(visibleWidgets(HOME_WIDGETS, gates([])))
    expect(out).toEqual(['quick-actions', 'stats', 'moderation', 'account', 'plugins'])
  })

  it('admits a section once its permission is held', () => {
    expect(ids(visibleWidgets(HOME_WIDGETS, gates(['reports.view'])))).toContain('revenue')
    expect(ids(visibleWidgets(HOME_WIDGETS, gates(['inventory.view'])))).toContain('low-stock')
  })

  // both gates have to pass: a granted section behind an off feature switch still stays away
  it('needs the feature switch as well as the permission', () => {
    expect(ids(visibleWidgets(HOME_WIDGETS, gates(['lens.view'])))).not.toContain('traffic')
    expect(ids(visibleWidgets(HOME_WIDGETS, gates(['lens.view'], ['signals'])))).toContain('traffic')
  })

  it('holds everything gated back while the grants are still loading', () => {
    const out = ids(
      visibleWidgets(HOME_WIDGETS, { ...gates(['reports.view'], ['signals']), permsLoading: true }),
    )
    expect(out).not.toContain('revenue')
    expect(out).toContain('quick-actions')
  })

  it('holds feature-gated sections back while the switches are still loading', () => {
    const out = ids(
      visibleWidgets(HOME_WIDGETS, { ...gates(['lens.view'], ['signals']), featuresLoading: true }),
    )
    expect(out).not.toContain('traffic')
  })
})
