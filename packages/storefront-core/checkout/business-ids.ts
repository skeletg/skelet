// Company identifiers on a business purchase. Which ones are asked for follows the company's country - Serbia's PIB and
// Matični broj, a VAT number elsewhere - as the store defines them per country in the dashboard. A country with none
// falls back to one generic tax ID, per sales/checkout/tax_id_field. The backend applies the same rules

import type { BusinessIdField, CountryOption } from "../countries/read";

import type { CheckoutSettings } from "./settings";

// The generic field's key - its label is the storefront's own "Tax ID", in the shopper's language
export const GENERIC_TAX_ID = "tax_id";

export function businessIdFieldsFor(
  countries: CountryOption[],
  countryCode: string,
  s: CheckoutSettings,
): BusinessIdField[] {
  const own = countries.find((c) => c.code === countryCode)?.businessIdFields ?? [];
  if (own.length > 0) return own;
  if (s.taxId === "hidden") return [];
  return [{ key: GENERIC_TAX_ID, label: "", required: s.taxId === "required", pattern: "", hint: "" }];
}

// The whole value must match, anchored or not - the same reading as the backend's RE2 check. A pattern the browser
// can't compile passes here, and the backend has the last word
export function matchesPattern(pattern: string, value: string): boolean {
  if (!pattern) return true;
  try {
    return new RegExp(`^(?:${pattern.replace(/^\^/, "").replace(/\$$/, "")})$`).test(value);
  } catch {
    return true;
  }
}

// The country the identifiers follow - where the company is billed, else where the order goes
export function companyCountry(o: {
  billingCountry: string | null;
  deliveryCountry: string | null;
  destinationCountry: string;
}): string {
  return o.billingCountry || o.deliveryCountry || o.destinationCountry;
}
