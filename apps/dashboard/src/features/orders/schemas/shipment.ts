import { z } from 'zod'
import { dollarsToCents, isNumberText, money } from '@/shared/schemas/fields'

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
    lines: z
      .array(
        z
          .object({
            id: z.string().describe('ID of the order line'),
            checked: z.boolean().describe('Include the line in this shipment'),
            quantity: z.string().describe('Units to ship, from 1 up to remaining, written as text'),
            remaining: z.number().describe('Units of the line not yet shipped'),
          })
          .refine((l) => !l.checked || isNumberText(l.quantity, { above: 0, max: l.remaining }), {
            message: 'Between 1 and what is still unshipped',
            path: ['quantity'],
          }),
      )
      .describe("The order's unshipped lines - at least one must be checked"),
    carrier: z.string().describe('Shipping carrier, such as UPS - empty string for none'),
    tracking: z.string().describe('Tracking number - empty string for none'),
    insured: money({ optional: true }).describe(
      'Insured value in major currency units written as text, such as "12.50" - empty string for none',
    ),
  })
  .describe("A shipment fulfilling some or all of an order's unshipped lines")
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
    insuredValueCents: v.insured.trim() ? dollarsToCents(v.insured) : null,
  }
}
