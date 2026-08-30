import { useEffect, useRef } from 'react'
import { usePrefsLoaded } from '@/lib/admin-prefs'
import { hadStoredScope, useConfigScope } from '@/lib/config-scope'
import { useScopeDefaultPref } from '@/lib/scope-default'

// Applies the roaming default scope on a device that has never picked one - see scope-default.ts
// for why this is a seed rather than live roaming state
export function ScopeDefaultSync() {
  const { setScope, channels } = useConfigScope()
  const [pref] = useScopeDefaultPref()
  const loaded = usePrefsLoaded()
  const applied = useRef(false)

  useEffect(() => {
    if (applied.current || hadStoredScope || !loaded) return
    if (!pref) {
      applied.current = true // nothing stored, so stop watching
      return
    }
    // channels have to be in before a CHANNEL default can be checked, and applying early would
    // just lose a race with the provider's own auto-correction
    if (pref.mode === 'CHANNEL' && channels.length === 0) return
    applied.current = true
    // a default pointing at a channel this admin lost access to is dropped, not snapped elsewhere
    if (pref.mode === 'CHANNEL' && !channels.some((c) => c.id === pref.channelId)) return
    setScope(pref)
  }, [loaded, pref, channels, setScope])

  return null
}
