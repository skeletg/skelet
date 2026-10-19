import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { ALL_STORES, currencyForChoice, issueChannelId } from './issue'
import { useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon } from 'lucide-react'

import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { useDebounced } from '@/shared/hooks/useDebounced'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatCents } from '@/lib/format'
import {
  AdjustGiftCardDocument,
  DisableGiftCardDocument,
  GiftCardByCodeDocument,
  GiftCardsAdminListDocument,
  IssueGiftCardDocument,
  type GiftCardsAdminListQuery,
} from './GiftCards.gql'

type GiftCardRow = GiftCardsAdminListQuery['giftCardsPaged']['items'][number]


function statusVariant(
  status: string,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'active':
      return 'default'
    case 'expired':
    case 'disabled':
      return 'destructive'
    default: // depleted
      return 'secondary'
  }
}

// majorToCents converts a user-entered amount in major units (e.g. "50.00") to integer minor units - Assumes 2-decimal
// currencies, which covers the common case; cent-exact entry is also accepted via the same field
function majorToCents(v: string): number | null {
  const n = Number(v)
  if (!Number.isFinite(n)) return null
  return Math.round(n * 100)
}

export function GiftCardsList() {
  // Server-paged and searched, URL-backed so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { pagination, globalFilter: search } = urlState
  const debouncedSearch = useDebounced(search.trim(), 300)
  const { data, loading, error } = useQuery(GiftCardsAdminListDocument, {
    variables: {
      search: debouncedSearch || null,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    },
  })
  const rows = useMemo(() => data?.giftCardsPaged.items ?? [], [data])

  const [issueOpen, setIssueOpen] = useState(false)
  const [adjusting, setAdjusting] = useState<GiftCardRow | null>(null)
  const [disabling, setDisabling] = useState<GiftCardRow | null>(null)
  const [detailCode, setDetailCode] = useState<string | null>(null)

  const columns = useMemo<ColumnDef<GiftCardRow, unknown>[]>(
    () => [
      {
        accessorKey: 'code',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title="Code" />
        ),
        cell: ({ row }) => (
          <button
            type="button"
            onClick={() => setDetailCode(row.original.code)}
            className="font-mono text-sm font-medium hover:underline"
          >
            {row.original.code}
          </button>
        ),
      },
      {
        accessorKey: 'balance',
        header: 'Balance',
        size: 150,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {formatCents(row.original.balance, row.original.currencyCode)}
            <span className="text-muted-foreground">
              {' '}
              / {formatCents(row.original.initialBalance, row.original.currencyCode)}
            </span>
          </span>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        size: 110,
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)}>
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'recipientEmail',
        header: 'Recipient',
        cell: ({ row }) => row.original.recipientEmail || '-',
      },
      {
        accessorKey: 'createdAt',
        header: 'Issued',
        size: 130,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {new Date(row.original.createdAt as string).toLocaleDateString()}
          </span>
        ),
      },
      {
        id: 'actions',
        size: 48,
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="size-8">
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => setDetailCode(row.original.code)}>
                View ledger
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setAdjusting(row.original)}>
                Adjust balance
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                disabled={row.original.status === 'disabled'}
                onSelect={() => setDisabling(row.original)}
              >
                Disable
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Gift cards"
        meta="Stored-value cards. Issued automatically when a gift-card product is purchased, or manually here."
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable
            manual={{
              rowCount: data?.giftCardsPaged.totalCount ?? 0,
              pagination,
              onPaginationChange: urlState.setPagination,
              globalFilter: search,
              onGlobalFilterChange: urlState.setGlobalFilter,
              columnFilters: urlState.columnFilters,
              onColumnFiltersChange: urlState.setColumnFilters,
            }}
            columns={columns}
            data={rows}
            loading={loading && rows.length === 0}
            error={error ? { message: error.message } : null}
            getRowId={(row) => row.id}
            searchPlaceholder="Search code or recipient…"
            emptyState="No gift cards yet."
            toolbarRightSlot={
              <Button size="sm" onClick={() => setIssueOpen(true)}>
                <PlusIcon className="size-4" />
                Issue gift card
              </Button>
            }
          />
        </DetailPageSection>
      </DetailPageBody>

      <IssueGiftCardDialog open={issueOpen} onOpenChange={setIssueOpen} />
      <AdjustGiftCardDialog card={adjusting} onClose={() => setAdjusting(null)} />
      <DisableGiftCardConfirm
        card={disabling}
        onClose={() => setDisabling(null)}
      />
      <GiftCardDetailDialog
        code={detailCode}
        onClose={() => setDetailCode(null)}
      />
    </DetailPageLayout>
  )
}

function IssueGiftCardDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const [currencyCode, setCurrencyCode] = useState('')
  const currencyOptions = useQuery(CurrenciesPickerDocument).data?.currencies ?? []
  const channels = useQuery(ChannelsDocument).data?.channels ?? []
  const [channelId, setChannelId] = useState(ALL_STORES)
  const [amount, setAmount] = useState('')
  const [recipientEmail, setRecipientEmail] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [issue, { loading }] = useMutation(IssueGiftCardDocument, {
    refetchQueries: [GiftCardsAdminListDocument],
  })

  function reset() {
    setCurrencyCode('')
    setChannelId(ALL_STORES)
    setAmount('')
    setRecipientEmail('')
    setExpiresAt('')
    setSubmitError(null)
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    const cents = majorToCents(amount)
    if (cents == null || cents <= 0) {
      setSubmitError('Enter a positive amount.')
      return
    }
    try {
      await issue({
        variables: {
          currencyCode: currencyCode.trim().toUpperCase(),
          amount: cents,
          recipientEmail: recipientEmail.trim() || null,
          expiresAt: expiresAt ? new Date(expiresAt).toISOString() : null,
          channelId: issueChannelId(channelId),
        },
      })
      reset()
      onOpenChange(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Issue failed.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) reset()
        onOpenChange(v)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Issue gift card</DialogTitle>
          <DialogDescription>
            Mint a card with an opening balance. A unique code is generated
            automatically.
          </DialogDescription>
        </DialogHeader>
        <form id="issue-gift-card" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="gc-currency">Currency</Label>
              <Select value={currencyCode} onValueChange={setCurrencyCode}>
                <SelectTrigger className="w-full" id="gc-currency">
                  <SelectValue placeholder="Pick a currency" />
                </SelectTrigger>
                <SelectContent>
                  {currencyOptions.map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      <span className="font-mono text-xs">{c.code}</span> · {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="gc-amount">Amount</Label>
              <Input
                id="gc-amount"
                required
                type="number"
                min="0"
                step="0.01"
                placeholder="50.00"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="gc-store">Spends on</Label>
            <Select
              value={channelId}
              onValueChange={(v) => {
                setChannelId(v)
                setCurrencyCode(currencyForChoice(channels, v, currencyCode))
              }}
            >
              <SelectTrigger id="gc-store">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL_STORES}>Every store in its currency</SelectItem>
                {channels.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name} ({c.currencyCode})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="gc-email">Recipient email (optional)</Label>
            <Input
              id="gc-email"
              type="email"
              placeholder="recipient@example.com"
              value={recipientEmail}
              onChange={(e) => setRecipientEmail(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="gc-expires">Expires (optional)</Label>
            <Input
              id="gc-expires"
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
            />
          </div>
          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Couldn&apos;t issue card</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button type="submit" form="issue-gift-card" disabled={loading}>
            {loading ? 'Issuing…' : 'Issue card'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AdjustGiftCardDialog({
  card,
  onClose,
}: {
  card: GiftCardRow | null
  onClose: () => void
}) {
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [adjust, { loading }] = useMutation(AdjustGiftCardDocument, {
    refetchQueries: [GiftCardsAdminListDocument],
  })

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!card) return
    setSubmitError(null)
    const cents = majorToCents(amount)
    if (cents == null || cents === 0) {
      setSubmitError('Enter a non-zero amount (negative to deduct).')
      return
    }
    try {
      await adjust({ variables: { id: card.id, amount: cents, note: note.trim() || null } })
      setAmount('')
      setNote('')
      onClose()
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Adjustment failed.')
    }
  }

  return (
    <Dialog
      open={!!card}
      onOpenChange={(v) => {
        if (!v) {
          setAmount('')
          setNote('')
          setSubmitError(null)
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Adjust balance</DialogTitle>
          <DialogDescription>
            Apply a correction to{' '}
            <span className="font-mono">{card?.code}</span>. Positive adds funds,
            negative deducts. Recorded in the card&apos;s ledger.
          </DialogDescription>
        </DialogHeader>
        <form id="adjust-gift-card" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="gc-adjust-amount">Amount (+/−)</Label>
            <Input
              id="gc-adjust-amount"
              required
              type="number"
              step="0.01"
              placeholder="-10.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="gc-adjust-note">Note (optional)</Label>
            <Input
              id="gc-adjust-note"
              placeholder="Goodwill credit, chargeback, …"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Couldn&apos;t adjust</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="adjust-gift-card" disabled={loading}>
            {loading ? 'Saving…' : 'Apply adjustment'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DisableGiftCardConfirm({
  card,
  onClose,
}: {
  card: GiftCardRow | null
  onClose: () => void
}) {
  const [err, setErr] = useState<string | null>(null)
  const [disable, { loading }] = useMutation(DisableGiftCardDocument, {
    refetchQueries: [GiftCardsAdminListDocument],
  })

  async function onConfirm() {
    if (!card) return
    setErr(null)
    try {
      await disable({ variables: { id: card.id } })
      onClose()
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Disable failed.')
    }
  }

  return (
    <AlertDialog
      open={!!card}
      onOpenChange={(v) => {
        if (!v) {
          setErr(null)
          onClose()
        }
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Disable {card?.code}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            The card can no longer be redeemed. Its remaining balance is kept on
            record but becomes unusable. This can&apos;t be undone from here.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {err && (
          <Alert variant="destructive">
            <AlertTitle>Disable failed</AlertTitle>
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={loading}
            onClick={(e) => {
              e.preventDefault()
              onConfirm()
            }}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {loading ? 'Disabling…' : 'Disable card'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function GiftCardDetailDialog({
  code,
  onClose,
}: {
  code: string | null
  onClose: () => void
}) {
  const { data, loading, error } = useQuery(GiftCardByCodeDocument, {
    variables: { code: code ?? '' },
    skip: !code,
  })
  const card = data?.giftCard
  const channels = useQuery(ChannelsDocument, { skip: !code }).data?.channels ?? []
  const store = card?.channelId ? (channels.find((c) => c.id === card.channelId)?.name ?? 'One store') : 'Every store'

  return (
    <Dialog open={!!code} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-mono">{code}</DialogTitle>
          <DialogDescription>Balance and transaction ledger.</DialogDescription>
        </DialogHeader>
        {loading ? (
          <p className="py-6 text-sm text-muted-foreground">Loading…</p>
        ) : error ? (
          <Alert variant="destructive">
            <AlertTitle>Couldn&apos;t load card</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : !card ? (
          <p className="py-6 text-sm text-muted-foreground">Not found.</p>
        ) : (
          <div className="grid gap-4">
            <div className="flex items-center gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Balance</p>
                <p className="text-lg font-semibold tabular-nums">
                  {formatCents(card.balance, card.currencyCode)}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">Spends on</p>
                <p>{store}</p>
              </div>
              <Badge variant={statusVariant(card.status)}>{card.status}</Badge>
              {card.recipientEmail && (
                <span className="text-muted-foreground">
                  → {card.recipientEmail}
                </span>
              )}
            </div>
            <div>
              <p className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Ledger
              </p>
              <ul className="divide-y rounded-md border text-sm">
                {card.transactions.length === 0 ? (
                  <li className="px-3 py-2 text-muted-foreground">
                    No transactions.
                  </li>
                ) : (
                  card.transactions.map((tx) => (
                    <li
                      key={tx.id}
                      className="flex items-center justify-between gap-3 px-3 py-2"
                    >
                      <div>
                        <span className="font-medium">{tx.reason}</span>
                        {tx.note && (
                          <span className="ml-2 text-muted-foreground">
                            {tx.note}
                          </span>
                        )}
                        <span className="ml-2 text-xs text-muted-foreground">
                          {new Date(tx.createdAt as string).toLocaleString()}
                        </span>
                      </div>
                      <span
                        className={`tabular-nums ${tx.amount < 0 ? 'text-destructive' : 'text-foreground'}`}
                      >
                        {tx.amount < 0 ? '' : '+'}
                        {formatCents(tx.amount, card.currencyCode)}
                      </span>
                    </li>
                  ))
                )}
              </ul>
            </div>
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
