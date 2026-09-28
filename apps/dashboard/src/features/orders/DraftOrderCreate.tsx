import { useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Trash2Icon, XIcon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { AddressFields } from '@/components/form/AddressFields'
import { FieldError } from '@/components/form/fields'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CartDocument } from '@/features/carts/Carts.gql'
import { CustomerSearch, ProductSearch } from '@/features/carts/pickers'
import { CreateDraftOrderDocument, OrdersPagedDocument, PlaceDraftOrderDocument } from './Orders.gql'
import {
  addDraftLine,
  discountCents,
  draftOrderSchema,
  toDraftOrderInput,
  toDraftOrderValues,
  type DraftCustomer,
  type DraftLine,
} from './schemas/draftOrder'

// The /orders/new route - with ?cartId= it prefills from that cart (customer + simple lines) and
// converts the cart on save; otherwise it's a blank manual order. The keyed form starts from the
// fetched cart, no sync-in-effect
export function DraftOrderCreate() {
  const [params] = useSearchParams()
  const cartId = params.get('cartId')
  const { data, loading } = useQuery(CartDocument, { variables: { id: cartId ?? '' }, skip: !cartId })
  if (cartId && loading && !data) return <p className="text-sm text-muted-foreground">Loading cart…</p>

  const cart = data?.cart
  const customer: DraftCustomer | null = cart?.customerId
    ? {
        id: cart.customerId,
        email: cart.customerEmail ?? '',
        label: cart.customerEmail ? `${cart.customerName ?? cart.customerEmail} · ${cart.customerEmail}` : (cart.customerName ?? 'Customer'),
      }
    : null
  const lines: DraftLine[] = (cart?.lines ?? [])
    .filter((l) => l.isSimple)
    .map((l) => ({ sku: l.productSku, name: l.productName, unitPrice: l.unitPrice, currency: cart?.currencyCode ?? 'USD', quantity: l.quantity }))

  return (
    <DraftOrderForm
      key={cart?.id ?? 'blank'}
      initial={{ customer, email: customer?.email ?? '', lines }}
      sourceCartId={cart?.id ?? null}
    />
  )
}

