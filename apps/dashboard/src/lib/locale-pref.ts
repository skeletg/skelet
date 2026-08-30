import { useCallback, useEffect, useRef } from 'react'
import { useLingui } from '@lingui/react'
import { usePreference, usePrefsLoaded } from '@/lib/admin-prefs'
import { activateLocale, SOURCE_LOCALE } from '@/lib/i18n'

// Bridges the active UI language to the roaming per-admin pref, same shape as theme-pref: the
// setter touches both sides at once, the sync only ever applies pref -> i18n, gated on what it
// last applied - so a local pick can't bounce off the store and revert itself

export const LOCALE_PREF_KEY = 'ui:locale'

export function useLocalePref(): [string, (locale: string) => void] {
  const { i18n } = useLingui() // re-renders on activate, so the picker tracks the live locale
  const [, setPref] = usePreference<string | null>(LOCALE_PREF_KEY, null)
  const set = useCallback(
    (locale: string) => {
      void activateLocale(locale).catch(() => undefined)
      // 'en' is stored, not deleted - resetting to English has to roam like any other choice
      setPref(locale)
    },
    [setPref],
  )
  return [i18n.locale, set]
}

export function useLocalePrefSync() {
  const { i18n } = useLingui()
  const [pref] = usePreference<string | null>(LOCALE_PREF_KEY, null)
  const loaded = usePrefsLoaded()
  const applied = useRef<string | null>(null)
  useEffect(() => {
    if (!loaded || !pref) return
    if (pref === applied.current) return
    applied.current = pref
    if (pref !== i18n.locale && pref !== SOURCE_LOCALE) {
      void activateLocale(pref).catch(() => undefined) // pack gone since the pref was written - stay in English
    } else if (pref === SOURCE_LOCALE && i18n.locale !== SOURCE_LOCALE) {
      void activateLocale(SOURCE_LOCALE).catch(() => undefined)
    }
  }, [loaded, pref, i18n])
}
