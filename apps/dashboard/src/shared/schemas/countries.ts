import { z } from 'zod'

// A comma-separated list of ISO 3166-1 alpha-2 codes, typed into a plain text box - "US, ca,MX".
// Kept as text in the form (that's what the input holds) and parsed on the way out

export function parseCountryList(text: string): string[] {
  const codes = text
    .split(',')
    .map((c) => c.trim().toUpperCase())
    .filter(Boolean)
  return [...new Set(codes)]
}

export function formatCountryList(codes?: readonly string[] | null): string {
  return (codes ?? []).join(', ')
}

export const countryListText = z
  .string()
  .refine((text) => parseCountryList(text).every((c) => /^[A-Z]{2}$/.test(c)), 'Use 2-letter ISO codes, comma-separated')
