import { Fragment, useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { ChevronRightIcon, GlobeIcon, SearchIcon, XIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { SettingsContent } from '@/features/settings/SettingsContent'
import { useConfigDrafts } from '@/features/settings/useConfigDrafts'
import { Highlight } from '@/features/settings/Highlight'
import { fieldMatches, labelMatches, searchTerms } from '@/features/settings/search'
import { PaymentAvailability } from '@/features/payments/PaymentAvailability'
import { ShippingCarrierAvailability } from '@/features/shipping/ShippingCarrierAvailability'
import { IdentityEditor } from '@/features/settings/IdentityEditor'
import { LocalesEditor } from '@/features/settings/LocalesEditor'
import { MetadataEditor } from '@/features/settings/MetadataEditor'
import { ChannelLocationsEditor } from '@/features/inventory/ChannelLocationsEditor'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import type { ConfigScopeInput } from '@/graphql/schema.types'

// Hosts every per-channel edit and every registry-driven config section on one route, with the
// sidebar scope switcher deciding what's edited: GLOBAL shows registry sections only,
// since channel editors need a resolved channel, CHANNEL adds the bespoke ones as overrides
// /channels/:id is the read-only view - all edits land here, deep-linkable via ?channelId=

// The selection lives in the URL, not in state - /settings/:sectionSlug[/:groupSlug] for registry sections, /settings/<editor> for the
// channel ones, so every pane is linkable and back walks them The sidebar filter rides ?q= for the same reason

const CHANNEL_EDITOR_KINDS = ['identity', 'locales', 'inventory', 'metadata'] as const

type ChannelEditorKind = (typeof CHANNEL_EDITOR_KINDS)[number]

type ActiveSelection =
  | { kind: 'config'; sectionSlug: string; groupSlug?: string }
  | { kind: ChannelEditorKind }

function isChannelEditorKind(slug: string): slug is ChannelEditorKind {
  return (CHANNEL_EDITOR_KINDS as readonly string[]).includes(slug)
}

function pathFor(selection: ActiveSelection): string {
  if (selection.kind !== 'config') return `/settings/${selection.kind}`
  const { sectionSlug, groupSlug } = selection
  return `/settings/${sectionSlug}${groupSlug ? `/${groupSlug}` : ''}`
}

// The registry provider has to wrap the body rather than live in it - a component can't consume
// a context it renders itself, so the page-level useConfigDrafts would register into nothing
export function SettingsPage() {
  const { contextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  return (
    <SaveRegistryContext.Provider value={contextValue}>
      <SettingsPageBody registry={{ dirtySections, isDirty, saveAll }} />
    </SaveRegistryContext.Provider>
  )
}

type PageRegistry = {
  dirtySections: ReturnType<typeof useSaveRegistry>['dirtySections']
  isDirty: boolean
  saveAll: () => Promise<void>
}

function SettingsPageBody({ registry }: { registry: PageRegistry }) {
  const { scope, setScope, effectiveChannelId, selectedChannel, isChannel } = useConfigScope()
  const permissions = usePermissions()
  const [searchParams, setSearchParams] = useSearchParams()
  const { sectionSlug: routeSection, groupSlug: routeGroup } = useParams()
  const { pathname, search } = useLocation()
  const navigate = useNavigate()

  const filter = searchParams.get('q') ?? ''
  const terms = searchTerms(filter)
  const needle = terms.join(' ')

  function setFilter(next: string) {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('q', next)
    else params.delete('q')
    setSearchParams(params, { replace: true })
  }

  // Deep link from /channels/:id "Open Settings" - strip the param after applying, so a later
  // header scope switch isn't fought by a stale URL
  useEffect(() => {
    const target = searchParams.get('channelId')
    if (!target) return
    const already =
      scope.mode === 'CHANNEL' && scope.channelId === target
    if (!already) {
      setScope({ mode: 'CHANNEL', channelId: target, locale: scope.locale })
    }
    const next = new URLSearchParams(searchParams)
    next.delete('channelId')
    setSearchParams(next, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams.get('channelId')])

  const scopeInput = useMemo<ConfigScopeInput | undefined>(() => {
    if (scope.mode === 'GLOBAL') return undefined
    if (!effectiveChannelId) return undefined
    return {
      kind: 'CHANNEL',
      channelId: effectiveChannelId,
      ...(scope.locale ? { locale: scope.locale } : {}),
    }
  }, [scope.mode, effectiveChannelId, scope.locale])

  const skipQuery = scope.mode === 'CHANNEL' && !effectiveChannelId
  const { data, loading } = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    skip: skipQuery,
  })

  const sectionsForSidebar = useMemo(() => {
    return (data?.configSections ?? [])
      .filter((s) => s.slug !== 'search')
      // seo/facets has its own richer UI under /seo/facets, same deal as search
      .map((s) => (s.slug === 'seo' ? { ...s, groups: s.groups.filter((g) => g.slug !== 'facets') } : s))
      .filter((s) => s.groups.some((g) => g.fields.length > 0))
  }, [data])

  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  const isChannelScope = scope.mode === 'CHANNEL' && isChannel

  // Registry sections win a slug clash - a plugin can register any slug, and its fields would
  // otherwise be unreachable
  const active = useMemo<ActiveSelection | null>(() => {
    if (!routeSection) return null
    const section = sectionsForSidebar.find((s) => s.slug === routeSection)
    if (section) {
      const groupsWithFields = section.groups.filter((g) => g.fields.length > 0)
      const groupSlug =
        groupsWithFields.length > 1 &&
        routeGroup &&
        groupsWithFields.some((g) => g.slug === routeGroup)
          ? routeGroup
          : undefined
      return { kind: 'config', sectionSlug: section.slug, groupSlug }
    }
    if (isChannelScope && isChannelEditorKind(routeSection)) return { kind: routeSection }
    return null
  }, [routeSection, routeGroup, sectionsForSidebar, isChannelScope])

  // Where an unresolvable URL lands - bare /settings, a stale slug, or a channel editor still in
  // the URL after the header flipped back to GLOBAL
  const fallbackPath = useMemo(() => {
    if (isChannelScope) return '/settings/identity'
    if (sectionsForSidebar.length > 0) return `/settings/${sectionsForSidebar[0].slug}`
    return null
  }, [isChannelScope, sectionsForSidebar])

  useEffect(() => {
    if (searchParams.get('channelId')) return // let the deep-link effect above settle first
    if (!active) {
      if (loading || skipQuery || !fallbackPath) return
      navigate({ pathname: fallbackPath, search }, { replace: true })
      return
    }
    const canonical = pathFor(active)
    // Drops a group segment the section doesn't own, so the URL always names the open pane
    if (canonical !== pathname) navigate({ pathname: canonical, search }, { replace: true })
  }, [active, loading, skipQuery, fallbackPath, pathname, search, navigate, searchParams])

  const drafts = useConfigDrafts({ sections: data?.configSections, scopeInput })

  // Both the bespoke channel editors and the config fields stage into this registry, so the header
  // Save is the only Save on the page - see docs/conventions.md "Staged save is THE rule"
  const { dirtySections, isDirty, saveAll } = registry
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const pendingCount = dirtySections.reduce((n, s) => n + s.diff.length, 0)

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  const cantEditGlobal = scope.mode === 'GLOBAL' && !permissions.has('config.manage_defaults')

  const scopeBadge = scope.mode === 'GLOBAL' ? (
    <Badge variant="default" className="gap-1">
      Platform defaults
    </Badge>
  ) : isChannel ? (
    <Badge variant="secondary" className="gap-1">
      {selectedChannel?.name} overrides
    </Badge>
  ) : (
    <Badge variant="outline" className="gap-1">
      No channel selected
    </Badge>
  )

  // Channel-scoped editors that edit columns on the channel itself rather than config fields -
  // slotted into the spot each logically belongs rather than fenced into their own group
  const channelEditorLabel: Record<ChannelEditorKind, string> = {
    identity: 'Identity',
    locales: 'Locales',
    inventory: 'Inventory locations',
    metadata: 'Metadata',
  }
  const salesSectionShown = sectionsForSidebar.some((s) => s.slug === 'sales')

  // The tree narrows with the same matcher the pane uses, so it always shows where the hits are
  function groupHit(section: { label: string }, group: { label: string; fields: Parameters<typeof fieldMatches>[0][] }) {
    if (labelMatches(`${section.label} ${group.label}`, terms)) return true
    return group.fields.some((f) => fieldMatches(f, terms, { section: section.label, group: group.label }))
  }
  const filteredSections = sectionsForSidebar
    .map((s) =>
      terms.length === 0 ? s : { ...s, groups: s.groups.filter((g) => groupHit(s, g)) },
    )
    .filter((s) => s.groups.length > 0)

  // Links, not buttons - middle-click and copy-link work, and the URL is the selection The active row takes the same tinted-pill treatment
  // as the app sidebar rather than a solid block, so the settings rail reads as part of the same nav
  function navLink(
    selection: ActiveSelection,
    label: string,
    isActive: boolean,
    opts?: { dense?: boolean },
  ) {
    const dense = !!opts?.dense
    return (
      <Link
        key={pathFor(selection)}
        to={pathFor(selection)}
        aria-current={isActive ? 'page' : undefined}
        className={cn(
          'flex w-full items-center gap-2 rounded-md px-2.5 text-left transition-colors',
          dense ? 'py-1.5 text-[13px]' : 'py-2 text-sm font-medium',
          isActive
            ? 'bg-sidebar-primary/10 font-medium text-sidebar-primary'
            : dense
              ? 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'
              : 'text-foreground/80 hover:bg-accent/60 hover:text-foreground',
        )}
      >
        <span className="min-w-0 flex-1 truncate">
          {terms.length > 0 ? <Highlight text={label} terms={terms} /> : label}
        </span>
      </Link>
    )
  }

  function renderChannelEditor(kind: ChannelEditorKind) {
    return navLink({ kind }, channelEditorLabel[kind], active?.kind === kind)
  }

  function renderConfigSection(s: (typeof sectionsForSidebar)[number]) {
    const groupsWithFields = s.groups.filter((g) => g.fields.length > 0)
    // While filtering, even a lone group stays nested - the tree's job is to show WHICH group
    // holds the match, and a bare section row would hide that
    const multiGroup =
      groupsWithFields.length > 1 || (terms.length > 0 && groupsWithFields.length > 0)
    if (!multiGroup) {
      const isActive =
        active?.kind === 'config' && active.sectionSlug === s.slug && !active.groupSlug
      return navLink({ kind: 'config', sectionSlug: s.slug }, s.label, isActive)
    }

    const activeInThis = active?.kind === 'config' && active.sectionSlug === s.slug
    // A search hit inside a section opens it, so matches are visible without a click
    const isOpen = activeInThis || !!expanded[s.slug] || terms.length > 0
    const listId = `settings-nav-${s.slug}`
    return (
      <div>
        <button
          type="button"
          aria-expanded={isOpen}
          aria-controls={listId}
          onClick={() => setExpanded((prev) => ({ ...prev, [s.slug]: !isOpen }))}
          className={cn(
            'flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm font-medium transition-colors',
            activeInThis ? 'text-foreground' : 'text-foreground/80',
            'hover:bg-accent/60 hover:text-foreground',
          )}
        >
          <span className="min-w-0 flex-1 truncate">
            {terms.length > 0 ? <Highlight text={s.label} terms={terms} /> : s.label}
          </span>
          <ChevronRightIcon
            className={cn(
              'size-3.5 shrink-0 text-muted-foreground/70 transition-transform duration-200 motion-reduce:transition-none',
              isOpen && 'rotate-90',
            )}
            aria-hidden
          />
        </button>
        {/* 0fr → 1fr animates height with no measuring and no keyframes - the same trick the
            dashboard header uses for its back button - and `inert` keeps collapsed links out of
            the tab order while they're still in the DOM */}
        <div
          id={listId}
          inert={!isOpen}
          className={cn(
            'grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none',
            isOpen ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
          )}
        >
          <div className="overflow-hidden">
            <div className="ml-3 mt-0.5 grid gap-0.5 border-l border-border pl-2.5">
              {groupsWithFields.map((g) => {
                const isActive =
                  active?.kind === 'config' &&
                  active.sectionSlug === s.slug &&
                  active.groupSlug === g.slug
                return navLink(
                  { kind: 'config', sectionSlug: s.slug, groupSlug: g.slug },
                  g.label,
                  isActive,
                  { dense: true },
                )
              })}
            </div>
          </div>
        </div>
      </div>
    )
  }

  // Eyebrow + title name the open pane - the sidebar says where you are, the header says what
  // you're editing, and neither repeats the other
  const paneHeading = ((): { eyebrow: string; title: string } => {
    if (needle) return { eyebrow: 'Filter', title: `Settings matching “${filter.trim()}”` }
    if (!active) return { eyebrow: 'Settings', title: 'Settings' }
    if (active.kind !== 'config') {
      return { eyebrow: selectedChannel?.name ?? 'Channel', title: channelEditorLabel[active.kind] }
    }
    const section = sectionsForSidebar.find((s) => s.slug === active.sectionSlug)
    const group = section?.groups.find((g) => g.slug === active.groupSlug)
    const single = section?.groups.filter((g) => g.fields.length > 0)
    return {
      eyebrow: section?.label ?? 'Settings',
      title: group?.label ?? (single?.length === 1 ? single[0].label : section?.label) ?? 'Settings',
    }
  })()

  // No locale in either line - settings resolve at the platform or the channel, never per locale
  const scopeLine =
    scope.mode === 'GLOBAL'
      ? 'Platform defaults - channels without an override inherit these'
      : isChannel
        ? `Overrides for ${selectedChannel?.name} - anything unset falls back to the platform defaults`
        : 'Pick a channel in the header to load its overrides'

  function renderBody() {
    if (scope.mode === 'CHANNEL' && !isChannel) {
      return (
        <Alert className="m-6">
          <AlertTitle>Pick a channel</AlertTitle>
          <AlertDescription>
            Choose one in the header scope picker. Its settings then load as overrides on top of
            the platform defaults.
          </AlertDescription>
        </Alert>
      )
    }
    // A filter looks across every section, so it takes over the pane
    if (needle) return <SettingsContent filter={filter} drafts={drafts} />
    if (!active) {
      if (loading || fallbackPath) return null // a rewrite is pending - don't flash all sections
      return <SettingsContent drafts={drafts} /> // nothing registered - it has the empty-state copy
    }
    if (active.kind === 'identity' && effectiveChannelId) {
      return <IdentityEditor channelId={effectiveChannelId} />
    }
    if (active.kind === 'locales' && effectiveChannelId) {
      return <LocalesEditor channelId={effectiveChannelId} />
    }
    if (active.kind === 'inventory' && effectiveChannelId) {
      return <ChannelLocationsEditor channelId={effectiveChannelId} />
    }
    if (active.kind === 'metadata' && effectiveChannelId) {
      return <MetadataEditor channelId={effectiveChannelId} />
    }
    const sectionSlug = active.kind === 'config' ? active.sectionSlug : undefined
    const groupSlug = active.kind === 'config' ? active.groupSlug : undefined
    return (
      <>
        {/* Per-channel availability lives in payment_methods / shipping_carriers rather than
            config fields, so it's surfaced inline above each section's own fields */}
        {sectionSlug === 'payment' && <PaymentAvailability providerSlug={groupSlug} />}
        {sectionSlug === 'shipping' && <ShippingCarrierAvailability carrierSlug={groupSlug} />}
        <SettingsContent
          activeSectionSlug={sectionSlug}
          activeGroupSlug={groupSlug}
          drafts={drafts}
        />
      </>
    )
  }

  const saveScopeLabel = isChannel
    ? `Channel · ${selectedChannel?.name ?? effectiveChannelId}`
    : 'Platform defaults'

  return (
    <DetailPageLayout
      toc={false} // the section sidebar IS this page's index
      leftSidebar={
        <aside className="flex w-80 shrink-0 flex-col border-r bg-muted/20">
          <div className="border-b p-3">
            {/* Its own form, so the browser can't pair this box with a SECRET field's password
                input elsewhere on the page and autofill it with the saved username - sections
                like AI render one, and the filter would silently take over the pane */}
            <form
              className="relative"
              role="search"
              autoComplete="off"
              onSubmit={(e) => e.preventDefault()}
            >
              <SearchIcon
                className="pointer-events-none absolute left-2.5 top-2.5 size-3.5 text-muted-foreground"
                aria-hidden
              />
              <Input
                type="search"
                name="settings-filter"
                autoComplete="off"
                data-1p-ignore
                data-lpignore="true"
                data-form-type="other"
                value={filter}
                // Last line of defence behind the attributes above, which are only browser heuristics: an automatic credential fill writes
                // without focusing the box, while typing, pasting and picking a suggestion all focus it first
                onChange={(e) => {
                  if (document.activeElement !== e.currentTarget) return
                  setFilter(e.target.value)
                }}
                placeholder="Filter settings"
                aria-label="Filter settings"
                className="h-9 pl-8 pr-8 text-sm [&::-webkit-search-cancel-button]:hidden"
              />
              {filter && (
                <button
                  type="button"
                  onClick={() => setFilter('')}
                  aria-label="Clear filter"
                  className="absolute right-1.5 top-1.5 rounded-sm p-1 text-muted-foreground hover:text-foreground"
                >
                  <XIcon className="size-3.5" />
                </button>
              )}
            </form>
          </div>
          <nav className="flex-1 space-y-0.5 overflow-y-auto p-3 pt-2">
            {/* Leads the list - the most-common first edit after creating a channel */}
            {isChannelScope && !needle && renderChannelEditor('identity')}
            {isChannelScope && !needle && renderChannelEditor('locales')}

            {filteredSections.length === 0 ? (
              <p className="px-3 py-2 text-xs text-muted-foreground">
                {needle ? `Nothing matches “${filter.trim()}”` : 'No settings registered yet.'}
              </p>
            ) : null}

            {/* Registry sections, with the remaining channel editors slotted next to their kin -
                inventory locations right after Sales, which holds the stock policy knobs */}
            {filteredSections.map((s) => (
              <Fragment key={s.slug}>
                {renderConfigSection(s)}
                {isChannelScope && !needle && s.slug === 'sales' && renderChannelEditor('inventory')}
              </Fragment>
            ))}

            {/* Fallbacks, so a channel editor still has a home when its neighbour registered no fields */}
            {isChannelScope && !needle && !salesSectionShown && renderChannelEditor('inventory')}
            {/* Free-form metadata is advanced - keep it last */}
            {isChannelScope && !needle && renderChannelEditor('metadata')}

            {scope.mode === 'CHANNEL' && !isChannel && (
              <div className="mt-3 flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-xs text-muted-foreground/60">
                <GlobeIcon className="size-4 shrink-0" aria-hidden />
                <span className="flex-1">Pick a channel to enable channel sections</span>
              </div>
            )}
          </nav>
        </aside>
      }
    >
      <DetailPageHeader
        title={
          <span className="flex min-w-0 flex-col">
            <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
              {paneHeading.eyebrow}
            </span>
            <span className="truncate">{paneHeading.title}</span>
          </span>
        }
        badges={scopeBadge}
        meta={<span className="text-xs text-muted-foreground">{scopeLine}</span>}
        actions={
          <Button
            type="button"
            size="sm"
            disabled={!isDirty || savingAll}
            onClick={() => { setSaveError(null); setConfirmingSave(true) }}
          >
            {savingAll
              ? 'Saving…'
              : pendingCount > 0
                ? `Save ${pendingCount} change${pendingCount === 1 ? '' : 's'}`
                : 'No changes'}
          </Button>
        }
      />

      <DetailPageBody className="divide-y-0">
        {cantEditGlobal && (
          <Alert className="m-6 mb-0">
            <AlertTitle>Read-only</AlertTitle>
            <AlertDescription>
              You don't have the <code>config.manage_defaults</code> permission, so platform
              defaults are visible here but not editable. Switch the header scope to a channel you
              administer to make changes.
            </AlertDescription>
          </Alert>
        )}

        {renderBody()}
      </DetailPageBody>

      {/* Lists every staged change - config fields and channel editors alike */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={saveScopeLabel}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
  )
}
