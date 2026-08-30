import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { MeDocument } from './Me.gql'

export type AllowedChannelScopes =
  | { allChannels: true; channelScopes: { channelId: string; locales?: string[] | null }[] }
  | { allChannels: false; channelScopes: { channelId: string; locales?: string[] | null }[] }

// Backed by the `me` query, so with Apollo's cache-first default this is a zero-cost lookup
// Each me.roles[*].scopes[*] row is one grant: NULL channelId means any channel, NULL locale
// any locale, and the broadest grant across stacked roles wins - superusers pass every check
// Manage implies view, so `foo.view` also matches a `foo.manage` grant, NOT the reverse

function effectiveSlugs(slug: string): string[] {
  if (slug.endsWith('.view')) {
    return [slug, slug.slice(0, -'.view'.length) + '.manage']
  }
  return [slug]
}
export function usePermissions() {
  const { data, loading } = useQuery(MeDocument, { fetchPolicy: 'cache-first' })
  const me = data?.me

  return useMemo(
    () => ({
      loading,
      isSuperuser: !!me?.isSuperuser,

      // Capability gates - for per-channel or per-locale decisions use canAccessChannelScope
      has: (slug: string) => {
        if (!me) return false
        if (me.isSuperuser) return true
        const candidates = effectiveSlugs(slug)
        return me.roles.some((role) =>
          role.scopes.some((scope) => candidates.includes(scope.permission.slug)),
        )
      },

      // For the channel picker - {allChannels: true} when a row has BOTH columns null
      allowedChannelScopes: (slug: string): AllowedChannelScopes => {
        if (!me) return { allChannels: false, channelScopes: [] }
        if (me.isSuperuser) return { allChannels: true, channelScopes: [] }

        const candidates = effectiveSlugs(slug)
        const matching = me.roles.flatMap((role) =>
          role.scopes.filter((s) => candidates.includes(s.permission.slug)),
        )
        if (matching.some((s) => s.channelId == null && s.locale == null)) {
          return { allChannels: true, channelScopes: [] }
        }

        // A null-channel scope applies to every channel, shown as a synthetic "*" entry for display
        const byChannel = new Map<string, Set<string> | null>()
        for (const scope of matching) {
          const key = scope.channelId ?? '*'
          if (scope.locale == null) {
            byChannel.set(key, null) // any locale on this channel
            continue
          }
          const existing = byChannel.get(key)
          if (existing === null) continue // already "any locale"
          const next = existing ?? new Set<string>()
          next.add(scope.locale)
          byChannel.set(key, next)
        }

        return {
          allChannels: false,
          channelScopes: Array.from(byChannel.entries())
            .filter(([k]) => k !== '*') // hide the channel-agnostic bucket from the picker
            .map(([channelId, locales]) => ({
              channelId,
              locales: locales === null ? [] : Array.from(locales),
            })),
        }
      },

      // Mirror of the resolver-side gate - a null `locale` argument means locale-agnostic
      canAccessChannelScope: (slug: string, channelId: string, locale?: string | null) => {
        if (!me) return false
        if (me.isSuperuser) return true

        const candidates = effectiveSlugs(slug)
        return me.roles.some((role) =>
          role.scopes.some((scope) => {
            if (!candidates.includes(scope.permission.slug)) return false
            if (scope.channelId != null && scope.channelId !== channelId) return false
            if (scope.locale != null && locale != null && scope.locale !== locale) return false
            return true
          }),
        )
      },
    }),
    [loading, me],
  )
}
