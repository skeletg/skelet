import { useCallback, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { EraserIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatCents } from '@/lib/format'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import {
  ClearProductListingDocument,
  ProductDocument,
  SetProductListingDocument,
} from './Catalog.gql'
import { useProductSaveSection, type SectionDiffEntry } from './productSaveRegistry'

type Listing = {
  isPublished: boolean
  price?: number | null
  url?: string | null
  effectivePrice?: number | null
  effectiveCurrency: string
  channel: { id: string; slug: string; name: string; currencyCode: string }
}

type ProductForListings = {
  id: string
  defaultPrice: number
  listings: Listing[]
}

type RowState = {
  channel: { id: string; slug: string; name: string; currencyCode: string }
  isPublished: boolean
  inheritPrice: boolean
  priceDollars: string
  // Per-channel URL override - Empty string = inherit the product's
  // global url on this channel
  url: string
  // Original values from the loaded product, used for diffing
  originalIsPublished: boolean
  originalInheritPrice: boolean
  originalPriceDollars: string
  originalUrl: string
  hadExistingListing: boolean
  // True when the user has explicitly clicked "Clear" for this row - the row's state is reset to defaults and the global Save will issue a
  // ClearProductListing mutation - Resetting fields by hand also produces a clean state but doesn't issue Clear unless this is true
  clearRequested: boolean
}

function dollarsToCents(input: string): number {
  const n = Number(input)
  if (!Number.isFinite(n)) return 0
  return Math.round(n * 100)
}

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2)
}

