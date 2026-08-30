import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { formatCents } from '@/lib/format'
import {
  EditOrderAddressDocument,
  OrderDocument,
  type OrderQuery,
} from './Orders.gql'

type OrderData = OrderQuery['order']
type Side = 'shipping' | 'billing'
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

// Mirrors AddressInput - required flags match the backend's validEditAddress
// (first/last name, street, city, postal code, 2-letter country)
const FIELDS: { key: keyof AddressForm; label: string; required?: boolean; half?: boolean }[] = [
  { key: 'firstName', label: 'First name', required: true, half: true },
  { key: 'lastName', label: 'Last name', required: true, half: true },
  { key: 'company', label: 'Company' },
  { key: 'street1', label: 'Street line 1', required: true },
  { key: 'street2', label: 'Street line 2' },
  { key: 'city', label: 'City', required: true, half: true },
  { key: 'state', label: 'State / region', half: true },
  { key: 'postalCode', label: 'Postal code', required: true, half: true },
  { key: 'countryCode', label: 'Country', required: true, half: true },
  { key: 'phone', label: 'Phone' },
]

type OrderAddress = OrderData['shippingAddress']

function seedForm(a: OrderAddress): AddressForm {
  return {
    firstName: a.firstName,
    lastName: a.lastName,
    company: a.company ?? '',
    street1: a.street1,
    street2: a.street2 ?? '',
    city: a.city,
    state: a.state ?? '',
    postalCode: a.postalCode,
    countryCode: a.countryCode,
    phone: a.phone ?? '',
  }
}

// Optional fields go over the wire as null when blank - the snapshot keeps
// them unset rather than storing an empty string
function toInput(f: AddressForm) {
  return {
    firstName: f.firstName.trim(),
    lastName: f.lastName.trim(),
    company: f.company.trim() || null,
    street1: f.street1.trim(),
    street2: f.street2.trim() || null,
    city: f.city.trim(),
    state: f.state.trim() || null,
    postalCode: f.postalCode.trim(),
    countryCode: f.countryCode.trim().toUpperCase(),
    phone: f.phone.trim() || null,
  }
}

function invalidReason(f: AddressForm): string | null {
  if (!f.firstName.trim() || !f.lastName.trim()) return 'first and last name are required'
  if (!f.street1.trim()) return 'street line 1 is required'
  if (!f.city.trim()) return 'city is required'
  if (!f.postalCode.trim()) return 'postal code is required'
  if (f.countryCode.trim().length !== 2) return 'country code must be 2 letters'
  return null
}

function changed(a: AddressForm, b: AddressForm) {
  return FIELDS.some(({ key }) => a[key].trim() !== b[key].trim())
}

