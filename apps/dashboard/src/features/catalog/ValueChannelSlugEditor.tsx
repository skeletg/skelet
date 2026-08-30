import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { slugify } from '@/lib/slug'
import { ChannelsDocument } from '@/features/channels/Channels.gql'

import type { EmbeddedPending } from './AttributeDescriptionEditor'
import {
  AttributeValueChannelSlugsDocument,
  DeleteAttributeValueChannelSlugDocument,
  SetAttributeValueChannelSlugDocument,
} from './Catalog.gql'

// Per-channel filter-URL slug overrides for one selectable AttributeValue - the global slug
// keys ?attr_<attr>=<slug> everywhere without a row, and a row pins channel-specific vocabulary
// Empty input inherits the global - Edits are STAGED: this reports a {dirty, flush} pending to
// the enclosing value row form, whose Save applies them
export function ValueChannelSlugEditor({
  valueId,
  globalSlug,
  canManage,
  onPendingChange,
}: {
  valueId: string
  globalSlug: string
  canManage: boolean
  onPendingChange: (pending: EmbeddedPending) => void
}) {
  const channelsQuery = useQuery(ChannelsDocument)
  const overridesQuery = useQuery(AttributeValueChannelSlugsDocument, {
    variables: { valueId },
  })

  const channels = useMemo(
    () => channelsQuery.data?.channels ?? [],
    [channelsQuery.data],
  )
  const serverByChannel = useMemo(() => {
    const m = new Map<string, string>()
    for (const o of overridesQuery.data?.attributeValueChannelSlugs ?? []) {
      m.set(o.channelId, o.slug)
    }
    return m
  }, [overridesQuery.data])

  // channelId → staged slug - Absent key = untouched (server value)
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  useEffect(() => {
    // Refetch landed - staged edits were applied or are stale
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDrafts({})
  }, [serverByChannel])

  const dirtyChannels = useMemo(
    () =>
      Object.entries(drafts)
        .filter(([id, slug]) => slug.trim() !== (serverByChannel.get(id) ?? ''))
        .map(([id]) => id),
    [drafts, serverByChannel],
  )

  const refetch = [{ query: AttributeValueChannelSlugsDocument, variables: { valueId } }]
  const [setMut] = useMutation(SetAttributeValueChannelSlugDocument, { refetchQueries: refetch })
  const [delMut] = useMutation(DeleteAttributeValueChannelSlugDocument, { refetchQueries: refetch })

  const flushRef = useRef({ drafts, dirtyChannels, serverByChannel })
  useEffect(() => {
    // Keep the flush closure's snapshot fresh on every commit - Declared
    // before the onPendingChange effect so it runs first in each commit
    flushRef.current = { drafts, dirtyChannels, serverByChannel }
  })

  useEffect(() => {
    onPendingChange({
      dirty: dirtyChannels.length > 0,
      flush: async () => {
        const cur = flushRef.current
        for (const channelId of cur.dirtyChannels) {
          const slug = slugify(cur.drafts[channelId] ?? '')
          if (slug === '') {
            if (cur.serverByChannel.has(channelId)) {
              await delMut({ variables: { valueId, channelId } })
            }
          } else {
            await setMut({ variables: { valueId, channelId, slug } })
          }
        }
      },
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dirtyChannels.join(','), valueId])

  if (channels.length <= 1 && serverByChannel.size === 0) {
    // Single-channel installs gain nothing from per-channel slugs -
    // keep the form quiet unless an override already exists
    return null
  }

  return (
    <div className="grid gap-1.5">
      <Label className="text-xs">Channel URL slugs</Label>
      <p className="text-[11px] text-muted-foreground">
        Override the filter-URL slug per channel. Empty = use the global slug
        (<code className="font-mono">{globalSlug}</code>). Applied with this row's Save.
      </p>
      <div className="grid gap-1.5">
        {channels.map((ch) => {
          const value = drafts[ch.id] ?? serverByChannel.get(ch.id) ?? ''
          const dirty = dirtyChannels.includes(ch.id)
          return (
            <div key={ch.id} className="flex flex-wrap items-center gap-2">
              <span className="w-32 truncate text-xs text-muted-foreground" title={ch.name}>
                {ch.name}
                {dirty ? ' *' : ''}
              </span>
              <Input
                className="h-7 w-40 font-mono text-xs"
                value={value}
                placeholder="inherit global"
                disabled={!canManage}
                onChange={(e) => setDrafts((prev) => ({ ...prev, [ch.id]: e.target.value }))}
                onBlur={() =>
                  setDrafts((prev) =>
                    prev[ch.id] === undefined
                      ? prev
                      : { ...prev, [ch.id]: prev[ch.id].trim() === '' ? '' : slugify(prev[ch.id]) },
                  )
                }
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}
