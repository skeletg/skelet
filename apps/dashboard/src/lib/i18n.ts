import { i18n } from '@lingui/core'
import { compileMessage } from '@lingui/message-utils/compileMessage'

// Admin-UI languages. English is inlined in the bundle by the Lingui macro (stripMessageField
// stays false in vite.config.ts), so it needs no catalog; every other language is a pack
// installed as a backend plugin, fetched here at runtime - which is why installing one never
// rebuilds this app. Catalogs arrive as raw ICU strings and compile in the browser: packs stay
// plain JSON a translator can produce without our toolchain

export const SOURCE_LOCALE = 'en'

// Device-local mirror of the roaming pref, so the login screen - rendered before any session or
// pref store exists - comes up in the language this browser last used
export const LOCALE_STORAGE_KEY = 'skelet.admin.locale'

export type AdminLocale = {
  locale: string
  name: string
  direction: 'ltr' | 'rtl'
  plugin: string
}

i18n.load(SOURCE_LOCALE, {})
i18n.activate(SOURCE_LOCALE)

export { i18n }

// One fetch per session - the installed set only changes on deploy. Failure means an empty
// picker, never a broken shell
let localesPromise: Promise<AdminLocale[]> | null = null
export function fetchAdminLocales(): Promise<AdminLocale[]> {
  localesPromise ??= fetch('/api/admin-ui/i18n/locales')
    .then((res) => (res.ok ? (res.json() as Promise<{ locales: AdminLocale[] }>) : { locales: [] }))
    .then((body) => body.locales ?? [])
    .catch(() => [])
  return localesPromise
}

const loadedCatalogs = new Set<string>([SOURCE_LOCALE])

// Fetch (once), compile, activate, and remember the choice for the next pre-auth boot
export async function activateLocale(locale: string): Promise<void> {
  if (locale !== SOURCE_LOCALE && !loadedCatalogs.has(locale)) {
    const res = await fetch(`/api/admin-ui/i18n/catalog/${encodeURIComponent(locale)}`)
    if (!res.ok) throw new Error(`catalog for ${locale}: HTTP ${res.status}`)
    const raw = (await res.json()) as Record<string, string>
    i18n.load(
      locale,
      Object.fromEntries(Object.entries(raw).map(([id, msg]) => [id, compileMessage(msg)])),
    )
    loadedCatalogs.add(locale)
  }
  i18n.activate(locale)
  document.documentElement.lang = locale
  const info = (await fetchAdminLocales()).find((l) => l.locale === locale)
  document.documentElement.dir = info?.direction ?? 'ltr'
  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  } catch {
    // storage can be unavailable (blocked, insecure context) - the choice just won't survive a reload
  }
}

// Called once from main.tsx - restores this browser's last language before anything renders text.
// A pack uninstalled since last visit 404s; fall back to English rather than dying pre-boot
export function bootLocale(): void {
  let stored: string | null
  try {
    stored = localStorage.getItem(LOCALE_STORAGE_KEY)
  } catch {
    return
  }
  if (stored && stored !== SOURCE_LOCALE) {
    void activateLocale(stored).catch(() => i18n.activate(SOURCE_LOCALE))
  }
}
