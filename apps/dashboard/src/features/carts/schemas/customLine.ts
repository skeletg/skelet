import { z } from 'zod'
import { money, numberText } from '@/shared/schemas/fields'

// A free-form line on an offer - an extra payment, a service, anything with no catalog product.
// It never ships, and is taxed by the class given here

export const customLineSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required').max(255, 'At most 255 characters').describe('What the line is for, as the recipient reads it'),
    price: money().describe('The unit price, in major units'),
    quantity: numberText({ above: 0 }).describe('How many, as text - usually 1'),
    taxClass: z.string().max(100, 'At most 100 characters').describe('The tax class the line is taxed by, such as standard - empty matches only rules that apply to every product'),
  })
  .describe('A free-form charge added to an offer cart')

export type CustomLineValues = z.infer<typeof customLineSchema>

export const emptyCustomLine: CustomLineValues = { name: '', price: '', quantity: '1', taxClass: 'standard' }
