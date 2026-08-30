// Derives the read-only headline and stat strip from a role's flat scope array - shared by
// RolesList and RoleDetail so the two stay numerically consistent and tweaks land in one place

export type ScopeLike = {
  permission: { id: string; slug: string; name: string }
  channelId?: string | null
  locale?: string | null
}

export type RoleSummary = {
  grantCount: number               // total scope rows
  uniquePermissionCount: number    // distinct permissions held
  uniquePermissionSlugs: string[]  // sorted, for headline text
  explicitChannelIds: string[]     // distinct channel ids appearing in scopes
  anyChannel: boolean              // at least one scope has NULL channel
  explicitLocales: string[]        // distinct non-null locales
  anyLocale: boolean
  isFullyGlobal: boolean           // every scope is (NULL, NULL)
}

export function buildRoleSummary(scopes: readonly ScopeLike[]): RoleSummary {
  const uniquePerms = new Map<string, string>() // id → slug
  const channelIds = new Set<string>()
  const locales = new Set<string>()
  let anyChannel = false
  let anyLocale = false
  let fullyGlobal = scopes.length > 0
  for (const s of scopes) {
    uniquePerms.set(s.permission.id, s.permission.slug)
    if (s.channelId) channelIds.add(s.channelId)
    else anyChannel = true
    if (s.locale) locales.add(s.locale)
    else anyLocale = true
    if (s.channelId != null || s.locale != null) fullyGlobal = false
  }
  return {
    grantCount: scopes.length,
    uniquePermissionCount: uniquePerms.size,
    uniquePermissionSlugs: Array.from(uniquePerms.values()).sort(),
    explicitChannelIds: Array.from(channelIds),
    anyChannel,
    explicitLocales: Array.from(locales),
    anyLocale,
    isFullyGlobal: fullyGlobal,
  }
}

// One human-readable sentence describing what the role does, e.g. "Grants `catalog.manage` and
// 7 others on 2 channels" - sized for one card line and truncated by the caller's CSS
export function headlineForRole(summary: RoleSummary, channelNameByID: (id: string) => string): string {
  if (summary.grantCount === 0) return 'No permission grants - role does nothing.'
  if (summary.isFullyGlobal) {
    return `Global access to ${summary.uniquePermissionCount} permission${
      summary.uniquePermissionCount === 1 ? '' : 's'
    }.`
  }
  const lead = summary.uniquePermissionSlugs[0]
  const others = summary.uniquePermissionCount - 1
  const subject =
    others > 0
      ? `\`${lead}\` and ${others} other${others === 1 ? '' : 's'}`
      : `\`${lead}\``
  const channelPart =
    summary.explicitChannelIds.length === 0 && summary.anyChannel
      ? 'on any channel'
      : summary.explicitChannelIds.length === 1
        ? `on ${channelNameByID(summary.explicitChannelIds[0])}`
        : `on ${summary.explicitChannelIds.length} channel${summary.explicitChannelIds.length === 1 ? '' : 's'}`
  const localePart =
    summary.explicitLocales.length === 0
      ? ''
      : summary.explicitLocales.length === 1
        ? ` · locale ${summary.explicitLocales[0]}`
        : ` · ${summary.explicitLocales.length} locales`
  return `Grants ${subject} ${channelPart}${localePart}.`
}
