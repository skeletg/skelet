import { z } from 'zod'

// A postal address as the forms edit it - every field a string, optional ones '' rather than null.
// The required set mirrors the backend's validEditAddress (names, street, city, postal code and a
// 2-letter country)

export const addressSchema = z.object({
  firstName: z.string(),
  lastName: z.string(),
  company: z.string(),
  street1: z.string(),
  street2: z.string(),
  city: z.string(),
  state: z.string(),
  postalCode: z.string(),
  countryCode: z.string(),
  phone: z.string(),
})

export type AddressValues = z.infer<typeof addressSchema>

export const ADDRESS_FIELDS: { key: keyof AddressValues; label: string; required?: boolean; half?: boolean }[] = [
  { key: 'firstName', label: 'First name', required: true, half: true },
  { key: 'lastName', label: 'Last name', required: true, half: true },
  { key: 'company', label: 'Company' },
  { key: 'street1', label: 'Street line 1', required: true },
  { key: 'street2', label: 'Street line 2' },
  { key: 'city', label: 'City', required: true, half: true },
  { key: 'state', label: 'State / region', half: true },
  { key: 'postalCode', label: 'Postal code', required: true, half: true },
  { key: 'countryCode', label: 'Country', required: true, half: true },
  { key: 'phone', label: 'Phone' },
]

type StoredAddress = {
  firstName: string
  lastName: string
  company?: string | null
  street1: string
  street2?: string | null
  city: string
  state?: string | null
  postalCode: string
  countryCode: string
  phone?: string | null
}

export function toAddressValues(a?: StoredAddress | null): AddressValues {
  return {
    firstName: a?.firstName ?? '',
    lastName: a?.lastName ?? '',
    company: a?.company ?? '',
    street1: a?.street1 ?? '',
    street2: a?.street2 ?? '',
    city: a?.city ?? '',
    state: a?.state ?? '',
    postalCode: a?.postalCode ?? '',
    countryCode: a?.countryCode ?? '',
    phone: a?.phone ?? '',
  }
}

// Optional fields go over the wire as null when blank - the stored snapshot keeps them unset rather
// than as empty strings
export function toAddressInput(a: AddressValues) {
  return {
    firstName: a.firstName.trim(),
    lastName: a.lastName.trim(),
    company: a.company.trim() || null,
    street1: a.street1.trim(),
    street2: a.street2.trim() || null,
    city: a.city.trim(),
    state: a.state.trim() || null,
    postalCode: a.postalCode.trim(),
    countryCode: a.countryCode.trim().toUpperCase(),
    phone: a.phone.trim() || null,
  }
}

export function addressChanged(a: AddressValues, b: AddressValues): boolean {
  return ADDRESS_FIELDS.some(({ key }) => a[key].trim() !== b[key].trim())
}

// The backend's required set, as issues on the address's own fields - `prefix` is where the address
// sits in the form (['shipping'] -> shipping.firstName)
export function addressIssues(
  a: AddressValues,
  prefix: (string | number)[],
  ctx: z.RefinementCtx,
  opts: { requireNames?: boolean } = {},
) {
  const requireNames = opts.requireNames ?? true
  for (const { key, label, required } of ADDRESS_FIELDS) {
    if (!requireNames && (key === 'firstName' || key === 'lastName')) continue
    if (required && !a[key].trim()) ctx.addIssue({ code: 'custom', path: [...prefix, key], message: `${label} is required` })
  }
  if (a.countryCode.trim() && a.countryCode.trim().length !== 2) {
    ctx.addIssue({ code: 'custom', path: [...prefix, 'countryCode'], message: 'Use a 2-letter country code' })
  }
}