// A manual order: pick a customer (or guest), add simple-product lines, set addresses and an optional
// discount, then "Save draft" (stays DRAFT, no stock hold) or "Save & place" (reserves stock, fires the
// placed-order events)
function DraftOrderForm({
  initial,
  sourceCartId,
}: {
  initial: { customer: DraftCustomer | null; email: string; lines: DraftLine[] }
  sourceCartId: string | null
}) {
  const navigate = useNavigate()
  const { effectiveChannelId, selectedChannel, scope } = useConfigScope()
  const [createDraft] = useMutation(CreateDraftOrderDocument, { refetchQueries: [OrdersPagedDocument] })
  const [placeDraft] = useMutation(PlaceDraftOrderDocument, { refetchQueries: [OrdersPagedDocument] })
  // Which of the two header buttons submitted, and the id the create came back with
  const placeOnSave = useRef(false)
  const createdId = useRef('')

  // The prefill arrives once per mount (the form is keyed on the cart) and must not move under the form
  const [baseline] = useState(() => toDraftOrderValues(initial))
  const record = useRecordForm({
    id: 'draft-order',
    title: 'Order',
    label: 'New draft order',
    isEdit: false,
    baseline,
    schema: draftOrderSchema,
    diff: () => [],
    persist: { routeKey: `draft-order:new:${sourceCartId ?? ''}` },
    submit: async (v) => {
      if (!effectiveChannelId) throw new Error('Pick a channel in the header scope picker to create an order.')
      const res = await createDraft({ variables: { input: toDraftOrderInput(v, effectiveChannelId, sourceCartId) } })
      const id = res.data?.createDraftOrder.id
      if (!id) throw new Error('Draft creation returned no id.')
      createdId.current = id
      if (placeOnSave.current) await placeDraft({ variables: { id } })
    },
    onSaved: () => navigate(`/orders/${createdId.current}`),
  })
  const { form } = record
  const values = useStore(form.store, (s) => s.values)
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  const currency = values.lines[0]?.currency || 'USD'
  const subtotal = values.lines.reduce((sum, l) => sum + l.unitPrice * l.quantity, 0)
  const discount = discountCents(values)

  // The ref is what the submit reads; the state is what the button labels render from
  const [placing, setPlacing] = useState(false)
  const submit = (place: boolean) => {
    placeOnSave.current = place
    setPlacing(place)
    record.save()
  }

  return (
    <RecordFormPage
      record={record}
      hideSave
      backTo="/orders"
      backLabel="Back to orders"
      title="New order"
      meta={<span>{selectedChannel ? `Channel: ${selectedChannel.name}` : 'No channel selected'}</span>}
      createLabel="Save draft"
      actions={
        <>
          <Button type="button" variant="outline" size="sm" disabled={submitting || !effectiveChannelId} onClick={() => submit(false)}>
            {submitting && !placing ? 'Saving…' : 'Save draft'}
          </Button>
          <Button type="button" size="sm" disabled={submitting || !effectiveChannelId} onClick={() => submit(true)}>
            {submitting && placing ? 'Placing…' : 'Save & place'}
          </Button>
        </>
      }
    >
      {scope.mode === 'GLOBAL' && (
        <div className="px-6 pt-6">
          <Alert>
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>Manual orders belong to a channel. Choose one in the header scope picker.</AlertDescription>
          </Alert>
        </div>
      )}
      {sourceCartId && (
        <div className="px-6 pt-6">
          <Alert>
            <AlertTitle>Building from a cart</AlertTitle>
            <AlertDescription>
              Customer and simple line items are prefilled from the cart. Bundle/configured items aren't carried over. The cart is
              marked converted when you save.
            </AlertDescription>
          </Alert>
        </div>
      )}

      <DetailPageSection title="Customer" description="Link a customer, or leave blank for a guest order. Email is required either way.">
        <form.Field name="customer">
          {(f) =>
            f.state.value ? (
              <div className="mb-3 flex max-w-md items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm">
                <span>{f.state.value.label}</span>
                <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => f.handleChange(null)} aria-label="Unlink customer">
                  <XIcon className="size-3.5" />
                </Button>
              </div>
            ) : (
              <CustomerSearch
                onPick={(c) => {
                  f.handleChange({ id: c.id, email: c.email, label: `${c.firstName} ${c.lastName} · ${c.email}` })
                  if (form.state.values.email.trim() === '') form.setFieldValue('email', c.email)
                }}
              />
            )
          }
        </form.Field>
        <form.AppField name="email">
          {(f) => <f.TextField id="draft-email" label="Email" type="email" placeholder="customer@example.com" className="mt-3 max-w-md" />}
        </form.AppField>
      </DetailPageSection>

      <DetailPageSection title="Line items" description="Simple products only. Prices are re-derived server-side from the catalog.">
        <ProductSearch
          onAdd={(p) =>
            form.setFieldValue('lines', (lines) => addDraftLine(lines, { sku: p.sku, name: p.name, unitPrice: p.unitPrice, currency: p.currency }))
          }
        />
        <form.Field name="lines" mode="array">
          {(lines) => (
            <div className="mt-3 grid gap-2">
              {lines.state.value.map((l, i) => (
                <div key={l.sku} className="flex items-center gap-3 rounded border px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{l.name}</div>
                    <div className="font-mono text-[10px] text-muted-foreground">
                      {l.sku} · {formatCents(l.unitPrice, l.currency)} each
                    </div>
                  </div>
                  <form.Field name={`lines[${i}].quantity`}>
                    {(f) => (
                      <Input
                        type="number"
                        min={1}
                        step="any"
                        aria-label={`${l.name} quantity`}
                        value={f.state.value}
                        onChange={(e) => f.handleChange(Number(e.target.value))}
                        className="w-20 tabular-nums"
                      />
                    )}
                  </form.Field>
                  <div className="w-24 text-right text-sm tabular-nums">{formatCents(l.unitPrice * l.quantity, l.currency)}</div>
                  <Button type="button" variant="ghost" size="icon" className="shrink-0" onClick={() => lines.removeValue(i)} aria-label="Remove line">
                    <Trash2Icon className="size-4 text-destructive" />
                  </Button>
                </div>
              ))}
              <FieldError field={lines} />
              {lines.state.value.length > 0 && (
                <>
                  <dl className="ml-auto grid w-64 grid-cols-[1fr_auto] gap-y-1 pt-2 text-sm">
                    <dt className="text-muted-foreground">Subtotal</dt>
                    <dd className="text-right tabular-nums">{formatCents(subtotal, currency)}</dd>
                    {discount > 0 && (
                      <>
                        <dt className="text-muted-foreground">Discount</dt>
                        <dd className="text-right tabular-nums">−{formatCents(Math.min(discount, subtotal), currency)}</dd>
                      </>
                    )}
                    <dt className="border-t pt-1 font-medium">Est. total*</dt>
                    <dd className="border-t pt-1 text-right font-medium tabular-nums">{formatCents(Math.max(subtotal - discount, 0), currency)}</dd>
                  </dl>
                  <p className="ml-auto w-64 text-[10px] text-muted-foreground">*Tax + shipping added server-side.</p>
                </>
              )}
            </div>
          )}
        </form.Field>
      </DetailPageSection>

      <DetailPageSection title="Discount" description="Optional flat whole-order discount in minor units (cents). Clamped to the subtotal.">
        <form.AppField name="discount">
          {(f) => <f.TextField id="draft-discount" label="Discount (cents)" inputMode="numeric" placeholder="0" className="max-w-xs" />}
        </form.AppField>
      </DetailPageSection>

      <DetailPageSection title="Addresses" description="Billing and shipping. Country/region must be enabled for the channel.">
        <div className="grid gap-6 md:grid-cols-2">
          <div className="grid content-start gap-3">
            <h4 className="text-sm font-semibold">Billing</h4>
            <AddressFields form={form} fields="billing" idPrefix="bill" optionalNames />
          </div>
          <div className="grid content-start gap-3">
            <div className="flex items-center justify-between">
              <h4 className="text-sm font-semibold">Shipping</h4>
              <form.Field name="shipSame">
                {(f) => (
                  <div className="flex items-center gap-2">
                    <Switch id="ship-same" checked={f.state.value} onCheckedChange={f.handleChange} />
                    <Label htmlFor="ship-same" className="cursor-pointer text-xs">
                      Same as billing
                    </Label>
                  </div>
                )}
              </form.Field>
            </div>
            {!values.shipSame && <AddressFields form={form} fields="shipping" idPrefix="ship" optionalNames />}
          </div>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Note" description="Optional internal/customer note stored on the order.">
        <form.AppField name="note">
          {(f) => <f.TextField aria-label="Note" placeholder="Phone order, net-30 invoice…" className="max-w-xl" />}
        </form.AppField>
      </DetailPageSection>
    </RecordFormPage>
  )
}
