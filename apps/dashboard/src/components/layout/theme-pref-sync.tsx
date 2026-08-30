import { useThemePrefSync } from '@/lib/theme-pref'

// Mount point for the theme <-> roaming-pref bridge - it has to sit inside AdminPrefsProvider and
// below ThemeProvider, and lib/theme.tsx must stay free of prefs imports or /login would throw
export function ThemePrefSync() {
  useThemePrefSync()
  return null
}
