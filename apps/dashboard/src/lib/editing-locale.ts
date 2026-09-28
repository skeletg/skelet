import { useConfigScope } from '@/lib/config-scope'

// The locale a translatable record is being edited in - the header scope's locale, else the
// selected channel's default, else the global default. Was the same ternary in nine pages
export function useEditingLocale(): string {
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  return scope.mode === 'CHANNEL'
    ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
    : globalDefaultLocale
}
