import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearFirstLogin, markFirstLogin, peekFirstLogin } from './first-login'
import { visibleSteps } from './welcome-context'
import { WELCOME_STEPS, type WelcomeStep } from './steps'
import sidebarSrc from '@/components/layout/app-sidebar.tsx?raw'
import tabsSrc from '@/components/layout/workspace-tab-strip.tsx?raw'
import searchSrc from '@/components/layout/admin-search.tsx?raw'
import bellSrc from '@/features/notifications/NotificationBell.tsx?raw'
import userSrc from '@/components/layout/nav-user.tsx?raw'

const step = (id: string, extra: Partial<WelcomeStep> = {}): WelcomeStep => ({
  id,
  title: id,
  Body: () => null,
  ...extra,
})

beforeEach(() => localStorage.clear())

describe('the first-login marker', () => {
  it('is off until the login form sets it', () => {
    expect(peekFirstLogin()).toBe(false)
    markFirstLogin()
    expect(peekFirstLogin()).toBe(true)
  })

  // peeking has to be repeatable, because it happens during render - only the clear consumes it
  it('survives repeated peeks and dies on the clear', () => {
    markFirstLogin()
    expect(peekFirstLogin()).toBe(true)
    expect(peekFirstLogin()).toBe(true)
    clearFirstLogin()
    expect(peekFirstLogin()).toBe(false)
  })

  it('clears idempotently, so a remount costs nothing', () => {
    markFirstLogin()
    clearFirstLogin()
    clearFirstLogin()
    expect(peekFirstLogin()).toBe(false)
  })

  it('shrugs off storage that throws', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('private mode')
    })
    expect(peekFirstLogin()).toBe(false)
    spy.mockRestore()
  })
})

describe('visibleSteps', () => {
  const gates = (held: string[], features: string[] = []) => ({
    has: (slug: string) => held.includes(slug),
    featureEnabled: (flag: string) => features.includes(flag),
  })
  const ids = (steps: WelcomeStep[]) => steps.map((s) => s.id)

  it('keeps ungated steps for an admin with no grants', () => {
    const steps = [step('a'), step('b', { permission: 'reports.view' })]
    expect(ids(visibleSteps(steps, gates([])))).toEqual(['a'])
  })

  it('admits a step once its permission is held', () => {
    const steps = [step('b', { permission: 'reports.view' })]
    expect(ids(visibleSteps(steps, gates(['reports.view'])))).toEqual(['b'])
  })

  // nobody should be introduced to a feature their deployment has switched off
  it('needs the feature switch too', () => {
    const steps = [step('c', { feature: 'signals' })]
    expect(ids(visibleSteps(steps, gates([])))).toEqual([])
    expect(ids(visibleSteps(steps, gates([], ['signals'])))).toEqual(['c'])
  })
})

// Every step points at a `data-tour` attribute on a real element. A rename or a removal in the shell
// would leave the tour dimming the page with nothing highlighted, which is exactly the kind of
// silent rot a test can catch - whether the element actually LAYS OUT is a rendering question, and
// the Playwright pass covers that
describe('tour anchors', () => {
  // The shell files that stamp the attributes, read as text through Vite's ?raw. Deliberately NOT a directory walk that includes this
  // feature: the step definitions name the attributes, so scanning them would make the assertion true by construction
  const stamped = new Set(
    [sidebarSrc, tabsSrc, searchSrc, bellSrc, userSrc]
      .flatMap((src) => [...src.matchAll(/data-tour="([\w-]+)"/g)])
      .map((m) => m[1]),
  )

  it('finds the attributes it points at', () => {
    expect(stamped.size).toBeGreaterThan(0)
    for (const step of WELCOME_STEPS) {
      if (!step.target) continue
      const name = /\[data-tour="([\w-]+)"\]/.exec(step.target)?.[1]
      expect(name, `step "${step.id}" has an odd target: ${step.target}`).toBeTruthy()
      expect(stamped, `step "${step.id}" points at data-tour="${name}", which nothing stamps`).toContain(name)
    }
  })

  it('gives every step a title and a body', () => {
    for (const step of WELCOME_STEPS) {
      expect(step.title.length).toBeGreaterThan(0)
      expect(typeof step.Body).toBe('function')
    }
  })
})
