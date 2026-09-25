import { z } from 'zod'

// Field rules every form shares - email, password and numbers typed as text - so a schema says what
// a field is rather than how it is checked

// ---------- Email ----------

export const EMAIL_MESSAGE = 'Enter a valid email address'

// Padding is forgiven - the submit mappers trim before sending
export const isEmail = (v: string) => z.email().safeParse(v.trim()).success

export const email = (message = EMAIL_MESSAGE) => z.string().refine(isEmail, message)

// Blank means "none"
export const optionalEmail = (message = EMAIL_MESSAGE) => z.string().refine((v) => v.trim() === '' || isEmail(v), message)

// ---------- Password ----------

// Mirrors the backend's auth.MinPasswordLen so it can be said up front rather than bounced back
export const MIN_PASSWORD_LENGTH = 8
export const PASSWORD_HINT = `At least ${MIN_PASSWORD_LENGTH} characters`

export const newPassword = () => z.string().min(MIN_PASSWORD_LENGTH, PASSWORD_HINT)

// ---------- Numbers ----------

// Numeric inputs bound as numbers (sort order, priority)
export const wholeNumber = (message = 'Whole numbers only') => z.number().int(message)

// Text inputs hold numbers as strings until they're sent - '' is how "not set" is typed
export type NumberTextRule = {
  // '' passes
  optional?: boolean
  int?: boolean
  // Inclusive
  min?: number
  // Exclusive
  above?: number
  // Inclusive
  max?: number
}

// The typed number, or null when it's blank or not a number
export function parseNumberText(v: string): number | null {
  const t = v.trim()
  if (t === '') return null
  const n = Number(t)
  return Number.isFinite(n) ? n : null
}

export function isNumberText(v: string, rule: NumberTextRule = {}): boolean {
  if (rule.optional && v.trim() === '') return true
  const n = parseNumberText(v)
  if (n === null) return false
  if (rule.int && !Number.isInteger(n)) return false
  if (rule.min !== undefined && n < rule.min) return false
  if (rule.above !== undefined && n <= rule.above) return false
  if (rule.max !== undefined && n > rule.max) return false
  return true
}

const countWord = (n: number) => (n === 0 ? 'zero' : n === 1 ? 'one' : String(n))

export function numberTextMessage({ int, min, above, max }: NumberTextRule): string {
  if (min !== undefined && max !== undefined) return int ? `A whole number from ${min} to ${max}` : `Between ${min} and ${max}`
  const bound =
    min !== undefined ? `${countWord(min)} or more` : above !== undefined ? `above ${above}` : max !== undefined ? `${max} or less` : ''
  if (int) return bound ? `A whole number, ${bound}` : 'A whole number'
  return bound ? bound[0].toUpperCase() + bound.slice(1) : 'A number'
}

export const numberText = (rule: NumberTextRule = {}, message = numberTextMessage(rule)) =>
  z.string().refine((v) => isNumberText(v, rule), message)

// ---------- Money ----------

// Typed in major units ("12.50"), stored as integer cents
export const money = (opts: { optional?: boolean } = {}) => numberText({ ...opts, min: 0 })

export const centsToDollars = (cents: number) => (cents / 100).toFixed(2)
export const dollarsToCents = (v: string) => Math.round((parseNumberText(v) ?? 0) * 100)
