import { usePreference } from '@/lib/admin-prefs'
import { type ConfigScopeState } from '@/lib/config-scope'

// The scope a new device starts in
//
// Deliberately a seed, not roaming live state: the live scope has to be readable synchronously so
// the very first GraphQL request carries the right X-Channel-Id and Locale, and roaming it would
// yank another open session's scope out from under whoever is working in it. So this applies once,
// only on a device that has never picked a scope - ScopeDefaultSync does the applying
export const SCOPE_DEFAULT_PREF_KEY = 'ui:defaultScope'

export function useScopeDefaultPref() {
  return usePreference<ConfigScopeState | null>(SCOPE_DEFAULT_PREF_KEY, null)
}
