import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { HomeIcon, SearchIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  CmsPageDocument,
  DeleteCmsPageChannelListingDocument,
  SetCmsPageChannelListingDocument,
  type CmsPageRowFragment,
} from '../Cms.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'

// CmsPageChannelsSection manages the page's per-channel listings - A page is
// global; toggling Published exposes it on that channel, and the Homepage
// button designates it as that channel's "/" (one per channel - the backend
// demotes any prior homepage) - Homepage requires Published
//
// Built to scale to many channels: a summary line + a search filter +
// published-first sort + a height-capped scroll list, so a deployment with
// dozens of channels stays scannable instead of rendering one giant block
// When the page is published on a channel a small "Set as homepage" toggle
// appears inline; the search box only shows once there are enough channels to
// warrant it - Mirrors blog's PostStoresSection
const CHANNEL_SEARCH_THRESHOLD = 7
const CHANNELS_INITIAL = 10
const CHANNELS_STEP = 15

// The staged state of one channel's listing. `listed` = a listing row should
// exist for this channel; without one the page falls back to global publish
type ListingDraft = { listed: boolean; isPublished: boolean; isHomepage: boolean; seoNoindex: boolean }
const EMPTY_LISTING: ListingDraft = { listed: false, isPublished: false, isHomepage: false, seoNoindex: false }

function seedListingDraft(listings: CmsPageRowFragment['channelListings']): Record<string, ListingDraft> {
  const m: Record<string, ListingDraft> = {}
  for (const l of listings) {
    m[l.channelId] = { listed: true, isPublished: l.isPublished, isHomepage: l.isHomepage, seoNoindex: l.seoNoindex }
  }
  return m
}

function listingStateLabel(s: ListingDraft): string {
  if (!s.listed) return 'Not listed'
  const parts = [s.isPublished ? 'Published' : 'Unpublished']
  if (s.isHomepage) parts.push('homepage')
  return parts.join(', ')
}

