import { useCallback, useMemo } from 'react'

// One grant: a permission at an optional channel + locale, where null means "any" - matches
// GraphQL's RolePermissionScopeInput 1:1, so the save payload is just the array
export type Scope = {
  permissionId: string
  permissionSlug: string
  channelId: string | null
  locale: string | null
}

export type PermissionRow = { id: string; slug: string; name: string }
export type GroupRow = { id: string; slug: string; name: string; permissions: PermissionRow[] }

type SetScopes = (update: (prev: Scope[]) => Scope[]) => void

// The grant operations and their derivations over a scope list someone else owns - the role page's
// form (schemas/role) holds the list, its baseline and its dirt; this is what the sidebar, panel and
// popover share, so they never disagree about what's granted where
export function useRoleDraft(scopes: Scope[], setScopes: SetScopes, groups: GroupRow[]) {
  const addScope = useCallback((permissionId: string, permissionSlug: string, channelId: string | null = null, locale: string | null = null) => {
    setScopes((prev) => {
      // No duplicate rows - Same (perm, channel, locale) tuple → no-op
      const exists = prev.some(
        (s) => s.permissionId === permissionId && s.channelId === channelId && s.locale === locale,
      )
      if (exists) return prev
      return [...prev, { permissionId, permissionSlug, channelId, locale }]
    })
  }, [setScopes])

  const removeScopeAt = useCallback((index: number) => {
    setScopes((prev) => prev.filter((_, i) => i !== index))
  }, [setScopes])

  const updateScopeAt = useCallback((index: number, patch: Partial<Pick<Scope, 'channelId' | 'locale'>>) => {
    setScopes((prev) => prev.map((s, i) => (i === index ? { ...s, ...patch } : s)))
  }, [setScopes])

  // ON ensures at least one row exists (any/any), OFF strips every row for the permission -
  // dropping multi-scope grants silently on toggle-off is the deliberate tradeoff
  const togglePermission = useCallback((perm: PermissionRow, granted: boolean) => {
    setScopes((prev) => {
      if (!granted) {
        return prev.filter((s) => s.permissionId !== perm.id)
      }
      const hasAny = prev.some((s) => s.permissionId === perm.id)
      if (hasAny) return prev
      return [...prev, { permissionId: perm.id, permissionSlug: perm.slug, channelId: null, locale: null }]
    })
  }, [setScopes])

  // Additive - skips permissions that already have a row, so a per-channel scope is never widened to global
  const grantAllInGroup = useCallback((group: GroupRow) => {
    setScopes((prev) => {
      const already = new Set(prev.map((s) => s.permissionId))
      const additions: Scope[] = []
      for (const p of group.permissions) {
        if (!already.has(p.id)) {
          additions.push({ permissionId: p.id, permissionSlug: p.slug, channelId: null, locale: null })
        }
      }
      return additions.length === 0 ? prev : [...prev, ...additions]
    })
  }, [setScopes])

  const revokeAllInGroup = useCallback((group: GroupRow) => {
    const ids = new Set(group.permissions.map((p) => p.id))
    setScopes((prev) => prev.filter((s) => !ids.has(s.permissionId)))
  }, [setScopes])

  // Additive and idempotent - appends (channel, locale) to every already-granted permission
  const applyScopeToGroup = useCallback((group: GroupRow, channelId: string | null, locale: string | null) => {
    setScopes((prev) => {
      const grantedIDs = new Set<string>()
      for (const s of prev) {
        if (group.permissions.some((p) => p.id === s.permissionId)) {
          grantedIDs.add(s.permissionId)
        }
      }
      const additions: Scope[] = []
      for (const permID of grantedIDs) {
        const exists = prev.some(
          (s) => s.permissionId === permID && s.channelId === channelId && s.locale === locale,
        )
        if (exists) continue
        const slug = group.permissions.find((p) => p.id === permID)?.slug ?? ''
        additions.push({ permissionId: permID, permissionSlug: slug, channelId, locale })
      }
      return additions.length === 0 ? prev : [...prev, ...additions]
    })
  }, [setScopes])

  // Entry order follows the master `scopes` array, so the editor stays predictable as rows are added
  const scopesByPermission = useMemo(() => {
    const out: Record<string, Scope[]> = {}
    scopes.forEach((s) => {
      ;(out[s.permissionId] ??= []).push(s)
    })
    return out
  }, [scopes])

  // Drives the sidebar "X / Y" badge - counts permissions with at least one row, not rows
  const grantedCountByGroup = useMemo(() => {
    const out: Record<string, number> = {}
    for (const g of groups) {
      let n = 0
      for (const p of g.permissions) {
        if (scopesByPermission[p.id]?.length) n++
      }
      out[g.id] = n
    }
    return out
  }, [groups, scopesByPermission])

  const summary = useMemo(() => {
    const groupsWithGrants: string[] = []
    for (const g of groups) {
      if (grantedCountByGroup[g.id] > 0) groupsWithGrants.push(g.name)
    }
    const channels = new Set<string>()
    const locales = new Set<string>()
    let anyChannel = false
    let anyLocale = false
    for (const s of scopes) {
      if (s.channelId) channels.add(s.channelId)
      else anyChannel = true
      if (s.locale) locales.add(s.locale)
      else anyLocale = true
    }
    return {
      grantCount: scopes.length,
      groupsWithGrants,
      explicitChannels: Array.from(channels),
      anyChannel,
      explicitLocales: Array.from(locales),
      anyLocale,
    }
  }, [scopes, groups, grantedCountByGroup])

  return {
    scopes,
    addScope,
    removeScopeAt,
    updateScopeAt,
    togglePermission,
    grantAllInGroup,
    revokeAllInGroup,
    applyScopeToGroup,
    scopesByPermission,
    grantedCountByGroup,
    summary,
  }
}

export type RoleDraftState = ReturnType<typeof useRoleDraft>
