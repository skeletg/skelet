import { useEffect, useRef, useState } from 'react'

import type { HealthStatus } from '@/graphql/schema.types'

// Split out of the page because exporting a non-component from a component file
// breaks Fast Refresh - and because the drift rule below is worth testing alone
export type Check = { key: string; label: string; status: HealthStatus; detail: string; hint?: string | null }

// Every detail carrying a relative time re-renders on EVERY poll ("last event 12s
// ago" -> "17s ago"), so comparing raw strings would wash the page five times a
// minute. Strip the drift and what's left is a value someone would want to notice
// Exported for its test - the rule is easy to get subtly wrong
export function meaningful(check: Check): string {
  return `${check.status}|${check.detail}|${check.hint ?? ''}`.replace(/\d+[smhd] ago/g, '')
}

// True for one animation's length after value changes - never on first render, or
// the whole page would flash on load
export function useJustChanged(value: string, ms = 1200): boolean {
  const previous = useRef(value)
  const [changed, setChanged] = useState(false)
  useEffect(() => {
    if (previous.current === value) return
    previous.current = value
    setChanged(true)
    const timer = setTimeout(() => setChanged(false), ms)
    return () => clearTimeout(timer)
  }, [value, ms])
  return changed
}
