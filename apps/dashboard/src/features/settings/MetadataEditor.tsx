import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Input } from '@/components/ui/input'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  ChannelDocument,
  ChannelsDocument,
  UpdateChannelDocument,
} from '@/features/channels/Channels.gql'

type MetadataEntry = { key: string; value: string }

// MetadataEditor is the per-channel free-form K/V bucket on the
// channels table - Distinct from config-registry sections because:
// (a) it has no schema - keys are chosen by the operator, and
// (b) values are not part of any policy chain (no GLOBAL fallback,
//     no inheritance) - It's just metadata
//
// Edits are staged locally and applied through the SettingsPage header
// Save + SaveConfirmDialog (the dashboard-wide convention, see
// docs/conventions.md) - Blank keys are dropped at save time

export function MetadataEditor({ channelId }: { channelId: string }) {
  const permissions = usePermissions()
  const canEdit = permissions.canAccessChannelScope('channels.manage', channelId)

  const { data, loading, error, refetch } = useQuery(ChannelDocument, {
    variables: { id: channelId },
    notifyOnNetworkStatusChange: true,
  })
  const channel = data?.channel

  const [entries, setEntries] = useState<MetadataEntry[]>([])

  // Reset staged entries when a different channel loads (state
  // adjustment during render, keyed on channel identity)
  const [prevChannel, setPrevChannel] = useState<typeof channel>(undefined)
  if (channel && channel !== prevChannel) {
    setPrevChannel(channel)
    setEntries(channel.metadata.map((e) => ({ ...e })))
  }

  const [updateChannel] = useMutation(UpdateChannelDocument, {
    refetchQueries: [
      { query: ChannelsDocument },
      { query: ChannelDocument, variables: { id: channelId } },
    ],
  })

  const cleaned = useMemo(
    () =>
      entries
        .map((e) => ({ key: e.key.trim(), value: e.value.trim() }))
        .filter((e) => e.key.length > 0),
    [entries],
  )

  // Per-key diff: added keys show '' → value, removed keys value → '', edited keys old → new - Duplicate keys collapse
  // last-wins, matching what the backend ends up storing
  const diff = useMemo<SectionDiffEntry[]>(() => {
    if (!channel) return []
    const before = new Map(channel.metadata.map((e) => [e.key.trim(), e.value.trim()]))
    const after = new Map(cleaned.map((e) => [e.key, e.value]))
    const out: SectionDiffEntry[] = []
    for (const [key, value] of after) {
      const prev = before.get(key)
      if (prev !== value) out.push({ label: key, before: prev ?? '', after: value })
    }
    for (const [key, value] of before) {
      if (!after.has(key)) out.push({ label: key, before: value, after: '' })
    }
    return out
  }, [channel, cleaned])

  useSaveSection({
    id: 'channel-metadata',
    title: 'Metadata',
    diff,
    save: async () => {
      if (!channel) return
      await updateChannel({
        variables: {
          input: {
            id: channel.id,
            metadata: cleaned,
            name: channel.name,
            defaultLocale: channel.defaultLocale,
            isActive: channel.isActive,
            currencyCode: channel.currencyCode,
            publicUrl: channel.publicUrl,
          },
        },
      })
    },
  })

  function update(idx: number, patch: Partial<MetadataEntry>) {
    setEntries((prev) => prev.map((e, i) => (i === idx ? { ...e, ...patch } : e)))
  }
  function remove(idx: number) {
    setEntries((prev) => prev.filter((_, i) => i !== idx))
  }
  function add() {
    setEntries((prev) => [...prev, { key: '', value: '' }])
  }

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
      description="Free-form key/value pairs. Not exposed to the storefront unless the backend opts in. Apply edits via the header Save button."
    >
      <div className="grid gap-2">
        {entries.length === 0 ? (
          <p className="text-xs text-muted-foreground">No entries.</p>
        ) : (
          <div className="grid gap-2">
            {entries.map((entry, idx) => (
              <div key={idx} className="grid grid-cols-[1fr_2fr_auto] gap-2">
                <Input
                  value={entry.key}
                  onChange={(e) => update(idx, { key: e.target.value })}
                  placeholder="key"
                  disabled={!canEdit}
                />
                <Input
                  value={entry.value}
                  onChange={(e) => update(idx, { value: e.target.value })}
                  placeholder="value"
                  disabled={!canEdit}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => remove(idx)}
                  aria-label="Remove entry"
                  disabled={!canEdit}
                >
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={add}
          className="w-fit"
          disabled={!canEdit}
        >
          <PlusIcon className="size-4" />
          Add entry
        </Button>

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
