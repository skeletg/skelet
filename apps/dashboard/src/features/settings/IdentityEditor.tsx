import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { display } from '@/shared/saving/diff'
import {
  ChannelDocument,
  ChannelsDocument,
  UpdateChannelDocument,
} from '@/features/channels/Channels.gql'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'

// IdentityEditor handles the per-channel identity columns that didn't
// fit the config registry model: name, currency code, public URL, and
// the is_active toggle - Slug and default locale are intentionally
// missing - slug is immutable post-create; default locale lives next
// to the supported-locales picker in LocalesEditor
//
// Edits are staged and applied through the SettingsPage header Save +
// SaveConfirmDialog (the dashboard-wide convention, see docs/conventions.md)
//
// Currency edits surface the existing backend guard
// (STORE_CURRENCY_BLOCKED / GLOBAL_CURRENCY_BLOCKED) via the
// friendlyCurrencyError helper so admins get a usable message rather
// than the raw GraphQL extension code

function friendlyCurrencyError(message: string): string {
  if (message.includes('STORE_CURRENCY_BLOCKED')) {
    return 'Some listings inherit price on this channel. Set explicit prices first, then change currency.'
  }
  if (message.includes('GLOBAL_CURRENCY_BLOCKED')) {
    return 'Some inheriting listings would become incoherent under the new global currency. Resolve them first.'
  }
  return message
}

export function IdentityEditor({ channelId }: { channelId: string }) {
  const permissions = usePermissions()
  const canEdit = permissions.canAccessChannelScope('channels.manage', channelId)

  const { data, loading, error, refetch } = useQuery(ChannelDocument, {
    variables: { id: channelId },
    notifyOnNetworkStatusChange: true,
  })
  const channel = data?.channel
  const currenciesQuery = useQuery(CurrenciesPickerDocument)
  // A channel whose currency was archived (or predates the picker) would otherwise render an
  // empty box that silently rewrites the value on save - keep its code selectable
  const currencies = useMemo(() => {
    const list = currenciesQuery.data?.currencies ?? []
    const code = channel?.currencyCode
    if (code && !list.some((c) => c.code === code)) {
      return [...list, { code, name: 'no longer active' }]
    }
    return list
  }, [currenciesQuery.data, channel?.currencyCode])

  const [name, setName] = useState('')
  const [currencyCode, setCurrencyCode] = useState('')
  const [publicUrl, setPublicUrl] = useState('')
  const [isActive, setIsActive] = useState(true)

  // Reset the form when a different channel loads (e.g. the header
  // scope picker switched) - Fold the channel's persisted values into
  // the local state so the inputs reflect saved truth - Applied as a
  // state adjustment during render, keyed on channel identity like the
  // old effect deps
  const [prevChannel, setPrevChannel] = useState<typeof channel>(undefined)
  if (channel && channel !== prevChannel) {
    setPrevChannel(channel)
    setName(channel.name)
    setCurrencyCode(channel.currencyCode)
    setPublicUrl(channel.publicUrl ?? '')
    setIsActive(channel.isActive)
  }

  const [updateChannel] = useMutation(UpdateChannelDocument, {
    refetchQueries: [
      { query: ChannelsDocument },
      { query: ChannelDocument, variables: { id: channelId } },
    ],
  })

  const diff = useMemo<SectionDiffEntry[]>(() => {
    if (!channel) return []
    const out: SectionDiffEntry[] = []
    if (name !== channel.name) out.push({ label: 'name', before: channel.name, after: name })
    if (currencyCode.toUpperCase() !== channel.currencyCode) {
      out.push({ label: 'currency', before: channel.currencyCode, after: currencyCode.toUpperCase() })
    }
    if ((publicUrl || '') !== (channel.publicUrl ?? '')) {
      out.push({ label: 'public URL', before: channel.publicUrl ?? '', after: publicUrl })
    }
    if (isActive !== channel.isActive) {
      out.push({ label: 'active', before: display(channel.isActive), after: display(isActive) })
    }
    return out
  }, [channel, name, currencyCode, publicUrl, isActive])

  useSaveSection({
    id: 'channel-identity',
    title: 'Identity',
    diff,
    save: async () => {
      if (!channel) return
      if (name.trim() === '') throw new Error('Name is required.')
      if (currencyCode.trim().length !== 3) throw new Error('Currency must be a 3-letter ISO code.')
      try {
        await updateChannel({
          variables: {
            input: {
              id: channel.id,
              name,
              defaultLocale: channel.defaultLocale,
              isActive,
              currencyCode: currencyCode.toUpperCase(),
              publicUrl: publicUrl || null,
              metadata: channel.metadata,
            },
          },
        })
      } catch (err) {
        // The friendly message replaces an unreadable backend one - keep the original as the cause
        // so it still reaches the console rather than vanishing
        throw new Error(
          friendlyCurrencyError(err instanceof Error ? err.message : 'Save failed.'),
          { cause: err },
        )
      }
    },
  })

  if (loading && !channel) {
    return (
      <DetailPageSection description="Loading channel…">
        <div />
      </DetailPageSection>
    )
  }

  if (error || !channel) {
    return (
      <DetailPageSection>
        <LoadError
          title="Couldn't load channel"
          message={error?.message ?? 'Channel not found.'}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </DetailPageSection>
    )
  }

  return (
    <DetailPageSection
      description={`Name, currency, URL, and active state for "${channel.name}". Slug (${channel.slug}) is immutable. Apply edits via the header Save button.`}
    >
      <div className="grid gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="identity-name">Name</Label>
            <Input
              id="identity-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              disabled={!canEdit}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="identity-currency">Currency</Label>
            {/* The store's own currencies, same picker the create dialog uses - typing a code
                here only ever produced a CURRENCY_NOT_FOUND at save */}
            <Select
              value={currencyCode}
              onValueChange={setCurrencyCode}
              disabled={!canEdit || currencies.length === 0}
            >
              <SelectTrigger id="identity-currency">
                <SelectValue placeholder="Pick a currency" />
              </SelectTrigger>
              <SelectContent>
                {currencies.map((c) => (
                  <SelectItem key={c.code} value={c.code}>
                    <span className="font-mono text-xs">{c.code}</span> · {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Add currencies under Settings - Currencies. Changing this is blocked when
              listings inherit the channel's price - fix those first.
            </p>
          </div>
          <div className="grid gap-2 sm:col-span-2">
            <Label htmlFor="identity-public-url">Public URL</Label>
            <Input
              id="identity-public-url"
              type="url"
              value={publicUrl}
              onChange={(e) => setPublicUrl(e.target.value)}
              disabled={!canEdit}
              placeholder="https://wheels.example.com"
            />
            <p className="text-xs text-muted-foreground">
              Used to build verification + password-reset links in transactional
              emails, and to match incoming Host headers to this channel.
            </p>
          </div>
        </div>

        <div className="flex items-start justify-between gap-4 rounded-md border p-3">
          <div className="grid gap-0.5">
            <Label htmlFor="identity-active" className="text-sm">
              Active
            </Label>
            <p className="text-xs text-muted-foreground">
              Inactive channels remain readable but the storefront filters
              them out of channel-switcher dropdowns.
            </p>
          </div>
          <Switch
            id="identity-active"
            checked={isActive}
            onCheckedChange={setIsActive}
            disabled={!canEdit}
          />
        </div>

        {!canEdit && (
          <Alert>
            <AlertTitle>Read-only</AlertTitle>
            <AlertDescription>
              You don't have channels.manage on this channel.
            </AlertDescription>
          </Alert>
        )}
      </div>
    </DetailPageSection>
  )
}
