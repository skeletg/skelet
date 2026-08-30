import { createContext, useContext } from 'react'
import type { WelcomeStep } from './steps'

// Split from WelcomeDialog so that file exports only components - fast refresh needs the separation

// Pure so the gating is testable without mounting anything: a step whose permission this admin
// doesn't hold, or whose feature switch is off, is absent rather than empty
export function visibleSteps(
  steps: WelcomeStep[],
  gates: { has: (slug: string) => boolean; featureEnabled: (flag: string) => boolean },
): WelcomeStep[] {
  return steps.filter((s) => {
    if (s.permission && !gates.has(s.permission)) return false
    if (s.feature && !gates.featureEnabled(s.feature)) return false
    return true
  })
}

export const WelcomeContext = createContext<{ open: () => void } | null>(null)

// Lets the user menu and Preferences restart the tour. Returns null rather than throwing, so a
// caller rendered outside the shell simply has nothing to open
export function useWelcome() {
  return useContext(WelcomeContext)
}
