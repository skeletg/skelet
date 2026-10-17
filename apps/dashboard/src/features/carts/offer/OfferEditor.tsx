import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'

import { AddressFields } from '@/components/form/AddressFields'
import { useAppForm } from '@/components/form/form'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { formatCents } from '@/lib/format'
import { useSaveSection, type SaveRegistryValue } from '@/shared/saving/saveRegistry'
import {
  AdminCartDeliveryGroupsDocument,
  AdminCartDeliveryPointsDocument,
  AdminCartOfferPaymentMethodsDocument,
  AdminUpdateCartOfferDocument,
  type CartFieldsFragment,
  type CartOfferFieldsFragment,
} from '../Carts.gql'
import {
  cartOfferSchema,
  offerDiff,
  OFFER_LOCKS,
  toOfferInput,
  toOfferValues,
  type CartOfferValues,
  type OfferDelivery,
} from '../schemas/cartOffer'

// The offer's staged half - its terms, what the recipient can't change, and the checkout they start
// from. One section in the page's save registry, written by one adminUpdateCartOffer. Mount it keyed
// by the cart id, so a refetch never resets what the admin is typing
export function OfferEditor({
  cart,
  offer,
  canEdit,
  registry,
}: {
  cart: CartFieldsFragment
  offer: CartOfferFieldsFragment
  canEdit: boolean
  registry: SaveRegistryValue
}) {
  const [baseline, setBaseline] = useState(() => toOfferValues(offer))
  const [update] = useMutation(AdminUpdateCartOfferDocument)
  const form = useAppForm({ defaultValues: baseline, validators: { onChange: cartOfferSchema } })
  const values = useStore(form.store, (s) => s.values)
  const needsShipping = offer.needsShipping
  const isGuest = !cart.customerId
  const methods = useQuery(AdminCartOfferPaymentMethodsDocument, { variables: { cartId: cart.id } }).data?.adminCartOfferPaymentMethods ?? []
  const paymentLabels = Object.fromEntries(methods.map((m) => [m.slug, m.displayName]))

  useSaveSection(
    {
      id: 'offer',
      title: 'Offer',
      diff: offerDiff(baseline, values, paymentLabels),
      save: async () => {
        const parsed = cartOfferSchema.safeParse(values)
        if (!parsed.success) throw new Error(parsed.error.issues[0]?.message ?? 'Check the offer’s fields.')
        const res = await update({ variables: { cartId: cart.id, input: toOfferInput(values, needsShipping) } })
        const saved = res.data?.adminUpdateCartOffer.offer
        if (!saved) throw new Error('The offer wasn’t saved.')
        const next = withLabels(toOfferValues(saved), values.deliveries)
        setBaseline(next)
        form.reset(next)
      },
    },
    registry,
  )

  const lockable = (shipping?: boolean) => canEdit && (!shipping || needsShipping)

  return (
    <>
      <DetailPageSection title="Offer" description="When it stops working, and what the recipient reads with it.">
        <fieldset disabled={!canEdit} className="grid max-w-xl gap-4">
          <form.AppField name="expiresAt">
            {(f) => <f.TextField id="offer-expires" label="Expires" type="datetime-local" hint="After this the link shows the offer as expired." />}
          </form.AppField>
          <form.AppField name="message">
            {(f) => <f.TextareaField id="offer-message" label="Message" rows={3} placeholder="A note shown in the email and on the offer" />}
          </form.AppField>
          <form.AppField name="showInAccount">
            {(f) => (
              <f.SwitchField
                id="offer-account"
                label="Show in the customer’s account"
                description={
                  isGuest
                    ? 'Only an offer made out to a customer can be listed in an account - a guest opens it from the link.'
                    : 'Also list it under Offers in their account, not just behind the email link.'
                }
                disabled={isGuest}
              />
            )}
          </form.AppField>
        </fieldset>
      </DetailPageSection>

      <DetailPageSection
        title="Locked for the recipient"
        description={needsShipping ? 'What you fix here, the recipient sees but can’t change.' : 'Nothing in this cart ships, so there is no address or delivery to lock.'}
      >
        <form.Field name="locks">
          {(f) => (
            <div className="grid max-w-xl gap-2">
              {OFFER_LOCKS.map((l) => {
                const on = f.state.value.includes(l.value)
                const enabled = lockable(l.shipping)
                return (
                  <label key={l.value} className={`flex items-start gap-2 text-sm ${enabled ? '' : 'opacity-60'}`}>
                    <Checkbox
                      checked={on && enabled}
                      disabled={!enabled}
                      onCheckedChange={(v) =>
                        f.handleChange(v ? [...f.state.value, l.value] : f.state.value.filter((x) => x !== l.value))
                      }
                      className="mt-0.5"
                    />
                    <span>
                      <span className="font-medium">{l.label}</span>
                      <span className="block text-xs text-muted-foreground">{l.hint}</span>
                    </span>
                  </label>
                )
              })}
              {f.state.meta.errors.length > 0 && (
                <p className="text-xs text-destructive">{String(f.state.meta.errors[0]?.message ?? f.state.meta.errors[0])}</p>
              )}
            </div>
          )}
        </form.Field>
      </DetailPageSection>

      <DetailPageSection
        title="Checkout"
        description="Fill in what the recipient checks out with. Anything left empty they fill in themselves, unless it’s locked."
      >
        <fieldset disabled={!canEdit} className="grid max-w-xl gap-6">
          {needsShipping && (
            <div className="grid gap-3">
              <form.AppField name="hasShipping">
                {(f) => <f.SwitchField id="offer-has-shipping" label="Shipping address" description="Where the order goes." />}
              </form.AppField>
              {values.hasShipping && <AddressFields form={form} fields="shipping" idPrefix="offer-ship" />}
            </div>
          )}

          <div className="grid gap-3">
            {needsShipping && values.hasShipping && (
              <form.AppField name="billingSameAsShipping">
                {(f) => <f.CheckboxField id="offer-bill-same" label="Bill to the shipping address" />}
              </form.AppField>
            )}
            {!(needsShipping && values.hasShipping && values.billingSameAsShipping) && (
              <>
                <form.AppField name="hasBilling">
                  {(f) => <f.SwitchField id="offer-has-billing" label="Billing address" description="Who the invoice is made out to." />}
                </form.AppField>
                {values.hasBilling && <AddressFields form={form} fields="billing" idPrefix="offer-bill" />}
              </>
            )}
          </div>

          <form.Field name="paymentMethods">
            {(f) => <PaymentMethodsPicker methods={methods} value={f.state.value} onChange={f.handleChange} />}
          </form.Field>

          {needsShipping && (
            <form.Field name="deliveries">
              {(f) => (
                <DeliveryPicker
                  cartId={cart.id}
                  currency={cart.currencyCode}
                  hasSavedAddress={!!offer.checkout.shippingAddress}
                  value={f.state.value}
                  onChange={f.handleChange}
                />
              )}
            </form.Field>
          )}
        </fieldset>
      </DetailPageSection>
    </>
  )
}