export function CmsPageChannelsSection({
  pageId,
  listings,
  canManage,
  refetchVariables,
}: {
  pageId: string
  listings: CmsPageRowFragment['channelListings']
  canManage: boolean
  refetchVariables: { id: string }
}) {
  const { data, loading } = useQuery(ChannelsDocument)
  const refetch = [{ query: CmsPageDocument, variables: refetchVariables }]
  const [setListing] = useMutation(SetCmsPageChannelListingDocument, { refetchQueries: refetch })
  const [deleteListing] = useMutation(DeleteCmsPageChannelListingDocument, { refetchQueries: refetch })
  const [query, setQuery] = useState('')
  const [shown, setShown] = useState(CHANNELS_INITIAL)

  // A new search starts from the top - don't carry a deep "load more" offset
  // into a freshly-filtered, shorter result set
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setShown(CHANNELS_INITIAL)
  }, [query])

  // Staged per-channel listing draft; toggles only touch this - The header Save
  // flushes the changed channels (the dashboard convention - no instant writes)
  const [draft, setDraft] = useState<Record<string, ListingDraft>>(() => seedListingDraft(listings))
  useEffect(() => {
    // Re-seed from the server after a save/refetch (or external change)
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(seedListingDraft(listings))
  }, [listings])

  const serverDraft = useMemo(() => seedListingDraft(listings), [listings])
  const cur = (id: string): ListingDraft => draft[id] ?? EMPTY_LISTING
  const set = (id: string, next: ListingDraft) => setDraft((prev) => ({ ...prev, [id]: next }))

  const active = useMemo(() => (data?.channels ?? []).filter((c) => c.isActive), [data?.channels])
  const isChanged = (id: string) => {
    const a = draft[id] ?? EMPTY_LISTING
    const b = serverDraft[id] ?? EMPTY_LISTING
    return a.listed !== b.listed || a.isPublished !== b.isPublished || a.isHomepage !== b.isHomepage
  }
  const changedChannels = useMemo(
    () => active.filter((c) => isChanged(c.id)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [active, draft, serverDraft],
  )

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      changedChannels.map((c) => ({
        label: c.name,
        before: listingStateLabel(serverDraft[c.id] ?? EMPTY_LISTING),
        after: listingStateLabel(draft[c.id] ?? EMPTY_LISTING),
      })),
    [changedChannels, serverDraft, draft],
  )

  useSaveSection({
    id: 'channels',
    title: 'Channels',
    diff,
    save: async () => {
      // One write per changed channel, sequential - Dropping a listing deletes
      // the row; otherwise upsert the published/homepage/noindex flags
      for (const c of changedChannels) {
        const d = draft[c.id] ?? EMPTY_LISTING
        const s = serverDraft[c.id] ?? EMPTY_LISTING
        if (!d.listed) {
          if (s.listed) await deleteListing({ variables: { pageId, channelId: c.id } })
        } else {
          await setListing({
            variables: { input: { pageId, channelId: c.id, isPublished: d.isPublished, seoNoindex: d.seoNoindex, isHomepage: d.isHomepage } },
          })
        }
      }
    },
  })

  const publishedCount = active.filter((c) => cur(c.id).isPublished).length
  const homepageCount = active.filter((c) => cur(c.id).isHomepage).length

  // Filter by name/slug, then sort published channels to the top (most likely
  // to be managed), then listed, each group alphabetical
  const q = query.trim().toLowerCase()
  const matched = active
    .filter((c) => !q || c.name.toLowerCase().includes(q) || c.slug.toLowerCase().includes(q))
    .sort((a, b) => {
      const rank = (id: string) => (cur(id).isPublished ? 0 : cur(id).listed ? 1 : 2)
      const d = rank(a.id) - rank(b.id)
      return d !== 0 ? d : a.name.localeCompare(b.name)
    })
  // Render only a page at a time so a deployment with hundreds of channels
  // doesn't put hundreds of rows in the DOM; "Load more" reveals the rest
  const visible = matched.slice(0, shown)
  const remaining = matched.length - visible.length

  if (loading) return <p className="text-sm text-muted-foreground">Loading channels…</p>
  if (active.length === 0) return <p className="text-sm text-muted-foreground">No active channels configured.</p>

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant="secondary">
          Published on {publishedCount} of {active.length}
        </Badge>
        {homepageCount > 0 && (
          <Badge variant="secondary" className="gap-1">
            <HomeIcon className="size-3" /> Homepage on {homepageCount}
          </Badge>
        )}
        {changedChannels.length > 0 && (
          <span className="text-amber-600">{changedChannels.length} unsaved · Save to apply</span>
        )}
      </div>

      {active.length >= CHANNEL_SEARCH_THRESHOLD && (
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={`Search ${active.length} channels…`}
            className="h-9 pl-8"
          />
        </div>
      )}

      <ul className="grid max-h-96 gap-1.5 overflow-y-auto pr-1">
        {visible.length === 0 ? (
          <li className="px-1 py-2 text-sm text-muted-foreground">No channels match “{query}”.</li>
        ) : (
          visible.map((c) => {
            const s = cur(c.id)
            const changed = isChanged(c.id)
            return (
              <li
                key={c.id}
                className={cn(
                  'flex items-center gap-3 rounded-md border px-3 py-2',
                  s.isPublished ? 'bg-card' : 'bg-muted/30',
                  changed && 'border-amber-400/70 ring-1 ring-amber-400/40',
                )}
              >
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-1.5 truncate text-sm font-medium">
                    {c.name}
                    {s.isHomepage && (
                      <span className="inline-flex items-center gap-0.5 rounded bg-primary/10 px-1 py-0.5 text-[10px] font-medium text-primary">
                        <HomeIcon className="size-3" /> Home
                      </span>
                    )}
                  </p>
                  <code className="text-xs text-muted-foreground">{c.slug}</code>
                </div>

                {s.isPublished && (
                  <Button
                    type="button"
                    variant={s.isHomepage ? 'secondary' : 'ghost'}
                    size="sm"
                    className="h-8 gap-1.5 px-2"
                    title={s.isHomepage ? 'This channel’s homepage (/)' : 'Set as this channel’s homepage (/)'}
                    disabled={!canManage}
                    onClick={() => set(c.id, { ...s, isHomepage: !s.isHomepage })}
                  >
                    <HomeIcon className={cn('size-4', s.isHomepage ? 'text-primary' : 'text-muted-foreground')} />
                    <span className="text-xs">Homepage</span>
                  </Button>
                )}

                <div className="flex items-center gap-1.5">
                  <Switch
                    id={`pub-${c.id}`}
                    checked={s.isPublished}
                    onCheckedChange={(v) =>
                      set(
                        c.id,
                        v
                          ? { listed: true, isPublished: true, isHomepage: s.isHomepage, seoNoindex: s.seoNoindex }
                          : { ...s, isPublished: false, isHomepage: false },
                      )
                    }
                    disabled={!canManage}
                  />
                  <Label htmlFor={`pub-${c.id}`} className="text-xs">Published</Label>
                </div>

                {s.listed && canManage ? (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8 shrink-0 text-muted-foreground"
                    title="Remove this channel listing (applied on Save)"
                    onClick={() => set(c.id, { ...EMPTY_LISTING, seoNoindex: s.seoNoindex })}
                  >
                    <Trash2Icon className="size-4" />
                  </Button>
                ) : (
                  <span className="size-8 shrink-0" aria-hidden />
                )}
              </li>
            )
          })
        )}
      </ul>

      {(remaining > 0 || shown > CHANNELS_INITIAL) && matched.length > 0 && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span>
            Showing {visible.length} of {matched.length}
          </span>
          {remaining > 0 && (
            <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={() => setShown((n) => n + CHANNELS_STEP)}>
              Load {Math.min(remaining, CHANNELS_STEP)} more
            </Button>
          )}
          {shown > CHANNELS_INITIAL && (
            <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={() => setShown(CHANNELS_INITIAL)}>
              Show fewer
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
