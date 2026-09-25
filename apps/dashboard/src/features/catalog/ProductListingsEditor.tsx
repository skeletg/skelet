import { useMemo } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { EraserIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatCents } from '@/lib/format'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { ClearProductListingDocument, ProductDocument } from './Catalog.gql'
import { useProductListingWrite } from './product-detail/productListing'
import {
  PRODUCT_LISTING_LABELS,
  productListingChanges,
  productListingsDiff,
  productListingsSchema,
  toProductListingValues,
  clearedStore,
} from './schemas/product'

// Per-store price and published flag - one staged section over every store in scope. A store with
// no listing inherits the product's default price and is unpublished; "Clear" stages removing the
// listing, which puts it back to that

type Listing = {
  isPublished: boolean
  price?: number | null
  effectivePrice?: number | null
  effectiveCurrency: string
  channel: { id: string }
}

type ProductForListings = {
  id: string
  listings: Listing[]
}

export function ProductListingsEditor({
  product,
  canManage,
  channelId,
}: {
  product: ProductForListings
  canManage: boolean
  channelId?: string
}) {
  const channelsQuery = useQuery(ChannelsDocument)
  const stores = useMemo(() => {
    const channels = channelsQuery.data?.channels ?? []
    return channelId ? channels.filter((channel) => channel.id === channelId) : channels
  }, [channelId, channelsQuery.data])

  const writeListing = useProductListingWrite(product.id)
  const [clearListing] = useMutation(ClearProductListingDocument, {
    refetchQueries: [{ query: ProductDocument, variables: { id: product.id } }],
  })

  const listingFor = (id: string) => product.listings.find((l) => l.channel.id === id)
  const hasListing = (id: string) => !!listingFor(id)
  const baseline = useMemo(
    () =>
      toProductListingValues(
        stores.map((s) => s.id),
        product.listings,
      ),
    [stores, product.listings],
  )
  const diff = useMemo(
    () =>
      productListingsDiff(
        (id) => stores.find((s) => s.id === id) ?? { name: 'Store', currencyCode: '' },
        (id) => product.listings.some((l) => l.channel.id === id),
      ),
    [stores, product.listings],
  )
  const { form, draft } = useSectionForm({
    id: 'listings',
    title: 'Per-store pricing & publishing',
    baseline,
    schema: productListingsSchema,
    diff,
    routeKey: `product-listings:${product.id}`,
    enabled: !!channelsQuery.data,
    submit: async (v) => {
      for (const change of productListingChanges(baseline, v, hasListing)) {
        if (change.clear) await clearListing({ variables: { productId: product.id, channelId: change.channelId } })
        else await writeListing(change.channelId, { isPublished: change.isPublished, price: change.price })
      }
    },
  })
  const values = useStore(form.store, (s) => s.values.stores)

  if (channelsQuery.loading) {
    return <p className="text-sm text-muted-foreground">Loading stores…</p>
  }
  if (stores.length === 0) {
    return (
      <Alert>
        <AlertTitle>No channels</AlertTitle>
        <AlertDescription>Create at least one channel to manage per-store pricing and publishing.</AlertDescription>
      </Alert>
    )
  }

  // Any edit to a row takes back its staged clear
  const edit = (i: number) => form.setFieldValue(`stores[${i}].clear`, false)

  return (
    <div className="grid gap-3">
      <Alert>
        <AlertTitle className="text-xs uppercase tracking-wider">How pricing works</AlertTitle>
        <AlertDescription className="text-xs">
          Each store either inherits the product's default amount in the store currency or has an explicit store price. Stores marked
          unpublished hide this product on the channel.
          {channelId ? ' Header scope is limiting this table to the selected channel.' : ''}
        </AlertDescription>
      </Alert>
      {draft.restoredAt !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_LISTING_LABELS} />}

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Channel</TableHead>
              <TableHead>Currency</TableHead>
              <TableHead className="w-44">Price</TableHead>
              <TableHead className="w-44">Effective</TableHead>
              <TableHead className="w-24">Published</TableHead>
              {canManage && <TableHead className="w-12 text-right" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {stores.map((store, i) => {
              const row = values[i]
              if (row?.channelId !== store.id) return null
              const existing = listingFor(store.id)
              const dirty = productListingChanges(baseline, { stores: [row] }, hasListing).length > 0
              return (
                <TableRow key={store.id} className={dirty ? 'bg-warning-soft' : undefined}>
                  <TableCell className="font-medium">
                    {store.name}
                    {dirty ? (
                      <Badge variant="outline" className="ml-2 text-[10px] uppercase tracking-wide">
                        {row.clear ? 'pending clear' : 'pending'}
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono font-normal">
                      {store.currencyCode}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="grid gap-1.5">
                      <form.Field name={`stores[${i}].inheritPrice`}>
                        {(f) => (
                          <div className="flex items-center gap-2">
                            <Switch
                              id={`inherit-${store.id}`}
                              checked={f.state.value}
                              onCheckedChange={(v) => {
                                f.handleChange(v)
                                edit(i)
                              }}
                              disabled={!canManage}
                            />
                            <Label htmlFor={`inherit-${store.id}`} className="text-xs">
                              Use default
                            </Label>
                          </div>
                        )}
                      </form.Field>
                      {!row.inheritPrice && (
                        <form.AppField name={`stores[${i}].price`}>
                          {(f) => (
                            <f.TextField
                              aria-label={`${store.name} price`}
                              type="number"
                              step="0.01"
                              min="0"
                              inputMode="decimal"
                              disabled={!canManage}
                              onValueChange={() => edit(i)}
                            />
                          )}
                        </form.AppField>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {existing?.effectivePrice != null ? formatCents(existing.effectivePrice, existing.effectiveCurrency) : '-'}
                  </TableCell>
                  <TableCell>
                    <form.Field name={`stores[${i}].isPublished`}>
                      {(f) => (
                        <Switch
                          aria-label={`${store.name} published`}
                          checked={f.state.value}
                          onCheckedChange={(v) => {
                            f.handleChange(v)
                            edit(i)
                          }}
                          disabled={!canManage}
                        />
                      )}
                    </form.Field>
                  </TableCell>
                  {canManage && (
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        {existing && !row.clear && (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => form.setFieldValue(`stores[${i}]`, clearedStore(store.id))}
                            aria-label={`Clear ${store.name}`}
                            title="Stage a clear - applied on Save (reverts to inherited default + unpublished)"
                          >
                            <EraserIcon className="size-3.5" />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