// The server returns no labels for deliveries - carry the ones the admin picked so the next diff reads well
function withLabels(v: CartOfferValues, picked: OfferDelivery[]): CartOfferValues {
  return {
    ...v,
    deliveries: v.deliveries.map((d) => ({
      ...d,
      label: picked.find((p) => p.groupKey === d.groupKey && p.optionId === d.optionId && p.methodCode === d.methodCode)?.label ?? d.label,
    })),
  }
}

// The methods the recipient may pay with, out of the ones the channel has enabled - one fixes it, none leaves them all
function PaymentMethodsPicker({
  methods,
  value,
  onChange,
}: {
  methods: { slug: string; displayName: string }[]
  value: string[]
  onChange: (v: string[]) => void
}) {
  // A method the channel has since switched off still shows, so it can be unticked
  const shown = [...methods, ...value.filter((s) => !methods.some((m) => m.slug === s)).map((s) => ({ slug: s, displayName: `${s} (no longer enabled)` }))]
  return (
    <div className="grid gap-2">
      <Label>Payment methods</Label>
      <p className="text-xs text-muted-foreground">
        {value.length === 0
          ? 'The recipient can pay with any method the store offers. Tick the ones they may use.'
          : value.length === 1
            ? 'The recipient pays with this method.'
            : 'The recipient chooses from these methods.'}
      </p>
      {shown.length === 0 ? (
        <p className="text-xs text-muted-foreground">The channel has no payment methods enabled.</p>
      ) : (
        <div className="grid gap-2 rounded-md border p-3">
          {shown.map((m) => {
            const on = value.includes(m.slug)
            return (
              <label key={m.slug} className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={on}
                  onCheckedChange={(v) => onChange(v ? [...value, m.slug] : value.filter((s) => s !== m.slug))}
                />
                {m.displayName}
              </label>
            )
          })}
        </div>
      )}
    </div>
  )
}

