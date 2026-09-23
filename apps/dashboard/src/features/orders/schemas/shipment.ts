import { z } from 'zod'

// Create-shipment dialog - a partial fulfilment of the order's still-unshipped lines, plus optional
// carrier, tracking and insured value

type OrderLine = { id: string; quantity: number }
type Fulfilment = { status: string; lines: { orderLineId: string; quantity: number }[] }

// Ordered minus already shipped (cancelled shipments don't count), per line
export function remainingByLine(lines: OrderLine[], fulfilments: Fulfilment[]): Map<string, number> {
  const shipped = new Map<string, number>()
  for (const f of fulfilments) {
    if (f.status === 'cancelled') continue
    for (const l of f.lines) shipped.set(l.orderLineId, (shipped.get(l.orderLineId) ?? 0) + l.quantity)
  }
  return new Map(lines.map((l) => [l.id, l.quantity - (shipped.get(l.id) ?? 0)]))
}

export const shipmentSchema = z
  .object({
    lines: z.array(
      z
        .object({ id: z.string(), checked: z.boolean(), quantity: z.string(), remaining: z.number() })
        .refine((l) => !l.checked || (Number(l.quantity) > 0 && Number(l.quantity) <= l.remaining), {
          message: 'Between 1 and what is still unshipped',
          path: ['quantity'],
        }),
    ),
    carrier: z.string(),
    tracking: z.string(),
    insured: z.string().refine((v) => v.trim() === '' || (Number.isFinite(Number(v)) && Number(v) >= 0), 'Must be 0 or more'),
  })
  .refine((v) => v.lines.some((l) => l.checked), { message: 'Select at least one line to ship', path: ['lines'] })

export type ShipmentValues = z.infer<typeof shipmentSchema>

// Every still-shippable line, ticked, at its full remainder
export function toShipmentValues(lines: OrderLine[], remaining: Map<string, number>): ShipmentValues {
  return {
    lines: lines
      .filter((l) => (remaining.get(l.id) ?? 0) > 0)
      .map((l) => ({ id: l.id, checked: true, quantity: String(remaining.get(l.id)), remaining: remaining.get(l.id) ?? 0 })),
    carrier: '',
    tracking: '',
    insured: '',
  }
}

export function toShipmentInput(v: ShipmentValues, orderId: string) {
  return {
    orderId,
    lines: v.lines.filter((l) => l.checked).map((l) => ({ orderLineId: l.id, quantity: Number(l.quantity) })),
    carrier: v.carrier.trim() || null,
    trackingNumber: v.tracking.trim() || null,
    insuredValueCents: v.insured.trim() ? Math.round(Number(v.insured) * 100) : null,
  }
}
