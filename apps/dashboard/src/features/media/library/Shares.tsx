import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { UserPlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CustomerSearch } from '@/features/carts/pickers'
import { MediaAssetSharesDocument, RevokeMediaShareDocument, ShareMediaAssetDocument, type MediaShareFieldsFragment as Share } from '../MediaLibrary.gql'

type Customer = { id: string; email: string; firstName: string; lastName: string }

const used = (s: Share) => (s.maxDownloads != null ? `${s.downloadsUsed} of ${s.maxDownloads} downloads` : `${s.downloadsUsed} ${s.downloadsUsed === 1 ? 'download' : 'downloads'}`)
const until = (s: Share) => (s.expiresAt ? (new Date(s.expiresAt) < new Date() ? 'ended' : `until ${new Date(s.expiresAt).toLocaleDateString()}`) : 'no end date')

// Who a private file has been handed to: a share puts it in that customer's account downloads, the only way anyone but an admin
// gets to it. A file made public keeps its shares, so there it shows only while it has some (onlyIfAny). A customer this admin may not see comes back unnamed
export function Shares({
  assetId,
  canShare,
  canSeeCustomers,
  onlyIfAny = false,
  onDirty,
}: {
  assetId: string
  canShare: boolean
  canSeeCustomers: boolean
  onlyIfAny?: boolean
  onDirty?: (dirty: boolean) => void
}) {
  const shares = useQuery(MediaAssetSharesDocument, { variables: { assetId }, fetchPolicy: 'cache-and-network' })
  const [share, { loading: sharing }] = useMutation(ShareMediaAssetDocument)
  const [revoke, { loading: revoking }] = useMutation(RevokeMediaShareDocument)
  const [adding, setAdding] = useState(false)
  const [customer, setCustomer] = useState<Customer | null>(null)
  const [limit, setLimit] = useState('')
  const [ends, setEnds] = useState('')
  const [error, setError] = useState<string | null>(null)
  const dirty = adding && (customer !== null || limit !== '' || ends !== '')
  useEffect(() => {
    onDirty?.(dirty)
    return () => onDirty?.(false)
  }, [dirty, onDirty])

  const close = () => {
    setAdding(false)
    setCustomer(null)
    setLimit('')
    setEnds('')
  }
  async function run(act: () => Promise<unknown>) {
    setError(null)
    try {
      await act()
      await shares.refetch()
    } catch (e) {
      setError(e instanceof Error ? e.message : "That didn't go through.")
    }
  }
  const send = () =>
    run(async () => {
      if (!customer) return
      await share({
        variables: {
          input: {
            assetId,
            customerId: customer.id,
            maxDownloads: limit.trim() === '' ? null : Number(limit),
            // the whole of the chosen day, in the admin's own time
            expiresAt: ends ? new Date(`${ends}T23:59:59`).toISOString() : null,
          },
        },
      })
      close()
    })

  const list = shares.data?.mediaAssetShares ?? []
  if (onlyIfAny && list.length === 0) return null
  return (
    <section aria-label="Shared with" className="grid gap-2 border-t border-foreground/10 pt-4">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-sm font-medium">Shared with</h3>
        <span className="text-xs text-muted-foreground">shows in their account downloads</span>
      </div>
      {shares.error ? (
        <p className="text-xs text-muted-foreground">Who this file is shared with couldn't be loaded.</p>
      ) : !shares.data ? (
        <p className="text-xs text-muted-foreground">Loading…</p>
      ) : list.length === 0 ? (
        <p className="text-xs text-muted-foreground">No one yet. Only admins can open this file.</p>
      ) : (
        <ul className="grid divide-y rounded-md border text-xs">
          {list.map((s) => (
            <li key={s.id} className="flex items-center gap-2 px-2.5 py-1.5">
              <span className="grid min-w-0 flex-1">
                <span className={s.email ? 'truncate font-medium' : 'truncate text-muted-foreground'}>
                  {s.email || "A customer you can't see"}
                  {s.email ? null : <span className="sr-only"> - seeing who takes the permission to view this customer's channel</span>}
                </span>
                <span className="text-muted-foreground">
                  {used(s)}, {until(s)}
                </span>
              </span>
              {canShare ? (
                <Button type="button" variant="ghost" size="icon" className="size-6 shrink-0 text-muted-foreground" aria-label={`Stop sharing with ${s.email || 'this customer'}`} disabled={revoking} onClick={() => void run(() => revoke({ variables: { id: s.id } }))}>
                  <XIcon className="size-3.5" />
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      {!canShare || shares.error || onlyIfAny ? null : !canSeeCustomers ? (
        <p className="text-xs text-muted-foreground">Sharing it with a customer also takes the permission to view customers.</p>
      ) : !adding ? (
        <Button type="button" variant="outline" size="sm" className="justify-start" onClick={() => setAdding(true)}>
          <UserPlusIcon className="size-4" />
          Share with a customer
        </Button>
      ) : (
        <div className="grid gap-2.5 rounded-md border p-2.5">
          {customer ? (
            <p className="flex items-center justify-between gap-2 text-xs">
              <span className="min-w-0 truncate">
                <span className="font-medium">{`${customer.firstName} ${customer.lastName}`.trim() || customer.email}</span> <span className="text-muted-foreground">{customer.email}</span>
              </span>
              <button type="button" aria-label="Change the customer" className="shrink-0 text-muted-foreground underline underline-offset-2 hover:text-foreground" onClick={() => setCustomer(null)}>
                Change
              </button>
            </p>
          ) : (
            <CustomerSearch onPick={setCustomer} />
          )}
          <div className="grid grid-cols-2 gap-2">
            <div className="grid gap-1">
              <Label htmlFor="share-limit" className="text-xs">
                Download limit
              </Label>
              <Input id="share-limit" type="number" min={1} inputMode="numeric" placeholder="No limit" className="h-8" value={limit} onChange={(e) => setLimit(e.target.value)} />
            </div>
            <div className="grid gap-1">
              <Label htmlFor="share-ends" className="text-xs">
                Last day
              </Label>
              <Input id="share-ends" type="date" className="h-8" value={ends} onChange={(e) => setEnds(e.target.value)} />
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={close}>
              Cancel
            </Button>
            <Button type="button" size="sm" disabled={!customer || sharing} onClick={() => void send()}>
              {sharing ? 'Sharing…' : 'Share'}
            </Button>
          </div>
        </div>
      )}
    </section>
  )
}
