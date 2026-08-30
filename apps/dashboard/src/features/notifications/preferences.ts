import { usePreference } from '@/lib/admin-prefs'

// What an admin wants to hear about
//
// Muting is enforced on the SERVER (the scope builder reads this key), because the unread badge is
// counted in SQL - a client-only mute would leave the badge counting rows the inbox refuses to
// show. The toast threshold is the opposite: purely local, since a quiet toast still belongs in
// the inbox and still counts
export const MUTED_CATEGORIES_PREF_KEY = 'notif:mutedCategories'
export const TOAST_MIN_SEVERITY_PREF_KEY = 'notif:toastMinSeverity'

// How many newest notifications the bell and the dashboard section each ask for - one number, so
// the two identical queries dedupe into one request instead of two
export const INBOX_PEEK = 8

export type ToastSeverity = 'info' | 'warning' | 'critical'

const SEVERITY_RANK: Record<string, number> = { info: 0, warning: 1, critical: 2 }

export function severityAtLeast(severity: string, min: ToastSeverity): boolean {
  return (SEVERITY_RANK[severity] ?? 0) >= SEVERITY_RANK[min]
}

export const TOAST_THRESHOLDS: { id: ToastSeverity; label: string; hint: string }[] = [
  { id: 'info', label: 'Everything', hint: 'Every notification pops a toast' },
  { id: 'warning', label: 'Warnings up', hint: 'Skip the routine ones' },
  { id: 'critical', label: 'Critical only', hint: 'Just the things that are on fire' },
]

// Only the categories something actually produces - `payment` and `order` are declared backend
// constants nothing posts yet, and a switch that can never do anything is worse than no switch
export const NOTIFICATION_CATEGORIES: { id: string; label: string; hint: string }[] = [
  { id: 'return', label: 'Returns', hint: 'A customer requested a return' },
  { id: 'review', label: 'Reviews', hint: 'A review is waiting on moderation' },
  { id: 'inventory', label: 'Inventory', hint: 'Stock ran low or ran out' },
  { id: 'form', label: 'Form submissions', hint: 'Someone submitted a web form' },
  { id: 'system', label: 'System', hint: 'Failing jobs and other ops trouble' },
]

// Wider than the switch list on purpose - an old row from a retired category still deserves a name
export const CATEGORY_LABEL: Record<string, string> = {
  ...Object.fromEntries(NOTIFICATION_CATEGORIES.map((c) => [c.id, c.label])),
  payment: 'Payment',
  order: 'Order',
}

export function useMutedCategories() {
  const [muted, setMuted] = usePreference<string[]>(MUTED_CATEGORIES_PREF_KEY, [])
  return {
    muted,
    isMuted: (id: string) => muted.includes(id),
    toggle: (id: string) =>
      setMuted((prev) => {
        const next = prev.includes(id) ? prev.filter((m) => m !== id) : [...prev, id]
        return next.length ? next : undefined // nothing muted keeps no row
      }),
    unmuteAll: () => setMuted(undefined),
  }
}

export function useToastMinSeverity() {
  const [min, setMin] = usePreference<ToastSeverity>(TOAST_MIN_SEVERITY_PREF_KEY, 'info')
  return {
    min: SEVERITY_RANK[min] === undefined ? ('info' as ToastSeverity) : min,
    setMin: (next: ToastSeverity) => setMin(next === 'info' ? undefined : next),
  }
}
