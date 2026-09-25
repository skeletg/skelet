import { useMemo } from 'react'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import {
  PRODUCT_CHANNEL_URL_LABELS,
  channelUrlChanges,
  productChannelUrlsDiff,
  productChannelUrlsSchema,
  toProductChannelUrlValues,
} from '../schemas/product'
import { useProductListingWrite } from './productListing'

// Per-store URL override editor, shown inside the SEO section - A product can have a different URL
// path on a specific store; blank inherits the product's own URL. The override rides on the store
// listing (there's no standalone per-channel URL mutation) - the listing writer keeps its published
// and price values as they are

type ListingLite = {
  url?: string | null
  channel: { id: string; name: string }
}

export function ChannelUrlOverridesEditor({
  productId,
  listings,
  canManage,
}: {
  productId: string
  listings: ListingLite[]
  canManage: boolean
}) {
  const writeListing = useProductListingWrite(productId)
  const baseline = useMemo(() => toProductChannelUrlValues(listings), [listings])
  const diff = useMemo(() => productChannelUrlsDiff((id) => listings.find((l) => l.channel.id === id)?.channel.name ?? 'Store'), [listings])
  const { form, draft } = useSectionForm({
    id: 'channel-urls',
    title: 'Per-store URL overrides',
    baseline,
    schema: productChannelUrlsSchema,
    diff,
    routeKey: `product-channel-urls:${productId}`,
    submit: async (v) => {
      for (const { channelId, url } of channelUrlChanges(baseline, v)) await writeListing(channelId, { url })
    },
  })

  return (
    <div className="grid gap-3 rounded-md border p-4">
      <div>
        <div className="text-sm font-medium">Per-store URL overrides</div>
        <p className="text-xs text-muted-foreground">
          Optional. Give this product a different URL path on a specific store. Blank inherits the product&rsquo;s own URL set above.
        </p>
      </div>
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_CHANNEL_URL_LABELS} />}
      {listings.length === 0 ? (
        <p className="rounded-md border border-dashed p-4 text-center text-xs text-muted-foreground">
          Not listed on any store yet. Publish this product on a store to set a per-store URL.
        </p>
      ) : (
        <fieldset disabled={!canManage} className="grid gap-2">
          {listings.map((l) => (
            <form.AppField key={l.channel.id} name={`urls.${l.channel.id}`}>
              {(f) => (
                <f.TextField
                  id={`churl-${l.channel.id}`}
                  label={l.channel.name}
                  placeholder="inherit product URL"
                  className="grid-cols-[minmax(7rem,1fr)_2fr] items-center gap-3 [&_input]:h-8 [&_input]:font-mono [&_input]:text-xs [&_label]:truncate [&_label]:font-normal [&_label]:text-muted-foreground"
                />
              )}
            </form.AppField>
          ))}
        </fieldset>
      )}
    </div>
  )
}