// EditOrderAddressDialog rewrites the address snapshot of a placed,
// pre-fulfillment order (backed by editOrderAddress) - Only the side you
// actually touched is sent - a shipping change makes the backend recompute
// tax and re-quote the recorded shipping method, a billing-only edit leaves
// the money alone - Once anything ships the snapshot freezes server-side
export function EditOrderAddressDialog({
  order,
  open,
  onOpenChange,
  onSaved,
}: {
  order: OrderData
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: (summary: string) => void
}) {
  const refetch = [{ query: OrderDocument, variables: { id: order.id } }, 'OrdersPaged' as const]
  const [editAddress, state] = useMutation(EditOrderAddressDocument, { refetchQueries: refetch })

  const [shipping, setShipping] = useState(() => seedForm(order.shippingAddress))
  const [billing, setBilling] = useState(() => seedForm(order.billingAddress))
  const [error, setError] = useState<string | null>(null)

  function seed() {
    setError(null)
    setShipping(seedForm(order.shippingAddress))
    setBilling(seedForm(order.billingAddress))
  }

  const shippingChanged = changed(shipping, seedForm(order.shippingAddress))
  const billingChanged = changed(billing, seedForm(order.billingAddress))
  const hasChanges = shippingChanged || billingChanged

  async function onSubmit() {
    if (!hasChanges) {
      setError('No changes to apply.')
      return
    }
    for (const [side, form, dirty] of [
      ['Shipping', shipping, shippingChanged],
      ['Billing', billing, billingChanged],
    ] as [string, AddressForm, boolean][]) {
      const reason = dirty ? invalidReason(form) : null
      if (reason) {
        setError(`${side} address: ${reason}.`)
        return
      }
    }
    setError(null)
    const before = { tax: order.taxAmount, shipping: order.shippingAmount, total: order.total }
    try {
      const res = await editAddress({
        variables: {
          input: {
            orderId: order.id,
            shippingAddress: shippingChanged ? toInput(shipping) : null,
            billingAddress: billingChanged ? toInput(billing) : null,
          },
        },
      })
      const after = res.data?.editOrderAddress
      onSaved(summarize(before, after, shippingChanged, billingChanged, order.currencyCode))
      onOpenChange(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Address edit failed.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (v) seed()
        onOpenChange(v)
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Edit addresses</DialogTitle>
          <DialogDescription>
            Rewrites the order's address snapshot. Only the side you change is sent. Changing the
            shipping address recomputes tax and re-quotes the recorded shipping method - if the
            carrier can't be reached the old shipping amount is kept.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 md:grid-cols-2">
          <AddressFields
            side="shipping"
            title="Shipping address"
            dirty={shippingChanged}
            form={shipping}
            onChange={(key, value) => setShipping((p) => ({ ...p, [key]: value }))}
          />
          <AddressFields
            side="billing"
            title="Billing address"
            dirty={billingChanged}
            form={billing}
            onChange={(key, value) => setBilling((p) => ({ ...p, [key]: value }))}
          />
        </div>

        {error && (
          <Alert variant="destructive">
            <AlertTitle>Failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={state.loading}>
            Cancel
          </Button>
          <Button type="button" onClick={onSubmit} disabled={state.loading || !hasChanges}>
            {state.loading ? 'Saving…' : 'Save addresses'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AddressFields({
  side,
  title,
  dirty,
  form,
  onChange,
}: {
  side: Side
  title: string
  dirty: boolean
  form: AddressForm
  onChange: (key: keyof AddressForm, value: string) => void
}) {
  const countryOptions = useQuery(CountriesPickerDocument).data?.countries ?? []
  return (
    <div className="rounded-md border p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</div>
        {dirty && <span className="text-xs text-muted-foreground">edited</span>}
      </div>
      <div className="grid grid-cols-2 gap-3">
        {FIELDS.map((f) => (
          <div key={f.key} className={`grid gap-1.5 ${f.half ? '' : 'col-span-2'}`}>
            <Label htmlFor={`${side}-${f.key}`} className="text-xs">
              {f.label}
              {f.required && ' *'}
            </Label>
            {f.key === 'countryCode' ? (
              // the country is a known list - typing it only ever produced a rejected save
              <Select value={form[f.key] || '__none__'} onValueChange={(v) => onChange(f.key, v === '__none__' ? '' : v)}>
                <SelectTrigger id={`${side}-${f.key}`}>
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
            ) : (
              <Input
                id={`${side}-${f.key}`}
                value={form[f.key]}
                onChange={(e) => onChange(f.key, e.target.value)}
              />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

// What the operator needs to know after the write: which side moved, and
// whether the recompute shifted any money
function summarize(
  before: { tax: number; shipping: number; total: number },
  after: { taxAmount: number; shippingAmount: number; total: number } | null | undefined,
  shippingChanged: boolean,
  billingChanged: boolean,
  currency: string,
) {
  const what =
    shippingChanged && billingChanged
      ? 'Shipping + billing address'
      : shippingChanged
        ? 'Shipping address'
        : 'Billing address'
  if (!after) return `${what} updated.`
  const moves: string[] = []
  const move = (label: string, from: number, to: number) => {
    if (from !== to) moves.push(`${label} ${formatCents(from, currency)} → ${formatCents(to, currency)}`)
  }
  move('tax', before.tax, after.taxAmount)
  move('shipping', before.shipping, after.shippingAmount)
  move('total', before.total, after.total)
  return moves.length === 0
    ? `${what} updated - totals unchanged.`
    : `${what} updated - ${moves.join(', ')}.`
}
