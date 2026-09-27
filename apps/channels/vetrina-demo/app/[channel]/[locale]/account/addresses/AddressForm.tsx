"use client";

import { Link } from "@/app/components/LocaleLink";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { addAddressAction, updateAddressAction } from "@/app/lib/auth/actions";
import { addressSchema } from "@/app/lib/auth/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export type AddressInitial = {
  firstName?: string;
  lastName?: string;
  company?: string | null;
  street1?: string;
  street2?: string | null;
  city?: string;
  state?: string | null;
  postalCode?: string;
  countryCode?: string;
  phone?: string | null;
};

type Props = {
  // When `id` is set the form posts to updateAddressAction; otherwise it creates a new
  // address via addAddressAction
  id?: string;
  initial?: AddressInitial;
  submitLabel: string;
};

export function AddressForm({ id, initial, submitLabel }: Props) {
  const action = id ? updateAddressAction : addAddressAction;
  const v = initial ?? {};
  const { form, fields, formAction, pending } = useActionForm(
    action,
    addressSchema,
    {
      defaultValue: {
        firstName: v.firstName,
        lastName: v.lastName,
        company: v.company ?? undefined,
        street1: v.street1,
        street2: v.street2 ?? undefined,
        city: v.city,
        state: v.state ?? undefined,
        postalCode: v.postalCode,
        countryCode: v.countryCode,
        phone: v.phone ?? undefined,
      },
    },
  );

  return (
    <form
      id={form.id}
      onSubmit={form.onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-3"
    >
      {id && <input type="hidden" name="id" value={id} />}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          field={fields.firstName}
          label="First name"
          required
          autoComplete="given-name"
        />
        <Field
          field={fields.lastName}
          label="Last name"
          required
          autoComplete="family-name"
        />
      </div>
      <Field
        field={fields.company}
        label="Company (optional)"
        autoComplete="organization"
      />
      <Field
        field={fields.street1}
        label="Street address"
        required
        autoComplete="address-line1"
      />
      <Field
        field={fields.street2}
        label="Apartment, suite, etc. (optional)"
        autoComplete="address-line2"
      />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          field={fields.city}
          label="City"
          required
          autoComplete="address-level2"
        />
        <Field
          field={fields.state}
          label="State / Region"
          autoComplete="address-level1"
        />
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          field={fields.postalCode}
          label="Postal code"
          required
          autoComplete="postal-code"
        />
        <Field
          field={fields.countryCode}
          label="Country code (2-letter)"
          required
          autoComplete="country"
          maxLength={2}
        />
      </div>
      <Field
        field={fields.phone}
        label="Phone (optional)"
        type="tel"
        autoComplete="tel"
      />

      <FormErrors errors={form.errors} />

      <div className="mt-2 flex items-center gap-4">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? "Saving…" : submitLabel}
        </button>
        <Link
          href="/account/addresses"
          className="text-sm text-muted-foreground hover:underline"
        >
          Cancel
        </Link>
      </div>
    </form>
  );
}
