import { useLocalePrefSync } from '@/lib/locale-pref'

// Mount point for the language <-> roaming-pref bridge, beside ThemePrefSync for the same reason:
// it needs AdminPrefsProvider, and lib/i18n must stay prefs-free so /login can use it
export function LocalePrefSync() {
  useLocalePrefSync()
  return null
}