const choiceKey = (c: { optionId: string; carrierSlug: string; methodCode: string }) => `${c.optionId}|${c.carrierSlug}|${c.methodCode}`

// One delivery per group, chosen from what the recipient's checkout would offer at the saved address
function DeliveryPicker({
  cartId,
  currency,
  hasSavedAddress,
  value,
  onChange,
}: {
  cartId: string
  currency: string
  hasSavedAddress: boolean
  value: OfferDelivery[]
  onChange: (v: OfferDelivery[]) => void
}) {
  const { data, loading } = useQuery(AdminCartDeliveryGroupsDocument, { variables: { cartId }, skip: !hasSavedAddress, fetchPolicy: 'cache-and-network' })
  const groups = data?.adminCartDeliveryGroups ?? []

  const set = (groupKey: string, next: OfferDelivery | null) =>
    onChange([...value.filter((d) => d.groupKey !== groupKey), ...(next ? [next] : [])])

  return (
    <div className="grid gap-3">
      <Label>Delivery</Label>
      {!hasSavedAddress ? (
        <p className="text-xs text-muted-foreground">Save a shipping address first - the choices are quoted for it.</p>
      ) : loading && groups.length === 0 ? (
        <p className="text-xs text-muted-foreground">Quoting deliveries…</p>
      ) : groups.length === 0 ? (
        <p className="text-xs text-muted-foreground">No delivery is offered for these items at the saved address.</p>
      ) : (
        groups.map((g) => {
          const current = value.find((d) => d.groupKey === g.key)
          const chosen = current ? g.choices.find((c) => choiceKey(c) === choiceKey(current)) : undefined
          return (
            <div key={g.key} className="grid gap-2 rounded-md border p-3">
              {groups.length > 1 && <p className="text-xs text-muted-foreground">{g.lines.join(', ')}</p>}
              <Select
                value={current ? choiceKey(current) : '__none'}
                onValueChange={(v) => {
                  const c = g.choices.find((x) => choiceKey(x) === v)
                  set(
                    g.key,
                    c
                      ? { groupKey: g.key, optionId: c.optionId, carrierSlug: c.carrierSlug, methodCode: c.methodCode, pointId: '', pointName: '', label: `${c.optionName} · ${c.methodName}` }
                      : null,
                  )
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none">Let the recipient choose</SelectItem>
                  {g.choices.map((c) => (
                    <SelectItem key={choiceKey(c)} value={choiceKey(c)}>
                      {c.optionName} · {c.methodName} - {formatCents(c.price, currency)}
                    </SelectItem>
                  ))}
                  {current && !chosen && <SelectItem value={choiceKey(current)}>{current.label || current.methodCode} (no longer offered)</SelectItem>}
                </SelectContent>
              </Select>
              {chosen?.needsPoint && current && (
                <PointPicker
                  cartId={cartId}
                  groupKey={g.key}
                  optionId={chosen.optionId}
                  value={current}
                  onPick={(id, name) => set(g.key, { ...current, pointId: id, pointName: name })}
                />
              )}
            </div>
          )
        })
      )}
    </div>
  )
}

function PointPicker({
  cartId,
  groupKey,
  optionId,
  value,
  onPick,
}: {
  cartId: string
  groupKey: string
  optionId: string
  value: OfferDelivery
  onPick: (id: string, name: string) => void
}) {
  const [search, setSearch] = useState('')
  const { data, loading } = useQuery(AdminCartDeliveryPointsDocument, {
    variables: { cartId, groupKey, optionId, search: search.trim() || null },
  })
  const points = data?.adminCartDeliveryPoints ?? []
  return (
    <div className="grid gap-2">
      <p className="text-xs">
        Pickup point: <span className="font-medium">{value.pointName || 'none chosen'}</span>
      </p>
      <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search points…" aria-label="Search points" />
      {loading && points.length === 0 ? (
        <p className="text-xs text-muted-foreground">Loading points…</p>
      ) : (
        <div className="max-h-48 divide-y overflow-auto rounded-md border">
          {points.map((p) => (
            <button
              key={p.id}
              type="button"
              onClick={() => onPick(p.id, p.name)}
              className={`block w-full px-3 py-2 text-left text-xs hover:bg-muted ${p.id === value.pointId ? 'bg-muted' : ''}`}
            >
              <span className="font-medium">{p.name}</span>
              <span className="block text-muted-foreground">{p.address}</span>
            </button>
          ))}
          {points.length === 0 && <p className="px-3 py-2 text-xs text-muted-foreground">No points found.</p>}
        </div>
      )}
    </div>
  )
}
