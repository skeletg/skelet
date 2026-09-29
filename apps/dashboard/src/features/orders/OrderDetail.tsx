import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useLazyQuery } from '@apollo/client/react'
import { usePageTitle } from '@/lib/page-title'
import { BanIcon, CheckCircle2Icon, CogIcon, FileTextIcon, PackageCheckIcon, PauseCircleIcon, PlayCircleIcon } from 'lucide-react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { formatCents } from '@/lib/format'
import { LoadError } from '@/shared/errors/LoadError'
import { EntityChannelForbidden, EntityWrongChannel } from '@/shared/errors/ChannelScopeError'
import { isForbiddenError } from '@/shared/errors/isForbiddenError'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import type { OrderStatus } from '@/graphql/schema.types'
import { ResendEmailMenu } from './ResendEmailMenu'
import { OrderStatusBadge } from './OrderStatusBadge'
import { PaymentStatusBadge } from './PaymentStatusBadge'
import { OrderPaymentsSection } from './OrderPaymentsSection'
import { OrderSummaryPanel } from './OrderSummaryPanel'
import { OrderTimeline } from './OrderTimeline'
import { SectionNote } from './SectionNote'
import { OrderCreditMemosSection } from './OrderCreditMemosSection'
import { OrderReturnsSection } from './OrderReturnsSection'
import { CreateShipmentDialog } from './CreateShipmentDialog'
import { EditOrderDialog } from './EditOrderDialog'
import { EditOrderAddressDialog } from './EditOrderAddressDialog'
import {
  CancelOrderDocument,
  ConfirmOrderDocument,
  HoldOrderDocument,
  ReleaseOrderDocument,
  FulfillOrderDocument,
  GenerateShippingLabelDocument,
  MarkOrderProcessingDocument,
  OrderDocument,
  PlaceDraftOrderDocument,
  OrderDocumentPdfDocument,
  OrdersPagedDocument,
} from './Orders.gql'
import { PluginSections } from '@/plugins/detail-sections'

// Shipping is gated on the status machine, so the note names the transition that unblocks it
function shipmentBlockedReason(status: string): string {
  switch (status) {
    case 'PENDING':
      return 'Nothing can ship yet - confirm the order, then mark it processing.'
    case 'CONFIRMED':
      return 'Mark the order processing to start shipping it.'
    case 'ON_HOLD':
      return 'The order is on hold. Release it to carry on fulfilling.'
    case 'CANCELLED':
      return 'The order was cancelled, so nothing more will ship.'
    case 'FULFILLED':
      return 'Everything on this order has shipped.'
    default:
      return 'Shipping is not available for an order in this status.'
  }
}

type Address = {
  firstName: string
  lastName: string
  company?: string | null
  street1: string
  street2?: string | null
  city: string
  state?: string | null
  postalCode: string
  countryCode: string
  phone?: string | null
}

