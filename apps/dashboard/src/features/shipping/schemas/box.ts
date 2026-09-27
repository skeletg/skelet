import { z } from 'zod'
import { numberText, wholeNumber } from '@/shared/schemas/fields'

// Shipping box dialog - a real carton the packer fits line items into

export type ExistingBox = {
  id: string
  code: string
  name: string
  innerLengthMm: number
  innerWidthMm: number
  innerHeightMm: number
  maxWeightGrams: number
  boxWeightGrams: number
  isActive: boolean
  sortOrder: number
}

// Dimensions are typed into empty boxes on create, so they stay text until they're sent
const millimetres = (what: string) =>
  numberText({ int: true, min: 1 }, 'Whole millimetres, at least 1').describe(`${what} in whole millimetres, at least 1, written as text`)
const grams = (what: string, blank: string) =>
  numberText({ optional: true, int: true, min: 0 }, 'Whole grams').describe(`${what}, in whole grams written as text - empty string or 0 ${blank}`)

export const boxSchema = z
  .object({
    // The upsert key
    code: z.string().trim().min(1, 'Code is required').describe('Stable identifier of the box - set on create, never changed'),
    name: z.string().describe('Display name of the box - empty string for none'),
    length: millimetres('Inner length'),
    width: millimetres('Inner width'),
    height: millimetres('Inner height'),
    maxWeight: grams('Heaviest load the box takes', 'for no limit'),
    boxWeight: grams("The empty carton's own weight, added to the shipment weight", 'for none'),
    sortOrder: wholeNumber().describe('Order the packer tries boxes in - lower first'),
    isActive: z.boolean().describe('Whether the packer can use the box'),
  })
  .describe('A shipping box: a real carton the packer fits order lines into')

export type BoxValues = z.infer<typeof boxSchema>

export function toBoxValues(box?: ExistingBox | null): BoxValues {
  return {
    code: box?.code ?? '',
    name: box?.name ?? '',
    length: box ? String(box.innerLengthMm) : '',
    width: box ? String(box.innerWidthMm) : '',
    height: box ? String(box.innerHeightMm) : '',
    maxWeight: box && box.maxWeightGrams > 0 ? String(box.maxWeightGrams) : '',
    boxWeight: box && box.boxWeightGrams > 0 ? String(box.boxWeightGrams) : '',
    sortOrder: box?.sortOrder ?? 0,
    isActive: box?.isActive ?? true,
  }
}

const whole = (v: string) => (v.trim() ? Number.parseInt(v, 10) : 0)

export function toBoxInput(v: BoxValues) {
  return {
    code: v.code.trim(),
    name: v.name.trim() || undefined,
    innerLengthMm: whole(v.length),
    innerWidthMm: whole(v.width),
    innerHeightMm: whole(v.height),
    maxWeightGrams: whole(v.maxWeight),
    boxWeightGrams: whole(v.boxWeight),
    sortOrder: v.sortOrder,
    isActive: v.isActive,
  }
}
