import { useMutation, useQuery } from '@apollo/client/react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { changed } from '@/shared/saving/diff'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { BlogPostDocument, DeleteBlogPostChannelListingDocument, SetBlogPostChannelListingDocument } from '../Blog.gql'

export type Listing = { channelId: string; isPublished: boolean; seoNoindex: boolean }

// Saves the stores' staged noindex. Mounted with the page, under its save registry, so it is there with the panel closed
export function StoreNoindexSave({
  postId,
  listings,
  refetchVariables,
  edits,
  onSaved,
}: {
  postId: string
  listings: Listing[]
  refetchVariables: { id: string; includeUnpublished: boolean }
  edits: Map<string, boolean>
  onSaved: () => void
}) {
  const { data } = useQuery(ChannelsDocument)
  const [setListing] = useMutation(SetBlogPostChannelListingDocument, { refetchQueries: [{ query: BlogPostDocument, variables: refetchVariables }] })
  const byStore = new Map(listings.map((l) => [l.channelId, l]))
  const diff = (data?.channels ?? [])
    .filter((s) => s.isActive)
    .map((sv) => (edits.has(sv.id) ? changed(`Store · ${sv.name} · noindex`, !!byStore.get(sv.id)?.seoNoindex, edits.get(sv.id)) : null))
    .filter((e): e is SectionDiffEntry => e !== null)
  useSaveSection({
    id: 'store-noindex',
    title: 'Available on stores',
    diff,
    save: async () => {
      for (const [channelId, seoNoindex] of edits) {
        const listing = byStore.get(channelId)
        if (!listing || listing.seoNoindex === seoNoindex) continue
        await setListing({ variables: { input: { postId, channelId, isPublished: listing.isPublished, seoNoindex } } })
      }
      onSaved()
    },
  })
  return null
}

// One row per active channel: published acts at once and creates the listing, noindex stages for the header Save
export function PostStores({
  postId,
  listings,
  canManage,
  refetchVariables,
  noindexEdits,
  onNoindex,
}: {
  postId: string
  listings: Listing[]
  canManage: boolean
  refetchVariables: { id: string; locale?: string; includeUnpublished: boolean }
  noindexEdits: Map<string, boolean>
  onNoindex: (next: Map<string, boolean>) => void
}) {
  const { data, loading } = useQuery(ChannelsDocument)
  const stores = data?.channels ?? []
  const refetch = [{ query: BlogPostDocument, variables: refetchVariables }]
  const [setListing, { loading: saving }] = useMutation(SetBlogPostChannelListingDocument, { refetchQueries: refetch })
  const [deleteListing, { loading: deleting }] = useMutation(DeleteBlogPostChannelListingDocument, { refetchQueries: refetch })

  const byStore = new Map(listings.map((l) => [l.channelId, l]))
  const active = stores.filter((s) => s.isActive)

  return (
    <div className="grid gap-2">
      <p className="text-xs text-muted-foreground">The post is global. Switch a store on to publish it there, then optionally hide it from search.</p>
      {loading ? (
        <p className="text-sm text-muted-foreground">Loading stores…</p>
      ) : active.length === 0 ? (
        <p className="text-sm text-muted-foreground">No active channels configured.</p>
      ) : (
        <ul className="grid gap-2">
          {active.map((sv) => {
            const listing = byStore.get(sv.id)
            const enabled = !!listing?.isPublished
            const noindex = !!listing?.seoNoindex
            return (
              <li key={sv.id} className="grid gap-2 rounded-md border px-3 py-2">
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{sv.name}</p>
                    <code className="text-xs text-muted-foreground">{sv.slug}</code>
                  </div>
                  {listing && canManage && (
                    <Button variant="ghost" size="sm" onClick={() => deleteListing({ variables: { postId, channelId: sv.id } })} disabled={deleting}>
                      Remove
                    </Button>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                  <div className="flex items-center gap-1.5">
                    <Switch
                      id={`pub-${sv.id}`}
                      checked={enabled}
                      onCheckedChange={(v) => setListing({ variables: { input: { postId, channelId: sv.id, isPublished: v, seoNoindex: noindex } } })}
                      disabled={!canManage || saving}
                    />
                    <Label htmlFor={`pub-${sv.id}`} className="text-xs">
                      Published
                    </Label>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Switch
                      id={`ni-${sv.id}`}
                      checked={noindexEdits.get(sv.id) ?? noindex}
                      onCheckedChange={(v) => onNoindex(new Map(noindexEdits).set(sv.id, v))}
                      disabled={!canManage || saving || !listing}
                    />
                    <Label htmlFor={`ni-${sv.id}`} className="text-xs">
                      Noindex
                    </Label>
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
