import { AsYouType, getCountryCallingCode, isValidPhoneNumber, parsePhoneNumberFromString, type CountryCode } from 'libphonenumber-js/min'

// Phone rules come from libphonenumber's metadata, the same data the backend validates with (lib/phone) - so a form
// refuses exactly what a save would. The backend stores E.164 ("+4915123456789")
//
// A country only reads a nationally typed number; one typed with its + is judged on its own, whatever the country

function asCountry(country: string | undefined): CountryCode | undefined {
  const c = country?.trim().toUpperCase()
  return c && /^[A-Z]{2}$/.test(c) ? (c as CountryCode) : undefined
}

// The message to show, or null when the phone is blank or valid. With no country (a customer's own phone) a national
// number can't be judged here - the backend reads it in the channel's default country - so only a +number is checked
export function phoneIssue(value: string, country?: string): string | null {
  const v = value.trim()
  if (!v) return null
  const c = asCountry(country)
  if (!c && !v.startsWith('+')) return null
  try {
    if (isValidPhoneNumber(v, c)) return null
  } catch {
    // A country code the metadata doesn't know
  }
  return c ? `Not a valid phone number for ${c} - or start it with + and the country code` : 'Not a valid phone number'
}

// Live formatting while typing - "015123456789" reads as "01512 3456789" in DE, "+33612345678" as "+33 6 12 34 56 78"
export function formatAsTyped(value: string, country?: string): string {
  if (!/\d/.test(value)) return value
  return new AsYouType(asCountry(country)).input(value)
}

// For display - a stored E.164 number in its international form, anything unparseable (older free-text rows) as it is
export function formatPhone(value: string): string {
  return parsePhoneNumberFromString(value)?.formatInternational() ?? value
}

// A country's "+49", as the backend derives it (lib/phone.CallingCode) - null for a code the metadata doesn't know
export function callingCode(country: string): string | null {
  const c = asCountry(country)
  if (!c) return null
  try {
    return `+${getCountryCallingCode(c)}`
  } catch {
    return null
  }
}