export function OrderDetail() {
  const { id = '' } = useParams()
  const { scope, selectedChannel } = useConfigScope()
  const permissions = usePermissions()

  const { data, loading, error, refetch: refetchOrder } = useQuery(OrderDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const order = data?.order
  usePageTitle(order ? `Order ${order.number}` : undefined)

  const refetch = [
    { query: OrderDocument, variables: { id } },
    OrdersPagedDocument,
  ]
  const [confirmOrder,     confirmState]    = useMutation(ConfirmOrderDocument,       { refetchQueries: refetch })
  const [markProcessing,   processingState] = useMutation(MarkOrderProcessingDocument,{ refetchQueries: refetch })
  const [holdOrder,        holdState]       = useMutation(HoldOrderDocument,           { refetchQueries: refetch })
  const [releaseOrder,     releaseState]    = useMutation(ReleaseOrderDocument,        { refetchQueries: refetch })
  const [fulfillOrder,     fulfillState]    = useMutation(FulfillOrderDocument,       { refetchQueries: refetch })
  const [cancelOrder,      cancelState]     = useMutation(CancelOrderDocument,        { refetchQueries: refetch })
  const [generateLabel,    labelState]      = useMutation(GenerateShippingLabelDocument, { refetchQueries: refetch })
  const [placeDraft,       placeDraftState]  = useMutation(PlaceDraftOrderDocument,       { refetchQueries: refetch })

  const [confirmingCancel, setConfirmingCancel] = useState(false)
  const [cancelError, setCancelError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  // Fulfill flow: clicking Fulfill opens a dialog that lets the admin
  // optionally stamp carrier + tracking before the transition - Empty
  // fields are fine - fulfillment can happen without tracking (digital
  // goods, in-store pickup, etc.) and a sync plugin subscriber can
  // always fill these from a carrier API after the fact
  const [fulfillOpen, setFulfillOpen] = useState(false)
  const [fulfillCarrier, setFulfillCarrier] = useState('')
  const [fulfillTracking, setFulfillTracking] = useState('')
  const [fulfillError, setFulfillError] = useState<string | null>(null)
  // Partial fulfillment (per-line shipment) + pre-fulfillment line editing
  const [shipmentOpen, setShipmentOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  // Address edits can move tax + shipping, so the dialog hands back a
  // summary of what the recompute did and we keep it on the page
  const [addressOpen, setAddressOpen] = useState(false)
  const [addressNotice, setAddressNotice] = useState<string | null>(null)

  async function runTransition(fn: () => Promise<unknown>) {
    setActionError(null)
    try {
      await fn()
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Transition failed.')
    }
  }

  async function onCancelConfirmed() {
    setCancelError(null)
    try {
      await cancelOrder({ variables: { id } })
      setConfirmingCancel(false)
    } catch (e) {
      setCancelError(e instanceof Error ? e.message : 'Cancel failed.')
    }
  }

  async function onFulfillConfirmed() {
    setFulfillError(null)
    try {
      await fulfillOrder({
        variables: {
          id,
          carrier: fulfillCarrier.trim() || null,
          trackingNumber: fulfillTracking.trim() || null,
        },
      })
      setFulfillOpen(false)
      setFulfillCarrier('')
      setFulfillTracking('')
    } catch (e) {
      setFulfillError(e instanceof Error ? e.message : 'Fulfill failed.')
    }
  }

  // First load only - a refetch (every mutation here triggers one) swapped the page out and back,
  // which re-opened any open dialog as a fresh form and threw away what the admin had typed
  if (loading && !order) return <p className="text-sm text-muted-foreground">Loading order…</p>
  if (error) {
    // Channel-scoped roles get `forbidden` when the order belongs to a channel outside their scopes - show the
    // permissions page, not a retryable error
    if (isForbiddenError(error)) {
      return (
        <EntityChannelForbidden
          entityLabel="order"
          permission="orders.view"
          backTo="/orders"
          backLabel="Back to orders"
        />
      )
    }
    return (
      <LoadError
        title="Couldn't load order"
        message={error.message}
        onRetry={() => void refetchOrder()}
        retrying={loading}
      />
    )
  }
  if (!order) return <p className="text-sm text-muted-foreground">Order not found.</p>

  // Header channel scope gate - orders are placed on exactly one channel; mirror the customer detail behaviour for
  // global admins browsing under a channel scope
  if (scope.mode === 'CHANNEL' && selectedChannel && order.channelId !== selectedChannel.id) {
    return (
      <EntityWrongChannel
        entityLabel="order"
        entityChannelId={order.channelId}
        permission="orders.view"
        backTo="/orders"
        backLabel="Back to orders"
      />
    )
  }

  const isTerminal = order.status === 'FULFILLED' || order.status === 'CANCELLED'
  const anyRunning =
    confirmState.loading || processingState.loading || fulfillState.loading || cancelState.loading

  const canUpdate = permissions.canAccessChannelScope('orders.update', order.channelId)
  const isDraft = order.status === 'DRAFT'
  // Line edits are valid for drafts and any pre-fulfillment status (backend enforces too)
  const isEditable =
    isDraft || order.status === 'PENDING' || order.status === 'CONFIRMED' || order.status === 'PROCESSING'
  // Per-line shipments make sense once processing and until fully shipped
  const canShip = order.status === 'PROCESSING' || order.status === 'PARTIALLY_FULFILLED'

  const resendProps = {
    orderId: id,
    disabled: anyRunning,
    hasFulfillments: order.fulfillments.length > 0,
    isCancelled: order.status === 'CANCELLED',
    isRefunded: order.paymentStatus === 'REFUNDED' || order.paymentStatus === 'PARTIALLY_REFUNDED',
  }

  // What has actually moved per line, from data the page already holds - the columns only
  // appear once something has, so a pending order keeps the narrow table
  const shippedByLine = new Map<string, number>()
  for (const f of order.fulfillments) {
    if (f.status === 'CANCELLED') continue
    for (const fl of f.lines) {
      shippedByLine.set(fl.orderLineId, (shippedByLine.get(fl.orderLineId) ?? 0) + fl.quantity)
    }
  }
  const returnedByLine = new Map<string, number>()
  for (const r of order.returns) {
    if (r.status === 'REJECTED') continue
    for (const rl of r.lines) {
      returnedByLine.set(rl.orderLineId, (returnedByLine.get(rl.orderLineId) ?? 0) + rl.quantity)
    }
  }
  const showMovement = shippedByLine.size > 0 || returnedByLine.size > 0
  const qtyLabel = (moved: number, ordered: number) =>
    moved >= ordered ? 'All' : `${moved} of ${ordered}`

  return (
    <DetailPageLayout
      rightSidebar={
        <aside className="scrollbar-accent hidden h-full w-[22rem] shrink-0 flex-col overflow-y-auto border-l bg-muted/10 xl:flex 2xl:w-[24rem]">
          <div className="border-b bg-background px-4 py-3">
            <div className="text-sm font-semibold">Summary</div>
          </div>
          <div className="p-4">
            <OrderSummaryPanel order={order} isDraft={isDraft} />
          </div>
          <div className="border-y bg-background px-4 py-2">
            <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Timeline
            </div>
          </div>
          <div className="p-4">
            <OrderTimeline events={order.events} />
          </div>
        </aside>
      }
    >
      <DetailPageHeader
        backTo="/orders"
        backLabel="Back to orders"
        title={<span className="font-mono">{order.number}</span>}
        badges={
          <>
            <OrderStatusBadge status={order.status} />
            <PaymentStatusBadge status={order.paymentStatus} />
          </>
        }
        meta={
          <>
            <span className="text-foreground">
              {order.shippingAddress.firstName} {order.shippingAddress.lastName}
            </span>
            <span>·</span>
            <span>{order.email}</span>
            <span>·</span>
            {order.customerId ? (
              <Link
                to={`/customers/${order.customerId}`}
                className="underline-offset-2 hover:underline"
              >
                Customer
              </Link>
            ) : (
              <Badge variant="outline" className="text-[10px] uppercase tracking-wider">
                Guest
              </Badge>
            )}
          </>
        }
        actions={
          isDraft ? (
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                disabled={placeDraftState.loading || !canUpdate}
                onClick={() => runTransition(() => placeDraft({ variables: { id } }))}
              >
                <PackageCheckIcon className="size-4" />
                {placeDraftState.loading ? 'Placing…' : 'Place order'}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={cancelState.loading}
                onClick={() => { setCancelError(null); setConfirmingCancel(true) }}
              >
                <BanIcon className="size-4" />
                Cancel
              </Button>
            </div>
          ) : isTerminal ? (
            <>
              <DocumentButtons orderId={id} />
              {canUpdate && <ResendEmailMenu {...resendProps} />}
            </>
          ) : (
            <>
            <DocumentButtons orderId={id} />
            {canUpdate && <ResendEmailMenu {...resendProps} />}
            <TransitionButtons
              status={order.status}
              disabled={anyRunning}
              onConfirm={() => runTransition(() => confirmOrder({ variables: { id } }))}
              onMarkProcessing={() => runTransition(() => markProcessing({ variables: { id } }))}
              onHold={() => runTransition(() => holdOrder({ variables: { id } }))}
              onRelease={() => runTransition(() => releaseOrder({ variables: { id } }))}
              onFulfill={() => {
                setFulfillError(null)
                setFulfillOpen(true)
              }}
              onCancel={() => {
                setCancelError(null)
                setConfirmingCancel(true)
              }}
              running={{
                confirm: confirmState.loading,
                processing: processingState.loading,
                fulfill: fulfillState.loading,
                cancel: cancelState.loading,
                hold: holdState.loading,
                release: releaseState.loading,
              }}
            />
            </>
          )
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

        {addressNotice && (
          <div className="px-6 pt-6">
            <Alert>
              <AlertTitle>Addresses updated</AlertTitle>
              <AlertDescription>{addressNotice}</AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection
          title="Line items"
          description={`${order.lines.length} ${order.lines.length === 1 ? 'item' : 'items'}`}
          trailing={
            canUpdate && isEditable ? (
              <Button size="sm" variant="outline" onClick={() => setEditOpen(true)}>
                Edit order
              </Button>
            ) : undefined
          }
        >
          <div className="overflow-x-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Product</TableHead>
                  <TableHead>SKU</TableHead>
                  <TableHead className="w-20 text-right">Qty</TableHead>
                  {showMovement && <TableHead className="w-28 text-right">Shipped</TableHead>}
                  {showMovement && <TableHead className="w-28 text-right">Returned</TableHead>}
                  <TableHead className="w-32 text-right">Unit price</TableHead>
                  <TableHead className="w-32 text-right">Line total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {order.lines.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell className="font-medium">
                      {l.name}
                      {l.isGift && (
                        <Badge variant="outline" className="ml-2 text-[10px] uppercase tracking-wider">
                          Gift
                        </Badge>
                      )}
                      {l.components.length > 0 && (
                        <ul className="mt-1 space-y-0.5 text-xs font-normal text-muted-foreground">
                          {l.components.map((c) => (
                            <li key={c.id}>
                              {c.quantity} x <span className="text-foreground">{c.name}</span>{' '}
                              <code className="font-mono">{c.sku}</code>
                            </li>
                          ))}
                        </ul>
                      )}
                      {l.options.length > 0 && (
                        <ul className="mt-1 space-y-0.5 text-xs font-normal text-muted-foreground">
                          {l.options.map((o) => (
                            <li key={o.id}>
                              {o.optionTitle}:{' '}
                              {o.fileUrl ? (
                                <a
                                  href={o.fileUrl}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="text-foreground underline"
                                >
                                  {o.textValue || 'Download'}
                                </a>
                              ) : (
                                <span className="text-foreground">{o.valueLabel || o.textValue}</span>
                              )}
                              {o.priceDelta !== 0 &&
                                ` (${o.priceDelta > 0 ? '+' : ''}${formatCents(o.priceDelta, order.currencyCode)})`}
                            </li>
                          ))}
                        </ul>
                      )}
                    </TableCell>
                    <TableCell>
                      <code className="font-mono text-xs text-muted-foreground">{l.sku}</code>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{l.quantity}</TableCell>
                    {showMovement && (
                      <TableCell className="text-right tabular-nums">
                        {shippedByLine.get(l.id) ? (
                          qtyLabel(shippedByLine.get(l.id) ?? 0, l.quantity)
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </TableCell>
                    )}
                    {showMovement && (
                      <TableCell className="text-right tabular-nums">
                        {returnedByLine.get(l.id) ? (
                          qtyLabel(returnedByLine.get(l.id) ?? 0, l.quantity)
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </TableCell>
                    )}
                    <TableCell className="text-right tabular-nums">
                      {formatCents(l.unitPrice, order.currencyCode)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatCents(l.totalPrice, order.currencyCode)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </DetailPageSection>


        <DetailPageSection
          title="Addresses"
          description="Captured at checkout. Editable until something ships - changing the shipping address recomputes tax and re-quotes shipping."
          trailing={
            canUpdate && isEditable ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => { setAddressNotice(null); setAddressOpen(true) }}
              >
                Edit addresses
              </Button>
            ) : undefined
          }
        >
          <div className="grid gap-4 md:grid-cols-2">
            <AddressBlock title="Billing address" address={order.billingAddress} />
            <AddressBlock title="Shipping address" address={order.shippingAddress} />
          </div>
        </DetailPageSection>

        <DetailPageSection title="Summary" className="xl:hidden">
          <OrderSummaryPanel order={order} isDraft={isDraft} />
        </DetailPageSection>

        {!isDraft && <OrderPaymentsSection order={order} />}

        {!isDraft && (
          <DetailPageSection
            title="Shipments"
            description="What has left the warehouse, and the labels for it."
            trailing={
              canUpdate ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!canShip}
                  onClick={() => setShipmentOpen(true)}
                >
                  Create shipment
                </Button>
              ) : undefined
            }
          >
            {!canShip && order.fulfillments.length === 0 && (
              <SectionNote>{shipmentBlockedReason(order.status)}</SectionNote>
            )}
            {order.fulfillments.length === 0 ? (
              // Orders fulfilled before per-shipment records existed carry carrier and tracking on the order itself
              order.carrier || order.trackingNumber ? (
                <p className="text-sm">
                  Shipped via {order.carrier || 'unspecified carrier'}
                  {order.trackingNumber && (
                    <>
                      {' · '}
                      {order.trackingUrl ? (
                        <a
                          href={order.trackingUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="underline underline-offset-2"
                        >
                          {order.trackingNumber}
                        </a>
                      ) : (
                        order.trackingNumber
                      )}
                    </>
                  )}
                  <span className="ml-2 text-xs text-muted-foreground">
                    recorded on the order, before per-shipment records
                  </span>
                </p>
              ) : canShip ? (
                <p className="text-sm text-muted-foreground">
                  No shipments yet. Create a partial shipment, or use “Fulfill” to ship everything.
                </p>
              ) : null
            ) : (
            <div className="grid gap-3">
              {[...order.fulfillments]
                .sort((a, b) => a.sequence - b.sequence)
                .map((f) => {
                  const nameOf = (lineId: string) =>
                    order.lines.find((l) => l.id === lineId)?.name ?? lineId
                  return (
                    <div key={f.id} className="rounded-md border p-4 text-sm">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="font-medium">
                          Shipment #{f.sequence}
                          <span className="ml-2 text-xs uppercase tracking-wide text-muted-foreground">
                            {f.status}
                          </span>
                        </div>
                        {(f.carrier || f.trackingNumber || f.insuredValueCents > 0) && (
                          <div className="text-xs text-muted-foreground">
                            {f.carrier || 'carrier'}
                            {f.trackingNumber ? ` · ${f.trackingNumber}` : ''}
                            {f.insuredValueCents > 0
                              ? ` · insured ${formatCents(f.insuredValueCents, order.currencyCode)}`
                              : ''}
                          </div>
                        )}
                      </div>
                      <ul className="mt-2 space-y-0.5 text-xs text-muted-foreground">
                        {f.lines.map((l) => (
                          <li key={l.id}>
                            {nameOf(l.orderLineId)} × <span className="tabular-nums">{l.quantity}</span>
                          </li>
                        ))}
                      </ul>
                      <div className="mt-3 flex flex-wrap items-center gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={labelState.loading}
                          onClick={async () => {
                            setActionError(null)
                            try {
                              await generateLabel({ variables: { fulfillmentId: f.id } })
                            } catch (e) {
                              setActionError(e instanceof Error ? e.message : 'Label generation failed.')
                            }
                          }}
                        >
                          {labelState.loading ? 'Generating…' : 'Generate label'}
                        </Button>
                        {f.labels.map((lbl) => (
                          <a
                            key={lbl.id}
                            href={lbl.dataUrl}
                            download={`label-${f.sequence}.${lbl.format}`}
                            className="text-xs text-foreground underline underline-offset-2"
                          >
                            Download label ({lbl.format})
                          </a>
                        ))}
                      </div>
                    </div>
                  )
                })}
            </div>
            )}
          </DetailPageSection>
        )}


        {!isDraft && <OrderReturnsSection order={order} />}

        {!isDraft && <OrderCreditMemosSection order={order} />}


        {order.customerNote && (
          <DetailPageSection title="Customer note">
            <p className="whitespace-pre-wrap text-sm">{order.customerNote}</p>
          </DetailPageSection>
        )}

        <DetailPageSection
          title="Timeline"
          description="Everything that happened to this order, newest first."
          className="xl:hidden"
        >
          <OrderTimeline events={order.events} />
        </DetailPageSection>
        <PluginSections target="order" entity={order} mode="edit" />
      </DetailPageBody>

      <AlertDialog
        open={confirmingCancel}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingCancel(false)
            setCancelError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel order {order.number}?</AlertDialogTitle>
            <AlertDialogDescription>
              The order moves to the CANCELLED state and no further transitions are possible.
              Cancellation cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {cancelError && (
            <Alert variant="destructive">
              <AlertTitle>Cancel failed</AlertTitle>
              <AlertDescription>{cancelError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={cancelState.loading}>Keep order</AlertDialogCancel>
            <AlertDialogAction
              disabled={cancelState.loading}
              onClick={(e) => { e.preventDefault(); onCancelConfirmed() }}
            >
              {cancelState.loading ? 'Cancelling…' : 'Cancel order'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={fulfillOpen}
        onOpenChange={(v) => {
          if (!v) {
            setFulfillOpen(false)
            setFulfillError(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Fulfill order {order.number}</DialogTitle>
            <DialogDescription>
              Mark the order as shipped. Carrier + tracking are optional -
              leave blank for digital goods, in-store pickup, or when a
              carrier-API plugin will fill them automatically.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-2">
              <Label htmlFor="fulfill-carrier">Carrier</Label>
              <Input
                id="fulfill-carrier"
                value={fulfillCarrier}
                onChange={(e) => setFulfillCarrier(e.target.value)}
                placeholder="USPS, FedEx, DHL…"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="fulfill-tracking">Tracking number</Label>
              <Input
                id="fulfill-tracking"
                value={fulfillTracking}
                onChange={(e) => setFulfillTracking(e.target.value)}
                placeholder="1Z999AA10123456784"
              />
            </div>
            {fulfillError && (
              <Alert variant="destructive">
                <AlertTitle>Fulfill failed</AlertTitle>
                <AlertDescription>{fulfillError}</AlertDescription>
              </Alert>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setFulfillOpen(false)}
              disabled={fulfillState.loading}
            >
              Cancel
            </Button>
            <Button
              type="button"
              onClick={onFulfillConfirmed}
              disabled={fulfillState.loading}
            >
              {fulfillState.loading ? 'Marking shipped…' : 'Mark shipped'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CreateShipmentDialog order={order} open={shipmentOpen} onOpenChange={setShipmentOpen} />
      <EditOrderDialog order={order} open={editOpen} onOpenChange={setEditOpen} />
      <EditOrderAddressDialog
        order={order}
        open={addressOpen}
        onOpenChange={setAddressOpen}
        onSaved={setAddressNotice}
      />
    </DetailPageLayout>
  )
}

// ---------- Helpers ----------

function TransitionButtons({
  status,
  disabled,
  onConfirm,
  onMarkProcessing,
  onHold,
  onRelease,
  onFulfill,
  onCancel,
  running,
}: {
  status: OrderStatus
  disabled: boolean
  onConfirm: () => void
  onMarkProcessing: () => void
  onHold: () => void
  onRelease: () => void
  onFulfill: () => void
  onCancel: () => void
  running: { confirm: boolean; processing: boolean; fulfill: boolean; cancel: boolean; hold: boolean; release: boolean }
}) {
  const buttons: React.ReactNode[] = []

  if (status === 'PENDING') {
    buttons.push(
      <Button key="confirm" size="sm" disabled={disabled} onClick={onConfirm}>
        <CheckCircle2Icon className="size-4" />
        {running.confirm ? 'Confirming…' : 'Confirm'}
      </Button>,
    )
  }
  if (status === 'CONFIRMED') {
    buttons.push(
      <Button key="processing" size="sm" disabled={disabled} onClick={onMarkProcessing}>
        <CogIcon className="size-4" />
        {running.processing ? 'Marking…' : 'Mark processing'}
      </Button>,
    )
  }
  if (status === 'PENDING' || status === 'CONFIRMED') {
    buttons.push(
      <Button key="hold" size="sm" variant="outline" disabled={disabled} onClick={onHold}>
        <PauseCircleIcon className="size-4" />
        {running.hold ? 'Holding…' : 'Hold'}
      </Button>,
    )
  }
  if (status === 'ON_HOLD') {
    buttons.push(
      <Button key="release" size="sm" disabled={disabled} onClick={onRelease}>
        <PlayCircleIcon className="size-4" />
        {running.release ? 'Releasing…' : 'Release hold'}
      </Button>,
    )
  }
  if (status === 'PROCESSING' || status === 'PARTIALLY_FULFILLED') {
    buttons.push(
      <Button key="fulfill" size="sm" disabled={disabled} onClick={onFulfill}>
        <PackageCheckIcon className="size-4" />
        {running.fulfill ? 'Fulfilling…' : status === 'PARTIALLY_FULFILLED' ? 'Fulfill remaining' : 'Fulfill'}
      </Button>,
    )
  }

  // Cancel is available from any non-terminal state
  buttons.push(
    <Button key="cancel" size="sm" variant="outline" disabled={disabled} onClick={onCancel}>
      <BanIcon className="size-4" />
      Cancel
    </Button>,
  )

  return <>{buttons}</>
}

function AddressBlock({ title, address }: { title: string; address: Address }) {
  return (
    <div className="rounded-md border p-4 text-sm">
      <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      <div className="font-medium">
        {address.firstName} {address.lastName}
      </div>
      {address.company && <div className="text-muted-foreground">{address.company}</div>}
      <div>{address.street1}</div>
      {address.street2 && <div>{address.street2}</div>}
      <div>
        {address.city}
        {address.state ? `, ${address.state}` : ''} {address.postalCode}
      </div>
      <div className="text-muted-foreground">{address.countryCode}</div>
      {address.phone && <div className="text-muted-foreground">{address.phone}</div>}
    </div>
  )
}


// Paperwork downloads: base64 PDF from the backend renderer
// Credit memos are NOT here - each memo is its own document, downloaded from the credit memos section
function DocumentButtons({ orderId }: { orderId: string }) {
  const [fetchPdf, { loading }] = useLazyQuery(OrderDocumentPdfDocument, { fetchPolicy: 'network-only' })
  async function download(kind: 'INVOICE' | 'PACKING_SLIP') {
    const res = await fetchPdf({ variables: { orderId, kind } })
    if (!res.data) return
    const bytes = Uint8Array.from(atob(res.data.orderDocumentPdf), (c) => c.charCodeAt(0))
    const blob = new Blob([bytes], { type: 'application/pdf' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${kind.toLowerCase()}-${orderId.slice(0, 8)}.pdf`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }
  return (
    <>
      <Button variant="outline" size="sm" disabled={loading} onClick={() => download('INVOICE')}>
        <FileTextIcon className="size-4" />
        Invoice
      </Button>
      <Button variant="outline" size="sm" disabled={loading} onClick={() => download('PACKING_SLIP')}>
        <FileTextIcon className="size-4" />
        Packing slip
      </Button>
    </>
  )
}
