import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { Trash2Icon, XIcon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { ProductsDocument } from '@/features/catalog/Catalog.gql'
import { CustomerSearch } from '@/features/carts/pickers'
import {
  CreateDraftOrderDocument,
  PlaceDraftOrderDocument,
} from './Orders.gql'
import { CartDocument } from '@/features/carts/Carts.gql'

type AddressForm = {
  firstName: string
  lastName: string
  company: string
  street1: string
  street2: string
  city: string
  state: string
  postalCode: string
  countryCode: string
  phone: string
}

const emptyAddress: AddressForm = {
  firstName: '', lastName: '', company: '', street1: '', street2: '',
  city: '', state: '', postalCode: '', countryCode: '', phone: '',
}

type DraftLine = { sku: string; name: string; unitPrice: number; currency: string; quantity: number }

type DraftCustomer = { id: string; email: string; label: string }

// DraftOrderForm builds an admin/manual order: pick a customer (or guest), add
// simple-product lines, set addresses + an optional manual discount, then "Save
// draft" (stays DRAFT, no stock hold) or "Save & place" (reserves stock, fires
// the placed-order events) - Prices are authoritative server-side; the UI values
// are hints. v1 is simple products only - Initial customer/email/lines + a
// sourceCartId are supplied by the wrapper when building from a cart
function DraftOrderForm({
  initialCustomer,
  initialEmail,
  initialLines,
  sourceCartId,
}: {
  initialCustomer: DraftCustomer | null
  initialEmail: string
  initialLines: DraftLine[]
  sourceCartId: string | null
}) {
  const navigate = useNavigate()
  const { effectiveChannelId, selectedChannel, scope } = useConfigScope()

  const draft = useDraft({
    customer: initialCustomer as DraftCustomer | null,
    email: initialEmail,
    lines: initialLines as DraftLine[],
    billing: emptyAddress as AddressForm,
    shipping: emptyAddress as AddressForm,
    shipSame: true,
    discount: '',
    note: '',
  }, 'New draft order')
  const { customer, email, lines, billing, shipping, shipSame, discount, note } = draft.values
  const { setCustomer, setEmail, setLines, setBilling, setShipping, setShipSame, setDiscount, setNote } = draft.setters
  const [error, setError] = useState<string | null>(null)

  const refetch = ['OrdersPaged']
  const [createDraft, createState] = useMutation(CreateDraftOrderDocument, { refetchQueries: refetch })
  const [placeDraft, placeState] = useMutation(PlaceDraftOrderDocument, { refetchQueries: refetch })
  const busy = createState.loading || placeState.loading

  const currency = lines[0]?.currency || 'USD'
  const subtotal = lines.reduce((sum, l) => sum + l.unitPrice * l.quantity, 0)
  const discountCents = Number.parseInt(discount.trim() || '0', 10) || 0
  const canSubmit = !!effectiveChannelId && email.trim() !== '' && lines.length > 0 && !busy

  function addLine(sku: string, name: string, unitPrice: number, lineCurrency: string) {
    setLines((prev) => {
      const existing = prev.find((l) => l.sku === sku)
      if (existing) return prev.map((l) => (l.sku === sku ? { ...l, quantity: l.quantity + 1 } : l))
      return [...prev, { sku, name, unitPrice, currency: lineCurrency, quantity: 1 }]
    })
  }

  function addrInput(a: AddressForm) {
    return {
      firstName: a.firstName,
      lastName: a.lastName,
      company: a.company.trim() || null,
      street1: a.street1,
      street2: a.street2.trim() || null,
      city: a.city,
      state: a.state.trim() || null,
      postalCode: a.postalCode,
      countryCode: a.countryCode.trim().toUpperCase(),
      phone: a.phone.trim() || null,
    }
  }

  async function onSubmit(place: boolean) {
    if (!effectiveChannelId) {
      setError('Pick a channel in the header scope picker to create an order.')
      return
    }
    if (lines.length === 0) {
      setError('Add at least one product.')
      return
    }
    const ship = shipSame ? billing : shipping
    for (const [label, a] of [['Billing', billing], ['Shipping', ship]] as const) {
      if (!a.street1 || !a.city || !a.postalCode || !a.countryCode) {
        setError(`${label} address needs street, city, postal code, and country.`)
        return
      }
    }
    setError(null)
    try {
      const res = await createDraft({
        variables: {
          input: {
            channelId: effectiveChannelId,
            customerId: customer?.id ?? null,
            email: email.trim(),
            billingAddress: addrInput(billing),
            shippingAddress: addrInput(ship),
            lines: lines.map((l) => ({ productSku: l.sku, quantity: l.quantity })),
            manualDiscountCents: discountCents > 0 ? discountCents : null,
            customerNote: note.trim() || null,
            sourceCartId,
          },
        },
      })
      const id = res.data?.createDraftOrder.id
      if (!id) throw new Error('Draft creation returned no id.')
      if (place) await placeDraft({ variables: { id } })
      await duringCommit(() => navigate(`/orders/${id}`))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create the order.')
    }
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/orders"
        backLabel="Back to orders"
        title="New order"
        meta={
          <span>
            {selectedChannel ? `Channel: ${selectedChannel.name}` : 'No channel selected'}
          </span>
        }
        actions={
          <>
            <Button variant="outline" size="sm" disabled={!canSubmit} onClick={() => onSubmit(false)}>
              {createState.loading && !placeState.loading ? 'Saving…' : 'Save draft'}
            </Button>
            <Button size="sm" disabled={!canSubmit} onClick={() => onSubmit(true)}>
              {placeState.loading ? 'Placing…' : 'Save & place'}
            </Button>
          </>
        }
      />
      <DetailPageBody>
        {scope.mode === 'GLOBAL' && (
          <div className="px-6 pt-6">
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>
                Manual orders belong to a channel. Choose one in the header scope picker.
              </AlertDescription>
            </Alert>
          </div>
        )}
        {error && (
          <div className="px-6 pt-6">
            <Alert variant="destructive">
              <AlertTitle>Couldn't create order</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        )}
        {sourceCartId && (
          <div className="px-6 pt-6">
            <Alert>
              <AlertTitle>Building from a cart</AlertTitle>
              <AlertDescription>
                Customer and simple line items are prefilled from the cart. Bundle/configured items
                aren't carried over. The cart is marked converted when you save.
              </AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection title="Customer" description="Link a customer, or leave blank for a guest order. Email is required either way.">
          {customer ? (
            <div className="mb-3 flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm">
              <span>{customer.label}</span>
              <Button variant="ghost" size="icon" className="size-7" onClick={() => setCustomer(null)} aria-label="Unlink customer">
                <XIcon className="size-3.5" />
              </Button>
            </div>
          ) : (
            <CustomerSearch
              onPick={(c) => {
                setCustomer({ id: c.id, email: c.email, label: `${c.firstName} ${c.lastName} · ${c.email}` })
                if (email.trim() === '') setEmail(c.email)
              }}
            />
          )}
          <div className="mt-3 grid max-w-md gap-1.5">
            <Label htmlFor="draft-email">Email</Label>
            <Input id="draft-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="customer@example.com" />
          </div>
        </DetailPageSection>

        <DetailPageSection title="Line items" description="Simple products only. Prices are re-derived server-side from the catalog.">
          <ProductSearch onAdd={addLine} />
          {lines.length > 0 && (
            <div className="mt-3 grid gap-2">
              {lines.map((l) => (
                <div key={l.sku} className="flex items-center gap-3 rounded border px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{l.name}</div>
                    <div className="font-mono text-[10px] text-muted-foreground">
                      {l.sku} · {formatCents(l.unitPrice, l.currency)} each
                    </div>
                  </div>
                  <Input
                    type="number"
                    min={1}
                    step="any"
                    value={l.quantity}
                    onChange={(e) =>
                      setLines((prev) => prev.map((x) => (x.sku === l.sku ? { ...x, quantity: Number(e.target.value) } : x)))
                    }
                    className="w-20 tabular-nums"
                  />
                  <div className="w-24 text-right text-sm tabular-nums">{formatCents(l.unitPrice * l.quantity, l.currency)}</div>
                  <Button variant="ghost" size="icon" className="shrink-0" onClick={() => setLines((prev) => prev.filter((x) => x.sku !== l.sku))} aria-label="Remove line">
                    <Trash2Icon className="size-4 text-destructive" />
                  </Button>
                </div>
              ))}
              <dl className="ml-auto grid w-64 grid-cols-[1fr_auto] gap-y-1 pt-2 text-sm">
                <dt className="text-muted-foreground">Subtotal</dt>
                <dd className="text-right tabular-nums">{formatCents(subtotal, currency)}</dd>
                {discountCents > 0 && (
                  <>
                    <dt className="text-muted-foreground">Discount</dt>
                    <dd className="text-right tabular-nums">−{formatCents(Math.min(discountCents, subtotal), currency)}</dd>
                  </>
                )}
                <dt className="border-t pt-1 font-medium">Est. total*</dt>
                <dd className="border-t pt-1 text-right font-medium tabular-nums">
                  {formatCents(Math.max(subtotal - discountCents, 0), currency)}
                </dd>
              </dl>
              <p className="ml-auto w-64 text-[10px] text-muted-foreground">*Tax + shipping added server-side.</p>
            </div>
          )}
        </DetailPageSection>

        <DetailPageSection title="Discount" description="Optional flat whole-order discount in minor units (cents). Clamped to the subtotal.">
          <div className="grid max-w-xs gap-1.5">
            <Label htmlFor="draft-discount">Discount (cents)</Label>
            <Input id="draft-discount" inputMode="numeric" value={discount} onChange={(e) => setDiscount(e.target.value)} placeholder="0" />
          </div>
        </DetailPageSection>

        <DetailPageSection title="Addresses" description="Billing and shipping. Country/region must be enabled for the channel.">
          <div className="grid gap-6 md:grid-cols-2">
            <div className="grid gap-3">
              <h4 className="text-sm font-semibold">Billing</h4>
              <AddressFields idPrefix="bill" value={billing} onChange={setBilling} />
            </div>
            <div className="grid gap-3">
              <div className="flex items-center justify-between">
                <h4 className="text-sm font-semibold">Shipping</h4>
                <div className="flex items-center gap-2">
                  <Switch id="ship-same" checked={shipSame} onCheckedChange={setShipSame} />
                  <Label htmlFor="ship-same" className="cursor-pointer text-xs">Same as billing</Label>
                </div>
              </div>
              {!shipSame && <AddressFields idPrefix="ship" value={shipping} onChange={setShipping} />}
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection title="Note" description="Optional internal/customer note stored on the order.">
          <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="Phone order, net-30 invoice…" className="max-w-xl" />
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// DraftOrderCreate is the route entry (/orders/new) - With ?cartId= it prefills
// the form from that cart (customer + simple lines) and converts the cart on
// save; otherwise it's a blank manual order - The keyed inner form initializes
// from the fetched cart without a sync-in-effect
export function DraftOrderCreate() {
  const [params] = useSearchParams()
  const cartId = params.get('cartId')
  const { data, loading } = useQuery(CartDocument, { variables: { id: cartId ?? '' }, skip: !cartId })

  if (cartId && loading && !data) {
    return <p className="text-sm text-muted-foreground">Loading cart…</p>
  }

  const cart = data?.cart
  let initialCustomer: DraftCustomer | null = null
  let initialEmail = ''
  const initialLines: DraftLine[] = []
  if (cart) {
    if (cart.customerId) {
      const label = cart.customerName ?? cart.customerEmail ?? 'Customer'
      initialCustomer = {
        id: cart.customerId,
        email: cart.customerEmail ?? '',
        label: cart.customerEmail ? `${label} · ${cart.customerEmail}` : label,
      }
      initialEmail = cart.customerEmail ?? ''
    }
    for (const l of cart.lines) {
      if (!l.isSimple) continue
      initialLines.push({
        sku: l.productSku,
        name: l.productName,
        unitPrice: l.unitPrice,
        currency: cart.currencyCode,
        quantity: l.quantity,
      })
    }
  }

  return (
    <DraftOrderForm
      key={cart?.id ?? 'blank'}
      initialCustomer={initialCustomer}
      initialEmail={initialEmail}
      initialLines={initialLines}
      sourceCartId={cart?.id ?? null}
    />
  )
}

function AddressFields({ idPrefix, value, onChange }: { idPrefix: string; value: AddressForm; onChange: (a: AddressForm) => void }) {
  const countryOptions = useQuery(CountriesPickerDocument).data?.countries ?? []
  const set = (k: keyof AddressForm) => (e: React.ChangeEvent<HTMLInputElement>) => onChange({ ...value, [k]: e.target.value })
  const field = (k: keyof AddressForm, label: string, ph?: string) => (
    <div className="grid gap-1">
      <Label htmlFor={`${idPrefix}-${k}`} className="text-xs">{label}</Label>
      <Input id={`${idPrefix}-${k}`} value={value[k]} onChange={set(k)} placeholder={ph} className="h-8" />
    </div>
  )
  // the country is a known list, so it picks rather than types - same control the order editor uses
  const countryField = () => (
    <div className="grid gap-1">
      <Label htmlFor={`${idPrefix}-countryCode`} className="text-xs">Country</Label>
      <Select
        value={value.countryCode || '__none__'}
        onValueChange={(v) => onChange({ ...value, countryCode: v === '__none__' ? '' : v })}
      >
        <SelectTrigger id={`${idPrefix}-countryCode`} className="h-8">
          <SelectValue placeholder="Country" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__">- None -</SelectItem>
          {countryOptions.map((c) => (
            <SelectItem key={c.code} value={c.code}>
              {c.emojiFlag ? `${c.emojiFlag} ` : ''}
              {c.name}
              <span className="ml-1 text-xs text-muted-foreground">({c.code})</span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
  return (
    <div className="grid grid-cols-2 gap-2">
      {field('firstName', 'First name')}
      {field('lastName', 'Last name')}
      <div className="col-span-2">{field('company', 'Company (optional)')}</div>
      <div className="col-span-2">{field('street1', 'Street')}</div>
      <div className="col-span-2">{field('street2', 'Street 2 (optional)')}</div>
      {field('city', 'City')}
      {field('state', 'State / region')}
      {field('postalCode', 'Postal code')}
      {countryField()}
      <div className="col-span-2">{field('phone', 'Phone (optional)')}</div>
    </div>
  )
}

function ProductSearch({ onAdd }: { onAdd: (sku: string, name: string, unitPrice: number, currency: string) => void }) {
  const { data, loading } = useQuery(ProductsDocument, { variables: {} })
  const [search, setSearch] = useState('')
  const matches = useMemo(() => {
    const all = (data?.products ?? []).filter((p) => p.kind === 'SIMPLE' && !!p.sku)
    const q = search.trim().toLowerCase()
    if (!q) return all.slice(0, 8)
    return all.filter((p) => [p.name ?? '', p.sku ?? ''].join(' ').toLowerCase().includes(q)).slice(0, 8)
  }, [data, search])
  return (
    <div className="grid max-w-md gap-2">
      <Input type="search" placeholder={loading ? 'Loading products…' : 'Search products to add…'} value={search} onChange={(e) => setSearch(e.target.value)} className="h-8" />
      {search.trim() !== '' && (
        <div className="grid gap-1 rounded-md border bg-popover p-1 text-sm">
          {matches.length === 0 ? (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">No simple products match.</p>
          ) : (
            matches.map((p) => (
              <button
                key={p.id}
                type="button"
                className="flex items-center justify-between gap-2 rounded px-2 py-1.5 text-left hover:bg-accent"
                onClick={() => { onAdd(p.sku ?? '', p.name ?? p.sku ?? '', p.defaultPrice ?? 0, p.defaultCurrencyCode ?? 'USD'); setSearch('') }}
              >
                <span className="grid gap-0.5">
                  <span className="font-medium">{p.name ?? p.sku ?? ''}</span>
                  <span className="font-mono text-xs text-muted-foreground">{p.sku ?? ''}</span>
                </span>
                <span className="text-xs tabular-nums text-muted-foreground">{formatCents(p.defaultPrice ?? 0, p.defaultCurrencyCode ?? 'USD')}</span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}
