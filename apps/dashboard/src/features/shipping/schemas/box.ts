import { z } from 'zod'

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
const millimetres = z
  .string()
  .refine((v) => /^\d+$/.test(v.trim()) && Number(v) >= 1, 'Whole millimetres, at least 1')
// Blank means 0 - no limit for the load, nothing added for the carton
const grams = z.string().refine((v) => v.trim() === '' || /^\d+$/.test(v.trim()), 'Whole grams')

export const boxSchema = z.object({
  // The upsert key - fixed after creation
  code: z.string().trim().min(1, 'Code is required'),
  name: z.string(),
  length: millimetres,
  width: millimetres,
  height: millimetres,
  maxWeight: grams,
  boxWeight: grams,
  sortOrder: z.number().int('Whole numbers only'),
  isActive: z.boolean(),
})

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
