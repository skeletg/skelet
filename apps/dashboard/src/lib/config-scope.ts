import { createContext, useContext } from 'react'
import type { ConfigScopeInput } from '@/graphql/schema.types'
import type { AllowedChannelScopes } from '@/shared/auth/usePermissions'

export const STORAGE_KEY = 'skelet-dash:config-scope'

export type ConfigScopeMode = 'GLOBAL' | 'CHANNEL'

export type ConfigScopeState = {
  mode: ConfigScopeMode
  channelId: string
  locale: string
}

export type ChannelScopeOption = {
  id: string
  name: string
  defaultLocale: string
  locales: Array<{ code: string }>
  isActive: boolean
}

export type ConfigScopeContextValue = {
  scope: ConfigScopeState
  /** Respects registered unsaved-changes guards - false when the user cancelled the discard prompt */
  setScope: (next: ConfigScopeState) => boolean
  scopeInput: ConfigScopeInput | undefined
  channels: ChannelScopeOption[]
  selectedChannel: ChannelScopeOption | null
  availableLocales: string[]
  loadingChannels: boolean
  isGlobal: boolean
  isChannel: boolean
  /** Explicit `scope.locale`, else the channel default, else '' so the backend falls back - pages must read this, never recompute it */
  effectiveLocale: string
  /** undefined in GLOBAL mode - mutations that REQUIRE a store gate behind `isChannel`, see `useScopeRequirement` */
  effectiveChannelId: string | undefined
  /** Final fallback when neither a scope locale nor an entity `contentLocale` exists, e.g. first paint of a Create screen - empty while the config query loads */
  globalDefaultLocale: string
  /** Grants for this page are channel-scoped, so there's no GLOBAL scope to sit in - the header disables Global and auto-snaps into a permitted channel */
  restrictedToChannels: boolean
}

export const DEFAULT_SCOPE: ConfigScopeState = {
  mode: 'GLOBAL',
  channelId: '',
  locale: '',
}

export const ConfigScopeContext = createContext<ConfigScopeContextValue | null>(null)

// Derived from the default-marked channel's `defaultLocale` column - canonical since
// the settings pass retired the `general/locale/default` knob, and free of a round-trip

// The unsaved-changes guard lives in lib/unsaved-changes - re-exported here so the callers that
// found it on this module keep their import path
export {
  confirmScopeSwitch,
  useUnsavedChangesGuard,
} from '@/lib/unsaved-changes'

// ---------- Provider ----------

export function filterChannelsByAllowedScopes<T extends ChannelScopeOption>(
  channels: T[],
  allowedScopes: AllowedChannelScopes,
) {
  if (allowedScopes.allChannels) return channels.filter((channel) => channel.isActive)
  const allowedIds = new Set(allowedScopes.channelScopes.map((scope) => scope.channelId))
  return channels.filter((channel) => channel.isActive && allowedIds.has(channel.id))
}

export function localesForAllowedChannel(
  channel: ChannelScopeOption | null | undefined,
  allowedScopes: AllowedChannelScopes,
): string[] {
  if (!channel) return []
  const codes = channel.locales.map((entry) => entry.code)
  const storeLocales = codes.length ? codes : [channel.defaultLocale]
  if (allowedScopes.allChannels) return storeLocales
  const allowed = allowedScopes.channelScopes.find((scope) => scope.channelId === channel.id)
  const locales = allowed?.locales ?? []
  if (locales.length === 0) return storeLocales
  return storeLocales.filter((locale) => locales.includes(locale))
}

// Whether this device already had a scope when the app booted - read at import time on purpose, because the provider writes the key during
// its own first render, so asking any later always says yes and the roaming default would never get its chance
export const hadStoredScope = (() => {
  try {
    return localStorage.getItem(STORAGE_KEY) !== null
  } catch {
    return false
  }
})()

export function readStoredScope(): ConfigScopeState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return DEFAULT_SCOPE
    const parsed = JSON.parse(raw) as Partial<ConfigScopeState>
    if (parsed.mode !== 'GLOBAL' && parsed.mode !== 'CHANNEL') return DEFAULT_SCOPE
    return {
      mode: parsed.mode,
      channelId: typeof parsed.channelId === 'string' ? parsed.channelId : '',
      locale: typeof parsed.locale === 'string' ? parsed.locale : '',
    }
  } catch {
    return DEFAULT_SCOPE
  }
}

export function toScopeInput(scope: ConfigScopeState): ConfigScopeInput | undefined {
  if (scope.mode === 'GLOBAL') return undefined

  return {
    kind: 'CHANNEL',
    channelId: scope.channelId,
    ...(scope.locale ? { locale: scope.locale } : {}),
  }
}

export function useConfigScope() {
  const context = useContext(ConfigScopeContext)
  if (!context) throw new Error('useConfigScope must be used within ConfigScopeProvider')
  return context
}

/**
 * Gates a write action on the header scope - put it on Save buttons AND in the
 * mutation callback, so a write can never run when the scope can't satisfy it
 */
export function useScopeRequirement(opts: {
  requiresChannel?: boolean
  requiresLocale?: boolean
  globalOnly?: boolean
}): { ok: boolean; reason: string | null } {
  const { isGlobal, isChannel, effectiveLocale } = useConfigScope()
  if (opts.globalOnly && !isGlobal) {
    return { ok: false, reason: 'This entity is global. Switch the header to GLOBAL to edit.' }
  }
  if (opts.requiresChannel && !isChannel) {
    return { ok: false, reason: 'Pick a channel in the header first.' }
  }
  if (opts.requiresLocale && !effectiveLocale) {
    return { ok: false, reason: 'Pick a locale in the header first.' }
  }
  return { ok: true, reason: null }
}
