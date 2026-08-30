import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { LoadError } from '@/shared/errors/LoadError'
import { cn } from '@/lib/utils'
import { ConfigField } from '@/features/settings/ConfigField'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import type { ConfigScopeInput } from '@/graphql/schema.types'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { fieldIsInherited } from '@/features/settings/provenance'
import { fieldMatches, labelMatches, searchTerms } from '@/features/settings/search'
import type { ConfigDrafts } from '@/features/settings/useConfigDrafts'

// Renders the registry-driven config tree (sections → groups → fields) for whatever the header's
// sidebar scope switcher points at: GLOBAL reads and writes the platform defaults (config.manage_defaults),
// CHANNEL the channel's overrides (config.manage on that channel) - the client-side permission check
// is a UX hint, the backend repeats it
// Staged edits live in the page's useConfigDrafts and arrive as props, so moving between panes never
// drops them - the page-header Save applies the lot

export function SettingsContent({
  activeSectionSlug,
  activeGroupSlug,
  includeSearchSection = false,
  filter = '',
  drafts,
}: {
  // Renders only this section, matching the sidebar's selection - every section when omitted
  activeSectionSlug?: string
  // Narrows further to one group inside the active section
  activeGroupSlug?: string
  // `search` has its own richer UI under /search-engine, so it's hidden here unless asked for
  includeSearchSection?: boolean
  // Free-text match against field labels and paths, from the sidebar filter
  filter?: string
  // Staged edits + their actions, owned by the page
  drafts: ConfigDrafts
}) {
  const { scope, effectiveChannelId, selectedChannel, isChannel } = useConfigScope()
  const locale = scope.locale
  const permissions = usePermissions()

  // Build scopeInput from the header scope - GLOBAL → undefined (the resolver maps that to DEFAULT) - CHANNEL → typed shape - CHANNEL
  // without a resolved channelId is held off (skip the query) - the page-level wrapper shows a "pick a channel" banner
  const scopeInput = useMemo<ConfigScopeInput | undefined>(() => {
    if (scope.mode === 'GLOBAL') return undefined
    if (!effectiveChannelId) return undefined
    return {
      kind: 'CHANNEL',
      channelId: effectiveChannelId,
      ...(locale ? { locale } : {}),
    }
  }, [scope.mode, effectiveChannelId, locale])

  // Edit gating is a UX hint; the backend repeats every check GLOBAL needs config.manage_defaults; CHANNEL needs
  // config.manage scoped to (channelId, locale) so a per-channel admin can't edit another channel's knobs
  const canEdit = useMemo(() => {
    if (scope.mode === 'GLOBAL') return permissions.has('config.manage_defaults')
    if (!effectiveChannelId) return false
    return permissions.canAccessChannelScope(
      'config.manage',
      effectiveChannelId,
      locale || undefined,
    )
  }, [scope.mode, effectiveChannelId, locale, permissions])

  const skipQuery = scope.mode === 'CHANNEL' && !effectiveChannelId

  const { data, loading, error, refetch } = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    notifyOnNetworkStatusChange: true,
    skip: skipQuery,
  })

  const terms = searchTerms(filter)
  const needle = terms.join(' ')

  const sections = useMemo(() => {
    const all = (data?.configSections ?? [])
      .filter((s) => includeSearchSection || s.slug !== 'search')
      // seo/facets has its own richer UI under /seo/facets, same deal as search
      .map((s) => (s.slug === 'seo' ? { ...s, groups: s.groups.filter((g) => g.slug !== 'facets') } : s))
    const bySection = activeSectionSlug
      ? all.filter((s) => s.slug === activeSectionSlug)
      : all
    const byGroup = activeGroupSlug
      ? bySection.map((s) => ({ ...s, groups: s.groups.filter((g) => g.slug === activeGroupSlug) }))
      : bySection
    if (terms.length === 0) return byGroup
    // A query searches the whole scope, so it widens past the active pane on purpose
    return (data?.configSections ?? [])
      .filter((s) => includeSearchSection || s.slug !== 'search')
      .map((s) => (s.slug === 'seo' ? { ...s, groups: s.groups.filter((g) => g.slug !== 'facets') } : s))
      .map((s) => ({
        ...s,
        groups: s.groups
          .map((g) => ({
            ...g,
            fields: labelMatches(`${s.label} ${g.label}`, terms)
              ? g.fields
              : g.fields.filter((f) =>
                  fieldMatches(f, terms, { section: s.label, group: g.label }),
                ),
          }))
          .filter((g) => g.fields.length > 0),
      }))
      .filter((s) => s.groups.length > 0)
  }, [data, activeSectionSlug, activeGroupSlug, includeSearchSection, terms])

  const { drafts: staged, onFieldChange, onReset, resettingPath, resetError } = drafts

  if (loading && sections.length === 0) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Loading settings…</p>
  }

  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load configuration"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }

  // A knob is only editable where its value is read - the API refuses the rest, so say so in the pane instead of letting someone stage a
  // write that will bounce Nothing is per-locale, so a locale in the header locks the whole pane rather than a group
  const localeLock =
    scope.mode === 'CHANNEL' && locale
      ? `Settings aren't per-locale - clear the locale in the header to edit ${selectedChannel?.name ?? 'this channel'}`
      : undefined

  function lockReason(scoping: string): string | undefined {
    if (localeLock) return localeLock
    if (scope.mode === 'GLOBAL') return undefined
    if (scoping === 'GLOBAL') return 'Platform-wide - switch the scope to Global to change it'
    return undefined
  }

  const visibleGroups = sections.flatMap((s) => s.groups.filter((g) => g.fields.length > 0))
  // With one group on screen the page header already names it - a second heading would just repeat
  // Filter results always keep both, since a match is meaningless without knowing where it lives
  const showGroupHeadings = visibleGroups.length > 1 || !!needle
  const showSectionHeadings = sections.length > 1 || !!needle
  // Where an inherited value actually came from - with a locale selected, a channel-wide row now
  // resolves (the fallback ladder used to skip it), and calling that "the platform defaults" is wrong
  function inheritedFrom(field: { scope: { kind: string; locale?: string | null } }): string {
    if (scope.mode === 'GLOBAL') return 'the built-in default'
    if (field.scope.kind === 'CHANNEL' && !field.scope.locale) return 'this channel'
    return 'the platform defaults'
  }

  if (visibleGroups.length === 0) {
    return (
      <div className="px-6 py-10">
        {needle ? (
          <p className="text-sm text-muted-foreground">
            Nothing matches “{filter.trim()}”. Try a shorter word, or part of a setting path.
          </p>
        ) : (
          <p className="max-w-prose text-sm text-muted-foreground">
            No settings registered for this section. Plugins and core packages add knobs through{' '}
            <code>config.Register</code>; they appear here automatically.
          </p>
        )}
      </div>
    )
  }

  return (
    <>
      {localeLock && (
        <p className="border-b bg-muted/30 px-6 py-2 text-xs text-muted-foreground">{localeLock}</p>
      )}
      {resetError && (
        <div className="px-6 pt-4">
          <Alert variant="destructive">
            <AlertTitle>Couldn't reset that value</AlertTitle>
            <AlertDescription>{resetError}</AlertDescription>
          </Alert>
        </div>
      )}

      {sections.map((section, sectionIndex) => {
        const groupsWithFields = section.groups.filter((g) => g.fields.length > 0)
        if (groupsWithFields.length === 0) return null
        return (
          // No top padding: the sticky header's own border is the rule above the first block, and
          // padding there would float it below the line as an empty strip
          <section key={section.slug} className="pb-2">
            {showSectionHeadings && (
              <h3 className="px-6 pb-1 pt-4 text-sm font-semibold tracking-tight">
                {section.label}
              </h3>
            )}
            {groupsWithFields.map((group, groupIndex) => {
              // One line for a wholly platform-wide group beats the same sentence on every row
              const locks = group.fields.map((f) => lockReason(f.scoping))
              const groupLock =
                localeLock || locks.length === 0
                  ? undefined
                  : locks.every((l) => l && l === locks[0])
                    ? locks[0]
                    : undefined
              // Whatever sits directly above the first block already ends in a rule - the page
              // header, or a lock banner - so only later blocks and headed ones draw their own
              const headed = showSectionHeadings || showGroupHeadings
              const drawTopRule = !(sectionIndex === 0 && groupIndex === 0) || headed
              return (
              <div key={group.slug}>
                {showGroupHeadings && (
                  <h4 className="px-6 pb-2 pt-5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                    {group.label}
                  </h4>
                )}
                {/* A form with autoComplete off, because browsers honour the form-level signal
                    more consistently than the per-input one - these are store settings, and an
                    autofilled "Support email" stages an edit nobody made
                    Enter is swallowed: saving goes through the page header, never a submit */}
                {groupLock && (
                  <p
                    className={cn(
                      'bg-muted/30 px-6 py-2 text-xs text-muted-foreground',
                      drawTopRule && 'border-t',
                    )}
                  >
                    {groupLock}
                  </p>
                )}
                <form
                  className={cn('divide-y', !groupLock && drawTopRule && 'border-t')}
                  autoComplete="off"
                  onSubmit={(e) => e.preventDefault()}
                >
                  {group.fields.map((field) => (
                    <ConfigField
                      key={field.path}
                      field={field}
                      draft={staged[field.path]}
                      onChange={(value) => onFieldChange(field.path, value)}
                      onReset={() => onReset(field.path)}
                      resetting={resettingPath === field.path}
                      disabled={!canEdit}
                      inherited={fieldIsInherited(field, scope.mode, effectiveChannelId, locale)}
                      scopeLabel={inheritedFrom(field)}
                      lockedReason={groupLock || localeLock ? ' ' : lockReason(field.scoping)}
                      highlight={terms}
                    />
                  ))}
                </form>
              </div>
              )
            })}
          </section>
        )
      })}

      {!canEdit && (
        <p className="px-6 py-4 text-xs text-muted-foreground">
          {scope.mode === 'CHANNEL' && isChannel
            ? `You can view ${selectedChannel?.name ?? 'this channel'}'s settings but not change them.`
            : 'You can view these settings but not change them.'}
        </p>
      )}
    </>
  )
}
