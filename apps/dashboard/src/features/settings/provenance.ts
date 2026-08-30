import type { ConfigFieldData } from '@/features/settings/ConfigField'

// Where the value a field currently shows actually lives - the config resolver answers with the
// EFFECTIVE value for the requested scope, so a channel pane is mostly platform defaults wearing
// the channel's clothes; `scope` on each field is what tells the two apart
// The sidebar counts overrides with this and the rows draw their provenance rail from it

type FieldScope = ConfigFieldData['scope']

export function fieldMatchesScope(
  fieldScope: FieldScope,
  mode: 'GLOBAL' | 'CHANNEL',
  channelId: string | undefined,
  locale: string,
) {
  if (mode === 'GLOBAL') {
    return (
      fieldScope.kind === 'DEFAULT' &&
      (locale ? fieldScope.locale === locale : !fieldScope.locale)
    )
  }
  return (
    fieldScope.kind === 'CHANNEL' &&
    fieldScope.channelId === channelId &&
    (locale ? fieldScope.locale === locale : !fieldScope.locale)
  )
}

export function fieldIsInherited(
  field: ConfigFieldData,
  mode: 'GLOBAL' | 'CHANNEL',
  channelId: string | undefined,
  locale: string,
) {
  return field.isDefault || !fieldMatchesScope(field.scope, mode, channelId, locale)
}