export function ProductListingsEditor({
  product,
  canManage,
  channelId,
  productQueryVariables,
}: {
  product: ProductForListings
  canManage: boolean
  channelId?: string
  productQueryVariables?: { id: string; locale?: string }
}) {
  const channelsQuery = useQuery(ChannelsDocument)
  const allChannels = useMemo(() => {
    const channels = channelsQuery.data?.channels ?? []
    return channelId ? channels.filter((channel) => channel.id === channelId) : channels
  }, [channelId, channelsQuery.data])

  // Build a row per known channel - A listing exists in product.listings when the admin has saved at least one explicit
  // value for it; otherwise we render a default row that inherits price + is unpublished
  const initialRows = useMemo<RowState[]>(() => {
    const byId = new Map(product.listings.map((l) => [l.channel.id, l]))
    return allChannels.map((sv) => {
      const existing = byId.get(sv.id)
      const isPublished = existing?.isPublished ?? false
      const inheritPrice = existing ? existing.price == null : true
      const priceDollars = existing?.price != null ? centsToDollars(existing.price) : ''
      const url = existing?.url ?? ''
      return {
        channel: { id: sv.id, slug: sv.slug, name: sv.name, currencyCode: sv.currencyCode },
        isPublished,
        inheritPrice,
        priceDollars,
        url,
        originalIsPublished: isPublished,
        originalInheritPrice: inheritPrice,
        originalPriceDollars: priceDollars,
        originalUrl: url,
        hadExistingListing: !!existing,
        clearRequested: false,
      }
    })
  }, [allChannels, product.listings])

  const [rows, setRows] = useState<RowState[]>(initialRows)
  // Reset row state whenever the product/listings change underneath
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setRows(initialRows)
  }, [initialRows])

  const refetch = [{ query: ProductDocument, variables: productQueryVariables ?? { id: product.id } }]
  const [setListing] = useMutation(SetProductListingDocument, { refetchQueries: refetch })
  const [clearListing] = useMutation(ClearProductListingDocument, { refetchQueries: refetch })

  function patchRow(id: string, patch: Partial<RowState>) {
    setRows((prev) =>
      prev.map((r) => {
        if (r.channel.id !== id) return r
        // Any manual edit cancels a pending clear request
        const next = { ...r, ...patch }
        if (patch.clearRequested === undefined) next.clearRequested = false
        return next
      }),
    )
  }

  function rowIsDirty(r: RowState): boolean {
    if (r.clearRequested) return r.hadExistingListing
    return (
      r.isPublished !== r.originalIsPublished ||
      r.inheritPrice !== r.originalInheritPrice ||
      (!r.inheritPrice && r.priceDollars !== r.originalPriceDollars) ||
      r.url !== r.originalUrl
    )
  }

  function rowDescribePrice(r: RowState, useDraft: boolean): string {
    const inheritPrice = useDraft ? r.inheritPrice : r.originalInheritPrice
    const priceDollars = useDraft ? r.priceDollars : r.originalPriceDollars
    if (inheritPrice) return 'inherit default'
    const cents = dollarsToCents(priceDollars)
    return formatCents(cents, r.channel.currencyCode)
  }

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    for (const r of rows) {
      if (!rowIsDirty(r)) continue
      if (r.clearRequested) {
        entries.push({
          label: r.channel.name,
          before: `${rowDescribePrice(r, false)} · ${r.originalIsPublished ? 'published' : 'unpublished'}`,
          after: 'cleared (revert to default)',
        })
        continue
      }
      const beforePub = r.originalIsPublished ? 'published' : 'unpublished'
      const afterPub = r.isPublished ? 'published' : 'unpublished'
      entries.push({
        label: r.channel.name,
        before: `${rowDescribePrice(r, false)} · ${beforePub}`,
        after: `${rowDescribePrice(r, true)} · ${afterPub}`,
      })
    }
    return entries
  }, [rows])

  const save = useCallback(async () => {
    for (const r of rows) {
      if (!rowIsDirty(r)) continue
      if (r.clearRequested) {
        await clearListing({
          variables: { productId: product.id, channelId: r.channel.id },
        })
        continue
      }
      await setListing({
        variables: {
          input: {
            productId: product.id,
            channelId: r.channel.id,
            isPublished: r.isPublished,
            price: r.inheritPrice ? null : dollarsToCents(r.priceDollars),
            url: r.url.trim() === '' ? null : r.url.trim(),
          },
        },
      })
    }
  }, [rows, product.id, setListing, clearListing])

  useProductSaveSection({
    id: 'listings',
    title: 'Per-store pricing & publishing',
    diff,
    save,
  })

  function onClear(row: RowState) {
    // Reset visible fields to the inherited / unpublished baseline AND mark
    // the row for explicit clear (if it had an existing listing)
    patchRow(row.channel.id, {
      isPublished: false,
      inheritPrice: true,
      priceDollars: '',
      url: '',
      clearRequested: row.hadExistingListing,
    })
  }

  if (channelsQuery.loading) {
    return <p className="text-sm text-muted-foreground">Loading stores…</p>
  }
  if (allChannels.length === 0) {
    return (
      <Alert>
        <AlertTitle>No channels</AlertTitle>
        <AlertDescription>
          Create at least one channel to manage per-store pricing and publishing.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="grid gap-3">
      <Alert>
        <AlertTitle className="text-xs uppercase tracking-wider">How pricing works</AlertTitle>
        <AlertDescription className="text-xs">
          Each store either inherits the product's default amount in the store currency or has an
          explicit store price. Stores marked unpublished hide this product on the channel.
          {channelId ? ' Header scope is limiting this table to the selected channel.' : ''}
        </AlertDescription>
      </Alert>

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
            {rows.map((row) => {
              const existingListing = product.listings.find(
                (l) => l.channel.id === row.channel.id,
              )
              const dirty = rowIsDirty(row)
              return (
                <TableRow key={row.channel.id} className={dirty ? 'bg-primary/5' : undefined}>
                  <TableCell className="font-medium">
                    {row.channel.name}
                    {dirty ? (
                      <Badge variant="outline" className="ml-2 text-[10px] uppercase tracking-wide">
                        {row.clearRequested ? 'pending clear' : 'pending'}
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono font-normal">
                      {row.channel.currencyCode}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="grid gap-1.5">
                      <div className="flex items-center gap-2">
                        <Switch
                          id={`inherit-${row.channel.id}`}
                          checked={row.inheritPrice}
                          onCheckedChange={(v) => patchRow(row.channel.id, { inheritPrice: v })}
                          disabled={!canManage}
                        />
                        <Label htmlFor={`inherit-${row.channel.id}`} className="text-xs">
                          Use default
                        </Label>
                      </div>
                      {!row.inheritPrice && (
                        <Input
                          type="number"
                          step="0.01"
                          min="0"
                          inputMode="decimal"
                          value={row.priceDollars}
                          onChange={(e) =>
                            patchRow(row.channel.id, { priceDollars: e.target.value })
                          }
                          disabled={!canManage}
                        />
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {existingListing?.effectivePrice != null
                      ? formatCents(
                          existingListing.effectivePrice,
                          existingListing.effectiveCurrency,
                        )
                      : '-'}
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={row.isPublished}
                      onCheckedChange={(v) => patchRow(row.channel.id, { isPublished: v })}
                      disabled={!canManage}
                    />
                  </TableCell>
                  {canManage && (
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        {existingListing && !row.clearRequested && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => onClear(row)}
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
