import { z } from 'zod'
import { isNumberText } from '@/shared/schemas/fields'

// Edit-order dialog - pre-fulfillment line changes. The service recomputes totals and re-reserves
// stock (a shortage aborts the edit); bundle line quantities are refused server-side

type OrderLine = { id: string; quantity: number; productName: string; productSku: string }

const positive = (v: string) => isNumberText(v, { above: 0 })

export const editOrderSchema = z
  .object({
    lines: z
      .array(
        z
          .object({
            id: z.string().describe('ID of the order line'),
            quantity: z.string().describe('New quantity, above 0, written as text'),
            removed: z.boolean().describe('Drop the line from the order'),
          })
          // 0 used to be silently ignored - a line is dropped with its remove button, not by zeroing it
          .refine((l) => l.removed || positive(l.quantity), { message: 'Above 0 - use remove to drop the line', path: ['quantity'] }),
      )
      .describe("The order's existing lines"),
    adds: z
      .array(
        z
          .object({
            sku: z.string().describe('SKU of the product to add - rows with an empty SKU are ignored'),
            quantity: z.string().describe('Units to add, above 0, written as text'),
          })
          .refine((a) => a.sku.trim() === '' || positive(a.quantity), { message: 'Above 0', path: ['quantity'] }),
      )
      .describe('New lines to add'),
  })
  .describe("Changes to a placed order's lines before fulfilment - totals and stock reservations are recomputed")

export type EditOrderValues = z.infer<typeof editOrderSchema>

export function toEditOrderValues(lines: OrderLine[]): EditOrderValues {
  return { lines: lines.map((l) => ({ id: l.id, quantity: String(l.quantity), removed: false })), adds: [] }
}

export function toEditOrderInput(v: EditOrderValues, lines: OrderLine[]) {
  const stored = new Map(lines.map((l) => [l.id, l.quantity]))
  return {
    updateLines: v.lines
      .filter((l) => !l.removed && Number(l.quantity) !== stored.get(l.id))
      .map((l) => ({ orderLineId: l.id, quantity: Number(l.quantity) })),
    removeLines: v.lines.filter((l) => l.removed).map((l) => l.id),
    addLines: v.adds
      .filter((a) => a.sku.trim() !== '')
      .map((a) => ({ sku: a.sku.trim(), quantity: Number(a.quantity) })),
  }
}

export function hasOrderChanges(input: ReturnType<typeof toEditOrderInput>): boolean {
  return input.updateLines.length + input.removeLines.length + input.addLines.length > 0
}
