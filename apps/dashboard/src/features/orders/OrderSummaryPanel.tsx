import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDownIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { formatCents } from '@/lib/format'
import type { OrderQuery } from './Orders.gql'

type OrderData = OrderQuery['order']

function Row({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'total' | 'due' | 'muted'
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 text-sm">
      <span className={tone === 'total' ? 'font-medium' : 'text-muted-foreground'}>{label}</span>
      <span
        className={
          tone === 'total'
            ? 'font-semibold tabular-nums'
            : tone === 'due'
              ? 'font-semibold tabular-nums text-amber-600 dark:text-amber-500'
              : tone === 'muted'
                ? 'tabular-nums text-muted-foreground'
                : 'tabular-nums'
        }
      >
        {value}
      </span>
    </div>
  )
}

function Group({ children }: { children: React.ReactNode }) {
  return <div className="grid min-w-0 gap-1.5 px-4 py-3">{children}</div>
}

// One line of a breakdown. min-w-0 is on the row AND the label because both are grid/flex items that default to min-width:auto - without it
// a long rule name sets the intrinsic width, and the fixed-width sidebar overflows instead of truncating
function BreakdownRow({
  label,
  title,
  value,
}: {
  label: React.ReactNode
  title: string
  value: string
}) {
  return (
    <div className="flex min-w-0 items-baseline justify-between gap-3 text-xs">
      <span className="min-w-0 truncate text-muted-foreground" title={title}>
        {label}
      </span>
      <span className="shrink-0 tabular-nums">{value}</span>
    </div>
  )
}

// A total with its own breakdown - collapsed by default, because the number is the
// answer most of the time and the rules behind it are the follow-up question
function ExpandableRow({
  label,
  value,
  children,
}: {
  label: string
  value: string
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="min-w-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-baseline justify-between gap-4 text-left text-sm"
      >
        <span className="flex items-center gap-1 text-muted-foreground hover:text-foreground">
          {label}
          <ChevronDownIcon
            className={`size-3 transition-transform ${open ? 'rotate-180' : ''}`}
            aria-hidden
          />
        </span>
        <span className="tabular-nums">{value}</span>
      </button>
      {open && <div className="mt-1.5 grid min-w-0 gap-1.5 pl-4">{children}</div>}
    </div>
  )
}

// The charge breakdown, what has actually been collected against it, and who it belongs to Rendered twice: as the right
// sidebar from lg up, inline in the body below that, since a hidden sidebar would take the totals with it on a laptop
export function OrderSummaryPanel({ order, isDraft }: { order: OrderData; isDraft: boolean }) {
  const money = (cents: number) => formatCents(cents, order.currencyCode)

  const succeeded = order.paymentTransactions.filter((t) => t.status === 'SUCCEEDED')
  const sum = (kind: string) =>
    succeeded.filter((t) => t.kind === kind).reduce((n, t) => n + t.amount, 0)

  // Offline payments write a capture row too, so cash on delivery reads the same as a card here
  const captured = sum('CAPTURE')
  const refunded = sum('REFUND')
  const tendered = order.giftCardAmount + order.storeCreditAmount
  const collected = captured + tendered
  const due = Math.max(0, order.total - collected)

  const ordered = order.lines.reduce((n, l) => n + l.quantity, 0)
  const shipped = order.fulfillments
    .filter((f) => f.status !== 'CANCELLED')
    .reduce((n, f) => n + f.lines.reduce((m, l) => m + l.quantity, 0), 0)
  const qty = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(2))

  return (
    <div className="divide-y rounded-md border">
      <Group>
        <Row label="Subtotal" value={money(order.subtotal)} />
        {order.discountAmount > 0 &&
          (order.appliedPromotions.length > 0 ? (
            <ExpandableRow label="Discount" value={`-${money(order.discountAmount)}`}>
              {order.appliedPromotions.map((p) => {
                const name = p.ruleName || p.ruleSlug || 'Deleted promotion'
                return (
                  <BreakdownRow
                    key={p.id}
                    title={p.code ? `${name} (${p.code})` : name}
                    label={
                      <>
                        {p.ruleId ? (
                          <Link to={`/promotions/${p.ruleId}`} className="hover:underline">
                            {name}
                          </Link>
                        ) : (
                          name
                        )}
                        {p.code && <span className="ml-1 font-mono">{p.code}</span>}
                      </>
                    }
                    value={`-${money(p.discount)}`}
                  />
                )
              })}
            </ExpandableRow>
          ) : (
            <Row label="Discount" value={`-${money(order.discountAmount)}`} />
          ))}
        <Row label="Shipping" value={money(order.shippingAmount)} />
        {order.taxLines.length > 0 ? (
          <ExpandableRow label="Tax" value={money(order.taxAmount)}>
            {order.taxLines.map((t) => {
              const name =
                (t.name || 'Tax') +
                (t.rateBps > 0 ? ` ${(t.rateBps / 100).toFixed(2).replace(/\.?0+$/, '')}%` : '')
              return <BreakdownRow key={t.id} title={name} label={name} value={money(t.amount)} />
            })}
          </ExpandableRow>
        ) : (
          <Row label="Tax" value={money(order.taxAmount)} />
        )}
      </Group>

      <Group>
        <Row label="Total" value={money(order.total)} tone="total" />
      </Group>

      {!isDraft && (
        <Group>
          <Row
            label="Collected"
            value={money(collected)}
            tone={collected === 0 ? 'muted' : undefined}
          />
          {tendered > 0 && (
            <div className="flex items-baseline justify-between gap-4 pl-3 text-xs text-muted-foreground">
              <span>Gift card or store credit</span>
              <span className="tabular-nums">{money(tendered)}</span>
            </div>
          )}
          {refunded > 0 && <Row label="Refunded" value={money(refunded)} tone="muted" />}
          {due > 0 ? (
            <Row label="Amount due" value={money(due)} tone="due" />
          ) : (
            <Row label="Balance" value="Settled" tone="muted" />
          )}
        </Group>
      )}

      {!isDraft && ordered > 0 && (
        <Group>
          <Row
            label="Fulfilled"
            value={shipped >= ordered ? 'All items' : `${qty(shipped)} of ${qty(ordered)}`}
            tone={shipped === 0 ? 'muted' : undefined}
          />
        </Group>
      )}

      <div className="grid gap-1 px-4 py-3">
        <div className="text-xs uppercase tracking-wider text-muted-foreground">Customer</div>
        <div className="text-sm">
          {order.shippingAddress.firstName} {order.shippingAddress.lastName}
        </div>
        <div className="truncate text-sm text-muted-foreground">{order.email}</div>
        <div className="pt-1">
          {order.customerId ? (
            <Link
              to={`/customers/${order.customerId}`}
              className="text-sm underline underline-offset-2"
            >
              View customer
            </Link>
          ) : (
            <Badge variant="outline" className="text-[10px] uppercase tracking-wider">
              Guest
            </Badge>
          )}
        </div>
        <div className="pt-2 text-xs text-muted-foreground">
          Placed {new Date(order.createdAt).toLocaleString()}
        </div>
      </div>
    </div>
  )
}
