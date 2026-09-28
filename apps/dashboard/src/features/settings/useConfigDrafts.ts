import { useCallback, useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import {
  ConfigSectionsDocument,
  ResetConfigValueDocument,
  SetConfigValueDocument,
} from '@/features/settings/Settings.gql'
import type { ConfigScopeInput } from '@/graphql/schema.types'
import { useUnsavedChangesGuard } from '@/lib/config-scope'
import { useSaveSection } from '@/shared/saving/saveRegistry'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'

// Staged config edits belong to the PAGE, not the pane - panes come and go as the sidebar moves
// between sections, channel editors and filter results, and edits parked in a pane vanish with it,
// silently, after the header has already counted them
// Everything here is keyed by config path, so a draft outlives whatever pane created it

export type ConfigSectionData = {
  slug: string
  label: string
  groups: { slug: string; label: string; fields: ConfigFieldData[] }[]
}

// What a value reads as in the confirm dialog - a raw 'true' or a select's stored key tells the
// admin nothing about what they're about to change
function displayValue(field: ConfigFieldData, raw: string): string {
  if (field.type === 'BOOL') return raw === 'true' ? 'Enabled' : 'Disabled'
  if (field.type === 'SELECT') return field.options?.find((o) => o.value === raw)?.label ?? raw
  return raw
}

function isValidJson(raw: string): boolean {
  try {
    JSON.parse(raw)
    return true
  } catch {
    return false
  }
}

export type ConfigDrafts = ReturnType<typeof useConfigDrafts>

export function useConfigDrafts({
  sections,
  scopeInput,
}: {
  sections: ConfigSectionData[] | undefined
  scopeInput: ConfigScopeInput | undefined
}) {
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [resettingPath, setResettingPath] = useState<string | null>(null)
  const [resetError, setResetError] = useState<string | null>(null)

  const [setConfigValue] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })
  const [resetConfigValue] = useMutation(ResetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })

  const fieldsByPath = useMemo(() => {
    const out = new Map<string, { field: ConfigFieldData; sectionLabel: string }>()
    for (const section of sections ?? []) {
      for (const group of section.groups) {
        for (const field of group.fields) out.set(field.path, { field, sectionLabel: section.label })
      }
    }
    return out
  }, [sections])

  const onFieldChange = useCallback((path: string, next: string | undefined) => {
    setDrafts((prev) => {
      const nextDrafts = { ...prev }
      if (next === undefined) delete nextDrafts[path]
      else nextDrafts[path] = next
      return nextDrafts
    })
  }, [])

  const onReset = useCallback(
    async (path: string) => {
      setResettingPath(path)
      setResetError(null)
      try {
        await resetConfigValue({ variables: { path, scope: scopeInput } })
        onFieldChange(path, undefined)
      } catch (e) {
        setResetError(e instanceof Error ? e.message : 'Reset failed.')
      } finally {
        setResettingPath(null)
      }
    },
    [onFieldChange, resetConfigValue, scopeInput],
  )

  const pending = useMemo(() => {
    const out: { field: ConfigFieldData; sectionLabel: string; after: string }[] = []
    for (const [path, after] of Object.entries(drafts)) {
      const entry = fieldsByPath.get(path)
      if (!entry) continue
      if (after === entry.field.value) continue
      // A blank secret means "leave the stored value unchanged" - never send ""
      // (that would wipe the configured secret) - Clearing uses the reset button
      if (entry.field.type === 'SECRET' && after.trim() === '') continue
      out.push({ ...entry, after })
    }
    return out
  }, [drafts, fieldsByPath])

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      pending.map(({ field, sectionLabel, after }) => ({
        kind: 'change' as const,
        label: `${sectionLabel} · ${field.label}`,
        before:
          field.type === 'SECRET'
            ? field.isSet
              ? 'configured (hidden)'
              : 'not set'
            : displayValue(field, field.value),
        after: field.type === 'SECRET' ? 'new secret (hidden)' : displayValue(field, after),
      })),
    [pending],
  )

  const save = useCallback(async () => {
    for (const { field, after } of pending) {
      if ((field.type === 'JSON' || field.type === 'SEARCH_ATTRIBUTES') && !isValidJson(after)) {
        throw new Error(`${field.label} isn't valid JSON - fix it and save again.`)
      }
    }
    await Promise.all(
      pending.map(({ field, after }) =>
        setConfigValue({ variables: { path: field.path, value: after, scope: scopeInput } }),
      ),
    )
    setDrafts({})
  }, [pending, scopeInput, setConfigValue])

  useSaveSection({ id: 'config-values', title: 'Settings', diff, save })
  useUnsavedChangesGuard(pending.length > 0, 'Settings')

  return { drafts, onFieldChange, onReset, resettingPath, resetError }
}
