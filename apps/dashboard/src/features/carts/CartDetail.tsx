import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { CopyIcon, FileTextIcon, Trash2Icon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { formatCents } from '@/lib/format'
import type { AdminCartLineOptionInput, ProductOptionInputType } from '@/graphql/schema.types'
import {
  AdminAddCartLineDocument,
  AdminClearCartDocument,
  AdminDuplicateCartDocument,
  AdminRemoveCartLineDocument,
  AdminUpdateCartLineDocument,
  CartAddProductOptionsDocument,
  CartDocument,
} from './Carts.gql'
import { visibleOptionIds } from './optionVisibility'
import { ProductSearch } from './pickers'
import { PluginSections } from '@/plugins/detail-sections'
import { usePermissions } from '@/shared/auth/usePermissions'

export function CartDetail() {
  // Turning a cart into an order needs orders.update - without it the button only led to an access-restricted page or a refused save
  const canCreateOrder = usePermissions().has('orders.update')
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { data, loading, error, refetch } = useQuery(CartDocument, { variables: { id }, skip: !id })
  const cart = data?.cart

  const [addLine, addState] = useMutation(AdminAddCartLineDocument)
  const [updateLine] = useMutation(AdminUpdateCartLineDocument)
  const [removeLine] = useMutation(AdminRemoveCartLineDocument)
  const [clearCart, clearState] = useMutation(AdminClearCartDocument)
  const [duplicateCart, dupState] = useMutation(AdminDuplicateCartDocument)
  const [actionError, setActionError] = useState<string | null>(null)
  // A picked product waits here while the admin fills its options - products without options add straight through
  const [pending, setPending] = useState<PickedProduct | null>(null)

  async function run(fn: () => Promise<unknown>) {
    setActionError(null)
    try {
      await fn()
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Action failed.')
    }
  }

  if (loading && !cart) return <p className="text-sm text-muted-foreground">Loading cart…</p>
  if (error) {
    return (
      <LoadError title="Couldn't load cart" message={error.message} onRetry={() => void refetch()} retrying={loading} />
    )
  }
  if (!cart) return <p className="text-sm text-muted-foreground">Cart not found.</p>

  const currency = cart.currencyCode
  const isActive = cart.status === 'active'
  const owner = cart.customerName ?? cart.customerEmail ?? 'Guest'

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/carts"
        backLabel="Back to carts"
        title={<span>{owner}</span>}
        badges={
          <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
            {cart.status}
          </Badge>
        }
        meta={
          <>
            <span className="font-medium text-foreground tabular-nums">{formatCents(cart.subtotal, currency)}</span>
            <span>·</span>
            <span>{cart.itemCount} {cart.itemCount === 1 ? 'item' : 'items'}</span>
            {cart.customerId && (
              <>
                <span>·</span>
                <Link to={`/customers/${cart.customerId}`} className="underline-offset-2 hover:underline">
                  Customer
                </Link>
              </>
            )}
          </>
        }
        actions={
          <div className="flex items-center gap-2">
            {canCreateOrder && (
              <Button
                size="sm"
                disabled={!isActive || cart.lines.length === 0}
                onClick={() => navigate(`/orders/new?cartId=${cart.id}`)}
              >
                <FileTextIcon className="size-4" />
                Create order
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              disabled={dupState.loading || cart.lines.length === 0}
              onClick={() =>
                run(async () => {
                  const res = await duplicateCart({ variables: { cartId: cart.id } })
                  const newId = res.data?.adminDuplicateCart.id
                  if (newId) navigate(`/carts/${newId}`)
                })
              }
            >
              <CopyIcon className="size-4" />
              {dupState.loading ? 'Duplicating…' : 'Duplicate'}
            </Button>
          </div>
        }
      />
      <DetailPageBody>
        {actionError && (
          <div className="px-6 pt-6">
            <Alert variant="destructive">
              <AlertTitle>Action failed</AlertTitle>
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection
          title="Line items"
          description={isActive ? 'Edit this cart live - the shopper shares it.' : 'This cart is no longer active and can’t be edited.'}
          trailing={
            isActive && cart.lines.length > 0 ? (
              <Button
                size="sm"
                variant="outline"
                disabled={clearState.loading}
                onClick={() => run(() => clearCart({ variables: { cartId: cart.id } }))}
              >
                {clearState.loading ? 'Clearing…' : 'Clear cart'}
              </Button>
            ) : undefined
          }
        >
          {isActive && (
            <div className="mb-4 max-w-md">
              <ProductSearch onAdd={setPending} />
              {pending && (
                <ConfigureLine
                  product={pending}
                  busy={addState.loading}
                  onCancel={() => setPending(null)}
                  onSubmit={(quantity, options) =>
                    void run(async () => {
                      const res = await addLine({
                        variables: {
                          cartId: cart.id,
                          sku: pending.sku,
                          quantity,
                          options: options.length > 0 ? options : undefined,
                        },
                      })
                      setPending(null)
                      return res
                    })
                  }
                />
              )}
              {addState.loading && <p className="mt-1 text-xs text-muted-foreground">Adding…</p>}
            </div>
          )}

          {cart.lines.length === 0 ? (
            <p className="text-sm text-muted-foreground">Cart is empty.</p>
          ) : (
            <div className="grid gap-2">
              {cart.lines.map((l) => (
                <div key={l.id} className="flex items-center gap-3 rounded border px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">
                      {l.name}
                      {!l.isSimple && (
                        <Badge variant="secondary" className="ml-2 text-[9px] uppercase">
                          {l.components.length > 0 ? 'bundle' : 'configured'}
                        </Badge>
                      )}
                    </div>
                    <div className="font-mono text-[10px] text-muted-foreground">
                      {l.sku} · {formatCents(l.unitPrice, currency)} each
                    </div>
                    {l.options.length > 0 && (
                      <ul className="mt-1 space-y-0.5">
                        {l.options.map((o) => (
                          <li key={o.id} className="text-[11px] text-muted-foreground">
                            <span className="font-medium text-foreground/80">{o.optionTitle}:</span>{' '}
                            {o.fileUrl ? (
                              <a href={o.fileUrl} target="_blank" rel="noreferrer" className="underline">
                                {o.textValue || 'uploaded file'}
                              </a>
                            ) : (
                              o.valueLabel || o.textValue
                            )}
                            {o.priceDelta !== 0 && (
                              <span className="ml-1 tabular-nums">
                                ({o.priceDelta > 0 ? '+' : '-'}
                                {formatCents(Math.abs(o.priceDelta), currency)})
                              </span>
                            )}
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                  {isActive && l.isSimple ? (
                    <QtyInput
                      key={`${l.id}:${l.quantity}`}
                      initial={l.quantity}
                      onCommit={(q) => run(() => updateLine({ variables: { cartId: cart.id, lineId: l.id, quantity: q } }))}
                    />
                  ) : (
                    <span className="w-20 text-right text-sm tabular-nums">{l.quantity}</span>
                  )}
                  <div className="w-24 text-right text-sm tabular-nums">{formatCents(l.totalPrice, currency)}</div>
                  {isActive && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="shrink-0"
                      onClick={() => run(() => removeLine({ variables: { cartId: cart.id, lineId: l.id } }))}
                      aria-label="Remove line"
                    >
                      <Trash2Icon className="size-4 text-destructive" />
                    </Button>
                  )}
                </div>
              ))}
              <div className="ml-auto flex w-64 items-center justify-between border-t pt-2 text-sm">
                <span className="font-medium">Subtotal</span>
                <span className="font-medium tabular-nums">{formatCents(cart.subtotal, currency)}</span>
              </div>
              <p className="ml-auto w-64 text-[10px] text-muted-foreground">Tax + shipping are added when you create the order.</p>
            </div>
          )}
        </DetailPageSection>
        <PluginSections target="cart" entity={cart ?? null} mode="edit" />
      </DetailPageBody>
    </DetailPageLayout>
  )
}

type PickedProduct = { id: string; sku: string; name: string; unitPrice: number; currency: string }

// ConfigureLine collects option picks for a chosen product before the add - without it
// any SKU carrying a required option is un-orderable from the console
function ConfigureLine({
  product,
  busy,
  onCancel,
  onSubmit,
}: {
  product: PickedProduct
  busy: boolean
  onCancel: () => void
  onSubmit: (quantity: number, options: AdminCartLineOptionInput[]) => void
}) {
  const { data, loading } = useQuery(CartAddProductOptionsDocument, { variables: { id: product.id } })
  const [quantity, setQuantity] = useState('1')
  const [picks, setPicks] = useState<Record<string, { valueIds: string[]; textValue: string }>>({})

  const options = useMemo(
    () => [...(data?.product.productOptions ?? [])].sort((a, b) => a.sortOrder - b.sortOrder),
    [data],
  )
  // Configured defaults are derived, not seeded into state - picks only holds what the admin actually touched
  const effective = useMemo(() => {
    const out: Record<string, { valueIds: string[]; textValue: string }> = {}
    for (const o of options) {
      if (picks[o.id]) {
        out[o.id] = picks[o.id]
        continue
      }
      const defaults = o.values.filter((v) => v.isDefault).map((v) => v.id)
      out[o.id] = {
        valueIds: isSingleSelect(o.inputType) ? defaults.slice(0, 1) : defaults,
        textValue: '',
      }
    }
    return out
  }, [options, picks])

  const pick = (id: string) => effective[id] ?? { valueIds: [], textValue: '' }
  const visible = useMemo(
    () =>
      visibleOptionIds(
        options,
        (id) => effective[id]?.valueIds ?? [],
        (id) => (effective[id]?.textValue ?? '').trim() !== '',
      ),
    [options, effective],
  )

  const shown = options.filter((o) => visible.has(o.id))
  const fileBlocked = shown.some((o) => o.inputType === 'FILE' && o.isRequired)
  const missing = shown.filter((o) => {
    if (!o.isRequired || o.inputType === 'FILE') return false
    const p = pick(o.id)
    return isSelectType(o.inputType) ? p.valueIds.length === 0 : p.textValue.trim() === ''
  })

  const submit = () => {
    const q = Number(quantity)
    if (!(q > 0)) return
    const payload: AdminCartLineOptionInput[] = []
    for (const o of shown) {
      if (o.inputType === 'FILE') continue
      const p = pick(o.id)
      if (isSelectType(o.inputType)) {
        if (p.valueIds.length > 0) payload.push({ optionId: o.id, valueIds: p.valueIds })
      } else if (p.textValue.trim() !== '') {
        payload.push({ optionId: o.id, textValue: p.textValue.trim() })
      }
    }
    onSubmit(q, payload)
  }

  return (
    <div className="mt-2 rounded-md border p-3">
      <div className="mb-2 flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{product.name}</p>
          <p className="font-mono text-[10px] text-muted-foreground">{product.sku}</p>
        </div>
        <Input
          type="number"
          min={1}
          step="any"
          value={quantity}
          onChange={(e) => setQuantity(e.target.value)}
          className="w-20 tabular-nums"
          aria-label="Quantity"
        />
      </div>

      {loading && <p className="text-xs text-muted-foreground">Loading options…</p>}

      {shown.length > 0 && (
        <div className="space-y-3 border-t pt-3">
          {shown.map((o) => (
            <div key={o.id} className="space-y-1">
              <Label className="text-xs">
                {o.title}
                {o.isRequired && <span className="ml-1 text-destructive">*</span>}
              </Label>
              {o.helpText && <p className="text-[10px] text-muted-foreground">{o.helpText}</p>}
              {o.inputType === 'FILE' ? (
                <p className="text-[11px] text-muted-foreground">
                  File options can only be filled by the shopper - ask them to add this item themselves.
                </p>
              ) : isSingleSelect(o.inputType) ? (
                <Select
                  value={pick(o.id).valueIds[0] ?? ''}
                  onValueChange={(v) => setPicks((p) => ({ ...p, [o.id]: { valueIds: [v], textValue: '' } }))}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue placeholder="Choose…" />
                  </SelectTrigger>
                  <SelectContent>
                    {[...o.values]
                      .sort((a, b) => a.sortOrder - b.sortOrder)
                      .map((v) => (
                        <SelectItem key={v.id} value={v.id}>
                          {v.title}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              ) : isSelectType(o.inputType) ? (
                <div className="space-y-1">
                  {[...o.values]
                    .sort((a, b) => a.sortOrder - b.sortOrder)
                    .map((v) => {
                      const checked = pick(o.id).valueIds.includes(v.id)
                      return (
                        <label key={v.id} className="flex items-center gap-2 text-xs">
                          <Checkbox
                            checked={checked}
                            onCheckedChange={() =>
                              setPicks((p) => {
                                const cur = p[o.id]?.valueIds ?? []
                                return {
                                  ...p,
                                  [o.id]: {
                                    valueIds: checked ? cur.filter((x) => x !== v.id) : [...cur, v.id],
                                    textValue: '',
                                  },
                                }
                              })
                            }
                          />
                          {v.title}
                        </label>
                      )
                    })}
                </div>
              ) : o.inputType === 'AREA' ? (
                <Textarea
                  rows={2}
                  maxLength={o.maxChars ?? undefined}
                  value={pick(o.id).textValue}
                  onChange={(e) => setPicks((p) => ({ ...p, [o.id]: { valueIds: [], textValue: e.target.value } }))}
                />
              ) : (
                <Input
                  type={textInputType(o.inputType)}
                  maxLength={o.maxChars ?? undefined}
                  value={pick(o.id).textValue}
                  onChange={(e) => setPicks((p) => ({ ...p, [o.id]: { valueIds: [], textValue: e.target.value } }))}
                  className="h-8"
                />
              )}
            </div>
          ))}
        </div>
      )}

      {fileBlocked && (
        <p className="mt-2 text-[11px] text-destructive">
          This product needs a file upload, so it can't be added from here.
        </p>
      )}

      <div className="mt-3 flex items-center gap-2">
        <Button size="sm" disabled={busy || loading || missing.length > 0 || fileBlocked} onClick={submit}>
          {busy ? 'Adding…' : 'Add to cart'}
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        {missing.length > 0 && (
          <span className="text-[11px] text-muted-foreground">Fill {missing.map((o) => o.title).join(', ')}</span>
        )}
      </div>
    </div>
  )
}

function isSelectType(t: ProductOptionInputType) {
  return t === 'DROPDOWN' || t === 'RADIO' || t === 'CHECKBOX' || t === 'MULTISELECT'
}

function isSingleSelect(t: ProductOptionInputType) {
  return t === 'DROPDOWN' || t === 'RADIO'
}

function textInputType(t: ProductOptionInputType) {
  if (t === 'DATE') return 'date'
  if (t === 'DATE_TIME') return 'datetime-local'
  if (t === 'TIME') return 'time'
  return 'text'
}

// QtyInput commits a quantity change on blur / Enter (not per keystroke) - Remount
// via a key including the server quantity reseeds it after an update
function QtyInput({ initial, onCommit }: { initial: number; onCommit: (q: number) => void }) {
  const [value, setValue] = useState(String(initial))
  const commit = () => {
    const q = Number(value)
    if (q > 0 && q !== initial) onCommit(q)
    else setValue(String(initial))
  }
  return (
    <Input
      type="number"
      min={1}
      step="any"
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
      }}
      className="w-20 tabular-nums"
    />
  )
}
