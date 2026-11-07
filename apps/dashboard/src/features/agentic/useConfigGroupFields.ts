import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import { fieldIsInherited } from '@/features/settings/provenance'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { useConfigDrafts, type ConfigSectionData } from '@/features/settings/useConfigDrafts'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import type { Field } from './FieldState'

// paths must be stable (a module constant) - it's a memo dependency
export function useConfigGroupFields(section: string, group: string, paths?: readonly string[]) {
  const { scope, scopeInput, effectiveChannelId, isChannel } = useConfigScope()
  const permissions = usePermissions()
  const locale = scope.locale
  const global = scope.mode === 'GLOBAL'
  const noStore = !global && !effectiveChannelId
  const may = (slug: string, globalSlug = slug) =>
    global ? permissions.has(globalSlug) : !!effectiveChannelId && permissions.canAccessChannelScope(slug, effectiveChannelId, locale || undefined)
  const canView = may('config.view')
  const canEdit = may('config.manage', 'config.manage_defaults')
  const { data, error } = useQuery(ConfigSectionsDocument, { variables: { scope: scopeInput }, skip: noStore || !canView })

  const found = data?.configSections.find((s) => s.slug === section)
  const picked = useMemo<ConfigSectionData[] | undefined>(
    () =>
      found
        ? [
            {
              ...found,
              groups: found.groups.filter((g) => g.slug === group).map((g) => ({ ...g, fields: paths ? g.fields.filter((f) => paths.includes(f.path)) : g.fields })),
            },
          ]
        : undefined,
    [found, group, paths],
  )
  const { drafts, onFieldChange, onReset, resettingPath, resetError } = useConfigDrafts({ sections: picked, scopeInput })

  const fields = new Map<string, Field>()
  for (const f of picked?.[0]?.groups[0]?.fields ?? []) {
    fields.set(f.path, {
      data: f,
      value: drafts[f.path] ?? f.value,
      dirty: drafts[f.path] !== undefined && drafts[f.path] !== f.value,
      set: (v) => onFieldChange(f.path, v === f.value ? undefined : v),
      reset: () => onReset(f.path),
      resetting: resettingPath === f.path,
      inherited: fieldIsInherited(f, scope.mode, effectiveChannelId, locale),
    })
  }

  return {
    fields,
    section: found,
    error,
    resetError,
    noStore,
    canView,
    canEdit,
    permissionsLoading: permissions.loading,
    localeLocked: isChannel && !!locale,
    here: isChannel ? 'for this store' : 'for every store',
    from: (f: ConfigFieldData) => (global ? 'the built-in default' : f.scope.kind === 'CHANNEL' ? 'this store' : 'the setting for every store'),
  }
}
