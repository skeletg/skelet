import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import { addressIssues, addressSchema } from '@/shared/schemas/address'
import { email } from '@/shared/schemas/fields'
import { cleanMetadata, metadataDiff, metadataIssues, metadataSchema, toMetadataValues, type MetadataEntry } from '@/shared/schemas/metadata'

// Customer page - THE field list (docs/conventions.md, "Record forms"): profile and account, the
// internal note, group, tax status, consents and metadata. Addresses are rows with their own add
// form and actions, so they're not in here

export const customerSchema = z
  .object({
    firstName: z.string().trim().min(1, 'First name is required'),
    lastName: z.string().trim().min(1, 'Last name is required'),
    email: email(),
    phone: z.string(),
    isActive: z.boolean(),
    isEmailVerified: z.boolean(),
    note: z.string(),
    groupId: z.string(),
    taxExempt: z.boolean(),
    taxExemptionNote: z.string(),
    vatId: z.string(),
    // Consent kind -> opted in
    consents: z.record(z.string(), z.boolean()),
    metadata: metadataSchema,
  })
  .superRefine((v, ctx) => metadataIssues(v.metadata, 'metadata', ctx))

export type CustomerValues = z.infer<typeof customerSchema>

export const CUSTOMER_LABELS: Record<keyof CustomerValues, string> = {
  firstName: 'First name',
  lastName: 'Last name',
  email: 'Email',
  phone: 'Phone',
  isActive: 'Active',
  isEmailVerified: 'Email verified',
  note: 'Note',
  groupId: 'Group',
  taxExempt: 'Tax exempt',
  taxExemptionNote: 'Exemption note',
  vatId: 'VAT ID',
  consents: 'Communication preferences',
  metadata: 'Metadata',
}

type ExistingCustomer = {
  firstName: string
  lastName: string
  email: string
  phone?: string | null
  isActive: boolean
  isEmailVerified: boolean
  note?: string | null
  group: { id: string }
  taxExempt: boolean
  taxExemptionNote: string
  vatId: string
  consents: { kind: string; optedIn: boolean }[]
  metadata: MetadataEntry[]
}

export function toCustomerValues(c: ExistingCustomer): CustomerValues {
  return {
    firstName: c.firstName,
    lastName: c.lastName,
    email: c.email,
    phone: c.phone ?? '',
    isActive: c.isActive,
    isEmailVerified: c.isEmailVerified,
    note: c.note ?? '',
    groupId: c.group.id,
    taxExempt: c.taxExempt,
    taxExemptionNote: c.taxExemptionNote,
    vatId: c.vatId,
    consents: Object.fromEntries(c.consents.map((k) => [k.kind, k.optedIn])),
    metadata: toMetadataValues(c.metadata),
  }
}

const differs = <K extends keyof CustomerValues>(a: CustomerValues, b: CustomerValues, keys: K[]) =>
  keys.some((k) => JSON.stringify(a[k]) !== JSON.stringify(b[k]))

const PROFILE = ['firstName', 'lastName', 'email', 'phone', 'isActive', 'isEmailVerified'] as const

// The customer's writes, each only when its part changed - profile, note and metadata share the
// one update; group, tax status, VAT ID and each consent have their own (a consent change lands as
// its own source="admin" entry in the audit trail)
export function customerChanges(before: CustomerValues, after: CustomerValues) {
  const update = {
    ...(differs(before, after, [...PROFILE])
      ? {
          firstName: after.firstName.trim(),
          lastName: after.lastName.trim(),
          email: after.email.trim(),
          phone: after.phone.trim() || null,
          isActive: after.isActive,
          isEmailVerified: after.isEmailVerified,
        }
      : {}),
    ...(before.note !== after.note ? { note: after.note } : {}),
    ...(metadataDiff(before.metadata, after.metadata).length > 0 ? { metadata: cleanMetadata(after.metadata) } : {}),
  }
  return {
    update: Object.keys(update).length > 0 ? update : null,
    groupId: before.groupId !== after.groupId ? after.groupId : null,
    taxStatus:
      before.taxExempt !== after.taxExempt || before.taxExemptionNote.trim() !== after.taxExemptionNote.trim()
        ? { taxExempt: after.taxExempt, note: after.taxExemptionNote.trim() || null }
        : null,
    vatId: before.vatId.trim() !== after.vatId.trim() ? after.vatId.trim() : null,
    consents: Object.keys(after.consents)
      .filter((kind) => before.consents[kind] !== after.consents[kind])
      .map((kind) => ({ kind, optedIn: after.consents[kind] })),
  }
}

// Long values (notes, metadata blobs) stay readable in the confirm dialog
const clip = (s: string, max = 80) => (s.length > max ? `${s.slice(0, max - 1)}…` : s)
const optIn = (on: boolean | undefined) => (on ? 'opted in' : 'opted out')

export function customerDiff(names: { group: (id: string) => string; consent: (kind: string) => string }) {
  return (original: CustomerValues, draft: CustomerValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      ...PROFILE.map((k) => ({ label: CUSTOMER_LABELS[k], value: (v: CustomerValues) => v[k] })),
      { label: CUSTOMER_LABELS.note, value: (v) => clip(v.note) },
      { label: CUSTOMER_LABELS.groupId, value: (v) => names.group(v.groupId) },
      { label: CUSTOMER_LABELS.taxExempt, value: (v) => v.taxExempt },
      { label: CUSTOMER_LABELS.taxExemptionNote, value: (v) => v.taxExemptionNote },
      { label: CUSTOMER_LABELS.vatId, value: (v) => v.vatId.trim() || '(none)' },
    ]),
    ...customerChanges(original, draft)
      .consents.map((c) => changed(names.consent(c.kind), optIn(original.consents[c.kind]), optIn(c.optedIn)))
      .filter((e) => e !== null),
    ...metadataDiff(original.metadata, draft.metadata),
  ]
}

// The add-address row form - the backend's required set, names included. Nested under `address` so
// the shared AddressFields group can render it
export const newAddressSchema = z
  .object({ address: addressSchema })
  .superRefine((v, ctx) => addressIssues(v.address, ['address'], ctx))
