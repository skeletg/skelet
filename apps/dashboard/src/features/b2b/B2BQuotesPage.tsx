import { useMutation, useQuery } from '@apollo/client/react'
import { useMemo, useState } from 'react'
import type { ColumnDef } from '@/components/data-table/tanstack'

import { DataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { formatCents } from '@/lib/format'

import {
  B2BQuotesDocument,
  CreateDraftOrderFromQuoteDocument,
  PriceB2BQuoteDocument,
} from './B2B.gql'

type QuoteRow = {
  id: string
  status: string
  note: string
  sellerNote: string
  totalCents: number
  createdAt: string
  items: { sku: string; name: string; quantity: number; unitPriceCents: number; optionsSummary: string }[]
}

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive'> = {
  requested: 'secondary',
  quoted: 'default',
  accepted: 'default',
  converted: 'default',
  declined: 'destructive',
  expired: 'destructive',
}

export function B2BQuotesPage() {
  const [status, setStatus] = useState('requested')
  const { data, loading, refetch } = useQuery(B2BQuotesDocument, {
    variables: { status: status === 'all' ? '' : status },
  })
  const [pricing, setPricing] = useState<QuoteRow | null>(null)
  const [convert] = useMutation(CreateDraftOrderFromQuoteDocument)
  const [convertMsg, setConvertMsg] = useState<string | null>(null)

  const rows: QuoteRow[] = useMemo(
    () =>
      (data?.b2bQuotes ?? []).map((q) => ({
        id: q.id,
        status: q.status,
        note: q.note,
        sellerNote: q.sellerNote,
        totalCents: q.totalCents,
        createdAt: q.createdAt as string,
        items: q.items.map((i) => ({
          sku: i.sku,
          name: i.name,
          quantity: i.quantity,
          unitPriceCents: i.unitPriceCents,
          optionsSummary: i.optionsSummary,
        })),
      })),
    [data],
  )

  const columns: ColumnDef<QuoteRow>[] = useMemo(
    () => [
      {
        id: 'lines',
        header: 'Request',
        cell: ({ row }) => (
          <div className="max-w-md">
            <p className="line-clamp-1 font-medium">
              {row.original.items
                .map((i) => `${i.quantity}× ${i.sku}`)
                .join(', ')}
            </p>
            {row.original.items.some((i) => i.optionsSummary) && (
              <p className="line-clamp-1 text-xs text-muted-foreground">
                {row.original.items
                  .filter((i) => i.optionsSummary)
                  .map((i) => i.optionsSummary)
                  .join(" · ")}
              </p>
            )}
            {row.original.note && (
              <p className="line-clamp-1 text-xs text-muted-foreground">
                “{row.original.note}”
              </p>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => (
          <Badge variant={STATUS_VARIANT[row.original.status] ?? 'secondary'}>
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'totalCents',
        header: 'Total',
        cell: ({ row }) =>
          row.original.totalCents > 0
            ? formatCents(row.original.totalCents, 'USD')
            : '-',
      },
      {
        accessorKey: 'createdAt',
        header: 'Requested',
        cell: ({ row }) => new Date(row.original.createdAt).toLocaleDateString(),
      },
      {
        id: 'actions',
        cell: ({ row }) => (
          <div className="flex justify-end gap-2">
            {(row.original.status === 'requested' || row.original.status === 'quoted') && (
              <Button variant="outline" size="sm" onClick={() => setPricing(row.original)}>
                {row.original.status === 'requested' ? 'Price' : 'Reprice'}
              </Button>
            )}
            {row.original.status === 'accepted' && (
              <Button
                size="sm"
                onClick={() =>
                  void convert({ variables: { quoteId: row.original.id } })
                    .then((res) => {
                      setConvertMsg(`Draft order ${res.data?.createDraftOrderFromQuote.number} created.`)
                      void refetch()
                    })
                    .catch((err: unknown) =>
                      setConvertMsg(err instanceof Error ? err.message : 'Conversion failed.'),
                    )
                }
              >
                Create draft order
              </Button>
            )}
          </div>
        ),
      },
    ],
    [convert, refetch],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="B2B quotes"
        meta="Price requested quotes, then convert accepted ones into draft orders at the negotiated prices."
        actions={
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="requested">Requested</SelectItem>
              <SelectItem value="quoted">Quoted</SelectItem>
              <SelectItem value="accepted">Accepted</SelectItem>
              <SelectItem value="converted">Converted</SelectItem>
              <SelectItem value="all">All</SelectItem>
            </SelectContent>
          </Select>
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          {convertMsg && <p className="text-sm text-muted-foreground">{convertMsg}</p>}
          <DataTable columns={columns} data={rows} loading={loading} />
        </DetailPageSection>
      </DetailPageBody>

      <PriceQuoteDialog
        quote={pricing}
        onClose={() => setPricing(null)}
        onPriced={() => void refetch()}
      />
    </DetailPageLayout>
  )
}

function PriceQuoteDialog({
  quote,
  onClose,
  onPriced,
}: {
  quote: QuoteRow | null
  onClose: () => void
  onPriced: () => void
}) {
  const [prices, setPrices] = useState<Record<string, string>>({})
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [price, { loading }] = useMutation(PriceB2BQuoteDocument)

  if (!quote) return null

  const priceFor = (sku: string, fallback: number) => {
    const raw = prices[sku]
    if (raw === undefined) return fallback
    const parsed = Math.round(Number.parseFloat(raw) * 100)
    return Number.isNaN(parsed) ? fallback : parsed
  }
  const total = quote.items.reduce(
    (sum, it) => sum + priceFor(it.sku, it.unitPriceCents) * it.quantity,
    0,
  )

  async function onSubmit() {
    if (!quote) return
    setError(null)
    try {
      await price({
        variables: {
          id: quote.id,
          items: quote.items.map((it) => ({
            sku: it.sku,
            name: it.name,
            quantity: it.quantity,
            unitPriceCents: priceFor(it.sku, it.unitPriceCents),
            optionsSummary: it.optionsSummary || null,
          })),
          sellerNote: note.trim() || quote.sellerNote || null,
        },
      })
      setPrices({})
      setNote('')
      onPriced()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not publish the quote.')
    }
  }

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Price quote</DialogTitle>
          <DialogDescription>
            Set the per-unit price for each requested line; publishing moves
            the quote to “quoted” for the customer to accept.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 text-sm">
          {quote.items.map((it) => (
            <div key={it.sku} className="grid grid-cols-[1fr_auto_8rem] items-center gap-2">
              <span className="truncate">
                {it.name} <span className="font-mono text-xs">({it.sku})</span>
              </span>
              <span className="text-muted-foreground">× {it.quantity}</span>
              <Input
                type="number"
                step="0.01"
                min="0"
                placeholder={(it.unitPriceCents / 100).toFixed(2)}
                value={prices[it.sku] ?? ''}
                onChange={(e) => setPrices((p) => ({ ...p, [it.sku]: e.target.value }))}
              />
            </div>
          ))}
          <Input
            placeholder="Note to the customer (optional)"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
          <p className="text-right font-medium">
            Quote total: {formatCents(total, 'USD')}
          </p>
          {error && <p className="text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={() => void onSubmit()} disabled={loading}>
            {loading ? 'Publishing…' : 'Publish quote'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
