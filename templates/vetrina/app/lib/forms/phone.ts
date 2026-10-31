import {
  AsYouType,
  getCountryCallingCode,
  isValidPhoneNumber,
  parsePhoneNumberFromString,
  type CountryCode,
} from "libphonenumber-js/min";
import type { z } from "zod";

import type { PhoneSettings } from "@/app/lib/countries/read";

// Phone rules come from libphonenumber's metadata, the same data the backend validates with - so the form refuses
// exactly what the backend would. Kept out of forms/schema.ts so forms without a phone don't carry the metadata
//
// The country only reads a nationally typed number ("064 123 4567"); one typed with its + validates whatever it is,
// so a shopper in Serbia can still give a German mobile. The backend stores E.164 either way

function asCountry(country: unknown): CountryCode | undefined {
  return typeof country === "string" && /^[A-Z]{2}$/.test(country) ? (country as CountryCode) : undefined;
}

export function isValidPhone(value: string, country: unknown): boolean {
  try {
    return isValidPhoneNumber(value, asCountry(country));
  } catch {
    // A country code the metadata doesn't know
    return false;
  }
}

// The country a phone was read as - "" when it can't be read at all
function countryOf(value: string, country: unknown): string {
  try {
    return parsePhoneNumberFromString(value, asCountry(country))?.country ?? "";
  } catch {
    return "";
  }
}

// An object-level check, since a phone can't be judged without its country. PhoneInput posts two companions beside
// the phone: <key>Country, the country a national number is read in, and <key>Allowed, the store's countries when it
// keeps phones to them. A form without them (no JS yet, a test) falls back to fallbackCountryKey - an address's own
// country. Blank passes, so optional stays optional
export function refinePhone(phoneKey: string, fallbackCountryKey?: string) {
  return (v: Record<string, unknown>, ctx: z.RefinementCtx) => {
    const phone = v[phoneKey];
    if (typeof phone !== "string" || phone === "") return;
    const country = v[`${phoneKey}Country`] || (fallbackCountryKey ? v[fallbackCountryKey] : undefined);
    if (!isValidPhone(phone, country)) {
      ctx.addIssue({ code: "custom", path: [phoneKey], message: "phoneInvalid" });
      return;
    }
    const allowed = splitCodes(v[`${phoneKey}Allowed`]);
    if (allowed.length > 0 && !allowed.includes(countryOf(phone, country))) {
      ctx.addIssue({ code: "custom", path: [phoneKey], message: "phoneCountryNotAllowed" });
    }
  };
}

function splitCodes(raw: unknown): string[] {
  return typeof raw === "string" ? raw.split(",").map((c) => c.trim()).filter(Boolean) : [];
}

// How one phone field behaves under the store's policy: the country a national number is read in, whether that's
// fixed, the example to show, and the countries a number may be from (empty for any)
export type PhoneFieldSetup = {
  country: string;
  locked: boolean;
  example?: string;
  allowed: string[];
};

// formCountry is the form's own country - an address's - and loses to a store locked to one country
export function phoneFieldFor(settings: PhoneSettings | undefined, formCountry: string): PhoneFieldSetup {
  const country = settings?.lockedCountryCode ?? (formCountry || settings?.defaultCountryCode || "");
  return {
    country,
    locked: Boolean(settings?.lockedCountryCode),
    example: settings?.formats.find((f) => f.countryCode === country)?.nationalExample,
    allowed: settings?.restrictedToStoreCountries ? settings.formats.map((f) => f.countryCode) : [],
  };
}

// Live formatting while typing - "0641234567" reads as "064 1234567" in RS, "+4915123456789" as "+49 1512 3456789"
export function formatAsTyped(value: string, country: string): string {
  // Deleting back over a formatting character would otherwise snap straight back
  if (!/\d/.test(value)) return value;
  return new AsYouType(asCountry(country)).input(value);
}

// A stored number as typed at home when it's from that country, else international
export function formatPhoneNational(value: string, country: string): string {
  const n = parsePhoneNumberFromString(value);
  if (!n) return value;
  return n.country === country ? n.formatNational() : n.formatInternational();
}

// For display - a stored E.164 number in its international form, anything unparseable (older free-text rows) as it is
export function formatPhone(value: string): string {
  return parsePhoneNumberFromString(value)?.formatInternational() ?? value;
}

// "+381" for RS - the input's placeholder, so the shopper sees which country a national number is read in
export function callingCode(country: string): string | null {
  const c = asCountry(country);
  if (!c) return null;
  try {
    return `+${getCountryCallingCode(c)}`;
  } catch {
    return null;
  }
}
