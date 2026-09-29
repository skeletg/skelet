import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@apollo/client/react'
import { CheckIcon, ChevronDownIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { usePermissions } from '@/shared/auth/usePermissions'
import { apolloClient } from '@/lib/apollo'
import { setEffectiveLocale } from '@/lib/locale-store'
import { setEffectiveChannelId } from '@/lib/channel-store'
import {
  ConfigScopeContext,
  DEFAULT_SCOPE,
  STORAGE_KEY,
  confirmScopeSwitch,
  filterChannelsByAllowedScopes,
  localesForAllowedChannel,
  readStoredScope,
  toScopeInput,
  useConfigScope,
  type ConfigScopeMode,
  type ConfigScopeState,
} from '@/lib/config-scope'

// Split from config-scope.tsx so that file carries only non-component exports and this one only components - React Fast
// Refresh needs the separation, and the 90+ importers of the hooks keep their import path

export function ConfigScopeProvider({
  children,
  permissionSlug,
}: {
  children: ReactNode
  permissionSlug?: string
}) {
  const [scope, setScopeState] = useState<ConfigScopeState>(() => readStoredScope())
  const channelsQuery = useQuery(ChannelsDocument)
  const globalDefaultLocale = useMemo(() => {
    const rows = channelsQuery.data?.channels
    // Still loading - empty, so a Create screen waits instead of writing under a guess
    if (!rows) return ''
    // With no channel flagged default the backend falls back to "en" (CatalogDefaultLocaleProvider), and create
    // insists on the default locale - mirror it, or every global Create fails with "locale is required"
    return rows.find((sv) => sv.isDefault)?.defaultLocale || 'en'
  }, [channelsQuery.data])
  const permissions = usePermissions()
  // Memoized so the fresh object literal doesn't invalidate every downstream
  // useMemo on each render
  const allowedScopes = useMemo(
    () =>
      permissionSlug
        ? permissions.allowedChannelScopes(permissionSlug)
        : { allChannels: true as const, channelScopes: [] },
    [permissionSlug, permissions],
  )
  const allowedByChannel = useMemo(() => {
    if (allowedScopes.allChannels) return null
    return new Map(allowedScopes.channelScopes.map((entry) => [entry.channelId, entry.locales ?? []]))
  }, [allowedScopes])

  const channels = useMemo(
    () => filterChannelsByAllowedScopes(channelsQuery.data?.channels ?? [], allowedScopes),
    [allowedScopes, channelsQuery.data],
  )
  const selectedChannel = useMemo(
    () => channels.find((channel) => channel.id === scope.channelId) ?? null,
    [scope.channelId, channels],
  )
  const visibleLocales = useMemo(
    () => localesForAllowedChannel(selectedChannel, allowedScopes),
    [allowedScopes, selectedChannel],
  )

  const writeScope = useCallback((next: ConfigScopeState) => {
    setScopeState(next)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  }, [])

  const setScope = useCallback(
    (next: ConfigScopeState): boolean => {
      // Skip the prompt for no-op transitions (e.g. provider auto-correction)
      const same =
        next.mode === scope.mode &&
        next.channelId === scope.channelId &&
        next.locale === scope.locale
      if (same) {
        writeScope(next)
        return true
      }
      if (!confirmScopeSwitch()) return false
      writeScope(next)
      return true
    },
    [scope.locale, scope.mode, scope.channelId, writeScope],
  )

  // Auto-correct a persisted scope the current admin can no longer use - role
  // revoked, or the dev switcher swapped identities - Bypasses the unsaved-changes
  // guard, being system-driven cleanup rather than a user action, and applied
  // during render so it converges in one extra pass
  const scopeCorrection = ((): ConfigScopeState | null => {
    if (channels.length === 0) return null

    // A persisted GLOBAL selection from a previous identity would leave the header showing "Global default" with an
    // empty channel field - snap to their first permitted channel instead
    if (scope.mode === 'GLOBAL') {
      if (allowedByChannel !== null) {
        return { mode: 'CHANNEL', channelId: channels[0].id, locale: '' }
      }
      return null
    }

    const nextChannel = selectedChannel ?? channels[0]
    const nextLocales = allowedByChannel?.get(nextChannel.id) ?? []
    const storeLocaleCodes = nextChannel.locales.map((entry) => entry.code)
    const visibleNextLocales = nextLocales.length > 0
      ? nextLocales
      : storeLocaleCodes.length
        ? storeLocaleCodes
        : [nextChannel.defaultLocale]

    if (!selectedChannel || (scope.locale && !visibleNextLocales.includes(scope.locale))) {
      return {
        mode: 'CHANNEL',
        channelId: nextChannel.id,
        locale: scope.locale && visibleNextLocales.includes(scope.locale) ? scope.locale : '',
      }
    }
    return null
  })()
  if (scopeCorrection) writeScope(scopeCorrection)

  const scopeInput = useMemo(() => toScopeInput(scope), [scope])

  const isGlobal = scope.mode === 'GLOBAL'
  const isChannel = scope.mode === 'CHANNEL' && !!selectedChannel
  const effectiveLocale = useMemo(() => {
    if (scope.locale) return scope.locale
    if (selectedChannel) return selectedChannel.defaultLocale
    return ''
  }, [scope.locale, selectedChannel])
  const effectiveChannelId = isChannel ? scope.channelId : undefined

  // The Apollo `localeLink` has no access to hooks, so mirror `effectiveLocale` into
  // the imperative store, then refetch active queries so the new `Locale` header
  // takes effect immediately
  //
  // NOT `resetStore()` - it cancels in-flight requests and refires them, the
  // "duplicate + cancelled request" pattern in DevTools when the picker is clicked mid-load
  const previousLocaleRef = useRef<string | null>(null)
  useEffect(() => {
    setEffectiveLocale(effectiveLocale)
    if (previousLocaleRef.current === null) {
  // Initial mount - the first fetch already carries the current header value
      previousLocaleRef.current = effectiveLocale
      return
    }
    if (previousLocaleRef.current === effectiveLocale) return
    previousLocaleRef.current = effectiveLocale
    apolloClient.refetchQueries({ include: 'active' }).catch(() => {
  // An individual refetch can fail (unauth on a no-longer-allowed store) - it
  // surfaces at the offending useQuery's error boundary, so swallow it here
    })
  }, [effectiveLocale])

  // Channel scope rides the `X-Channel-Id` header rather than query variables, so
  // Apollo won't refire on its own - same shape as the locale effect above
  const previousChannelRef = useRef<string | null>(null)
  useEffect(() => {
    setEffectiveChannelId(effectiveChannelId ?? '')
    if (previousChannelRef.current === null) {
      previousChannelRef.current = effectiveChannelId ?? ''
      return
    }
    if (previousChannelRef.current === (effectiveChannelId ?? '')) return
    previousChannelRef.current = effectiveChannelId ?? ''
    apolloClient.refetchQueries({ include: 'active' }).catch(() => {
    })
  }, [effectiveChannelId])

  const value = useMemo(
    () => ({
      scope,
      setScope,
      scopeInput,
      channels,
      selectedChannel,
      availableLocales: visibleLocales,
      loadingChannels: channelsQuery.loading || permissions.loading,
      isGlobal,
      isChannel,
      effectiveLocale,
      effectiveChannelId,
      globalDefaultLocale,
      restrictedToChannels: allowedByChannel !== null,
    }),
    [
      visibleLocales,
      scope,
      scopeInput,
      selectedChannel,
      setScope,
      channels,
      channelsQuery.loading,
      permissions.loading,
      isGlobal,
      isChannel,
      effectiveLocale,
      effectiveChannelId,
      globalDefaultLocale,
      allowedByChannel,
    ],
  )

  return <ConfigScopeContext.Provider value={value}>{children}</ConfigScopeContext.Provider>
}

// Searchable single-select (shadcn combobox) for the channel and locale pickers - the filter input only appears past a
// handful of options, so small stores keep the plain dropdown feel while a store with dozens of locales gets type-ahead
function ScopeCombobox({
  id,
  value,
  options,
  placeholder,
  disabled,
  className,
  onSelect,
}: {
  id: string
  value: string
  options: Array<{ value: string; label: string }>
  placeholder: string
  disabled?: boolean
  className?: string
  onSelect: (value: string) => void
}) {
  const [open, setOpen] = useState(false)
  const selected = options.find((o) => o.value === value)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn('h-8 justify-between gap-1 px-3 font-normal', className)}
        >
          <span className={cn('truncate', !selected && 'text-muted-foreground')}>
            {selected?.label ?? placeholder}
          </span>
          <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--radix-popover-trigger-width) min-w-40 p-0">
        <Command>
          {options.length > 7 && <CommandInput placeholder="Search…" className="h-8 text-xs" />}
          <CommandList>
            <CommandEmpty>No matches.</CommandEmpty>
            <CommandGroup>
              {options.map((o) => (
                <CommandItem
                  key={o.value}
                  value={o.label}
                  onSelect={() => {
                    onSelect(o.value)
                    setOpen(false)
                  }}
                  className="text-xs"
                >
                  <CheckIcon className={cn('size-3.5', o.value === value ? 'opacity-100' : 'opacity-0')} />
                  {o.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

// The scope/channel/locale selects - stacked for popovers, inline for toolbars - idSuffix keeps
// the label/select ids unique when several copies are mounted at once
export function ConfigScopeFields({
  stacked = false,
  idSuffix,
}: {
  stacked?: boolean
  idSuffix: string
}) {
  const {
    scope,
    setScope,
    channels,
    selectedChannel,
    availableLocales,
    loadingChannels,
    restrictedToChannels,
  } = useConfigScope()

  const trigger = (w: string) => (stacked ? 'w-full text-xs' : `${w} text-xs`)
  const labelCls = stacked ? 'mb-1 block text-xs text-muted-foreground' : 'sr-only'
  const sfx = idSuffix
  return (
      <div
        className={
          stacked
            ? 'flex flex-col gap-3'
            : // nowrap like the header it sits in - wrapping here is what folded the header into two rows
              'flex min-w-0 items-center gap-2'
        }
      >
        <div>
          <Label htmlFor={`cfg-header-scope-kind${sfx}`} className={labelCls}>
            Scope
          </Label>
          <Select
            value={scope.mode}
            onValueChange={(value: ConfigScopeMode) =>
              setScope(
                value === 'GLOBAL'
                  ? DEFAULT_SCOPE
                  : {
                      mode: 'CHANNEL',
                      channelId: scope.channelId || channels[0]?.id || '',
                      locale: '',
                    },
              )
            }
          >
            <SelectTrigger
              id={`cfg-header-scope-kind${sfx}`}
              size="sm"
              className={trigger('w-[145px]')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {/* Channel-restricted admins have no global scope - the provider
                  auto-snaps them into a permitted channel, so this stays unpickable */}
              <SelectItem value="GLOBAL" disabled={restrictedToChannels}>
                Global default
              </SelectItem>
              <SelectItem value="CHANNEL">Channel</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div>
          <Label htmlFor={`cfg-header-channel${sfx}`} className={labelCls}>
            Channel
          </Label>
          <ScopeCombobox
            id={`cfg-header-channel${sfx}`}
            value={scope.channelId}
            options={channels.map((channel) => ({ value: channel.id, label: channel.name }))}
            placeholder={loadingChannels ? 'Loading...' : 'Channel'}
            disabled={scope.mode !== 'CHANNEL' || loadingChannels || channels.length === 0}
            className={trigger('w-[180px]')}
            onSelect={(channelId) => setScope({ ...scope, channelId, locale: '' })}
          />
        </div>

        <div>
          <Label htmlFor={`cfg-header-locale${sfx}`} className={labelCls}>
            Locale
          </Label>
          <ScopeCombobox
            id={`cfg-header-locale${sfx}`}
            value={scope.locale || '__store_default__'}
            options={[
              { value: '__store_default__', label: 'Channel default' },
              ...availableLocales.map((locale) => ({ value: locale, label: locale })),
            ]}
            placeholder="Locale"
            disabled={scope.mode !== 'CHANNEL' || !selectedChannel}
            className={trigger('w-[150px]')}
            onSelect={(value) =>
              setScope({ ...scope, locale: value === '__store_default__' ? '' : value })
            }
          />
        </div>
      </div>
    )
}
