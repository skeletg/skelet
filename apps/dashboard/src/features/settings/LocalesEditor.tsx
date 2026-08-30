import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, removed } from '@/shared/saving/diff'
import {
  LocalesPickerDocument,
  SetChannelLocalesDocument,
} from '@/features/locales/Locales.gql'
import {
  ChannelDocument,
  ChannelsDocument,
  UpdateChannelDocument,
} from '@/features/channels/Channels.gql'

// LocalesEditor combines the supported-locales picker (junction
// `channel_locales`) and the default-locale select - Edits are staged
// locally and applied through the SettingsPage header Save +
// SaveConfirmDialog (the dashboard-wide convention, see docs/conventions.md):
// confirming fires SetChannelLocalesDocument for the supported set and
// UpdateChannelDocument for the default, sequenced so the backend's
// "default must stay supported" guard is never violated mid-save

export function LocalesEditor({ channelId }: { channelId: string }) {
  const permissions = usePermissions()
  const canEdit = permissions.canAccessChannelScope('channels.manage', channelId)

  const channelQuery = useQuery(ChannelDocument, {
    variables: { id: channelId },
    notifyOnNetworkStatusChange: true,
  })
  const localesQuery = useQuery(LocalesPickerDocument)
  const allLocales = useMemo(
    () =>
      [...(localesQuery.data?.locales ?? [])].sort((a, b) => a.code.localeCompare(b.code)),
    [localesQuery.data],
  )

  const channel = channelQuery.data?.channel
  const savedCodes = useMemo(
    () => (channel?.locales ?? []).map((l) => l.code),
    [channel],
  )

  // Staged drafts: reset to server truth whenever the channel payload
  // (re)loads - including the refetch that follows a successful save
  const [stagedCodes, setStagedCodes] = useState<string[]>([])
  const [stagedDefault, setStagedDefault] = useState('')
  const [hint, setHint] = useState<string | null>(null)

  // (State adjustment during render, keyed on channel identity -
  // savedCodes only changes when channel does.)
  const [prevChannel, setPrevChannel] = useState<typeof channel>(undefined)
  if (channel !== prevChannel) {
    setPrevChannel(channel)
    setStagedCodes(savedCodes)
    setStagedDefault(channel?.defaultLocale ?? '')
    setHint(null)
  }

  const refetchQueries = [
    { query: ChannelDocument, variables: { id: channelId } },
    { query: ChannelsDocument },
  ]
  const [setStoreLocales] = useMutation(SetChannelLocalesDocument, {
    refetchQueries,
  })
  const [updateChannel] = useMutation(UpdateChannelDocument, {
    refetchQueries,
  })

  function toggleLocale(code: string) {
    setHint(null)
    if (stagedCodes.includes(code)) {
      // Never let the operator yank the (staged) default locale out of the supported set - the backend would reject the
      // save anyway Surface a hint instead
      if (code === stagedDefault) {
        setHint(
          `${code} is the channel's default locale. Change the default first, then remove this entry.`,
        )
        return
      }
      setStagedCodes((prev) => prev.filter((c) => c !== code))
    } else {
      setStagedCodes((prev) => [...prev, code])
    }
  }

  const codesChanged = useMemo(() => {
    if (!channel) return false
    return [...stagedCodes].sort().join(',') !== [...savedCodes].sort().join(',')
  }, [channel, stagedCodes, savedCodes])
  const defaultChanged = !!channel && stagedDefault !== channel.defaultLocale

  const diff = useMemo<SectionDiffEntry[]>(() => {
    if (!channel) return []
    const out: SectionDiffEntry[] = []
    if (codesChanged) {
      for (const c of [...stagedCodes].sort()) {
        if (!savedCodes.includes(c)) out.push(added(`locale · ${c}`))
      }
      for (const c of [...savedCodes].sort()) {
        if (!stagedCodes.includes(c)) out.push(removed(`locale · ${c}`))
      }
    }
    if (defaultChanged) {
      out.push({ label: 'default locale', before: channel.defaultLocale, after: stagedDefault })
    }
    return out
  }, [channel, codesChanged, defaultChanged, savedCodes, stagedCodes, stagedDefault])

  useSaveSection({
    id: 'channel-locales',
    title: 'Locales',
    diff,
    save: async () => {
      if (!channel) return
      if (stagedDefault && !stagedCodes.includes(stagedDefault)) {
        throw new Error('The default locale must be one of the supported locales.')
      }
      if (codesChanged) {
        // Keep the outgoing default in the set until the new default
        // lands, so the backend never sees an unsupported default
        const interim =
          defaultChanged && !stagedCodes.includes(channel.defaultLocale)
            ? [...stagedCodes, channel.defaultLocale]
            : stagedCodes
        await setStoreLocales({ variables: { channelId: channel.id, codes: interim } })
        if (defaultChanged) {
          await saveDefault()
          if (interim.length !== stagedCodes.length) {
            await setStoreLocales({ variables: { channelId: channel.id, codes: stagedCodes } })
          }
        }
      } else if (defaultChanged) {
        await saveDefault()
      }
    },
  })

  async function saveDefault() {
    if (!channel) return
    await updateChannel({
      variables: {
        input: {
          id: channel.id,
          defaultLocale: stagedDefault,
          name: channel.name,
          isActive: channel.isActive,
          currencyCode: channel.currencyCode,
          publicUrl: channel.publicUrl,
          metadata: channel.metadata,
        },
      },
    })
  }

  if (channelQuery.loading && !channel) {
    return (
      <DetailPageSection description="Loading channel…">
        <div />
      </DetailPageSection>
    )
  }

  if (channelQuery.error || !channel) {
    return (
      <DetailPageSection>
        <LoadError
          title="Couldn't load channel"
          message={channelQuery.error?.message ?? 'Channel not found.'}
          onRetry={() => void channelQuery.refetch()}
          retrying={channelQuery.loading}
        />
      </DetailPageSection>
    )
  }

  return (
    <DetailPageSection
      description="Languages this channel serves. Apply edits via the header Save button."
    >
      <div className="grid gap-4">
        <div className="grid gap-2">
          <Label>Supported locales</Label>
          {localesQuery.loading ? (
            <p className="text-xs text-muted-foreground">Loading languages…</p>
          ) : allLocales.length === 0 ? (
            <Alert>
              <AlertTitle>No active languages defined</AlertTitle>
              <AlertDescription>
                Create at least one language on the Languages page first, then
                assign it here.
              </AlertDescription>
            </Alert>
          ) : (
            <div className="grid gap-2 rounded-md border p-3 sm:grid-cols-2">
              {allLocales.map((entry) => (
                <label
                  key={entry.code}
                  className="flex cursor-pointer items-start gap-2 rounded px-2 py-1 hover:bg-accent/50 has-[:checked]:bg-accent"
                >
                  <Checkbox
                    checked={stagedCodes.includes(entry.code)}
                    onCheckedChange={() => toggleLocale(entry.code)}
                    className="mt-0.5"
                    disabled={!canEdit}
                  />
                  <div className="grid gap-0.5 leading-tight">
                    <div className="flex items-center gap-2 text-sm">
                      <span className="text-base leading-none" aria-hidden>
                        {entry.flagEmoji || '🏳️'}
                      </span>
                      <span className="font-medium">{entry.nativeName || entry.name}</span>
                      <code className="font-mono text-xs text-muted-foreground">
                        ({entry.code})
                      </code>
                    </div>
                    {entry.nativeName && entry.name !== entry.nativeName && (
                      <span className="pl-7 text-xs text-muted-foreground">
                        {entry.name}
                      </span>
                    )}
                  </div>
                </label>
              ))}
            </div>
          )}
        </div>

        <div className="grid gap-2 max-w-sm">
          <Label htmlFor="locales-default">Default locale</Label>
          <Select
            value={stagedCodes.includes(stagedDefault) ? stagedDefault : ''}
            onValueChange={(v) => { setHint(null); setStagedDefault(v) }}
            disabled={stagedCodes.length === 0 || !canEdit}
          >
            <SelectTrigger id="locales-default">
              <SelectValue
                placeholder={
                  stagedCodes.length === 0
                    ? 'Select supported locales first'
                    : 'Pick a default…'
                }
              />
            </SelectTrigger>
            <SelectContent>
              {stagedCodes.map((code) => {
                const entry =
                  allLocales.find((l) => l.code === code) ?? {
                    code,
                    name: code,
                    nativeName: code,
                    flagEmoji: '',
                  }
                return (
                  <SelectItem key={code} value={code}>
                    <span className="mr-1.5" aria-hidden>
                      {entry.flagEmoji || '🏳️'}
                    </span>
                    {entry.nativeName || entry.name}{' '}
                    <span className="text-xs text-muted-foreground">({code})</span>
                  </SelectItem>
                )
              })}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Only locales selected above are eligible.
          </p>
        </div>

        {!canEdit && (
          <Alert>
            <AlertTitle>Read-only</AlertTitle>
            <AlertDescription>
              You don't have channels.manage on this channel.
            </AlertDescription>
          </Alert>
        )}

        {hint && (
          <Alert variant="destructive">
            <AlertTitle>Can't stage that</AlertTitle>
            <AlertDescription>{hint}</AlertDescription>
          </Alert>
        )}
      </div>
    </DetailPageSection>
  )
}
