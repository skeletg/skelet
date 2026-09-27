"use client";

import { Link } from "@/app/components/LocaleLink";
import { useActionState } from "react";

import {
  Field,
  FormError,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import {
  addAddressAction,
  updateAddressAction,
  type AuthResult,
} from "@/app/lib/auth/actions";

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
  const [state, formAction, pending] = useActionState<
    AuthResult | null,
    FormData
  >(action, null);

  const v = initial ?? {};

  return (
    <form action={formAction} className="flex flex-col gap-3">
      {id && <input type="hidden" name="id" value={id} />}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          name="firstName"
          label="First name"
          required
          defaultValue={v.firstName}
          autoComplete="given-name"
        />
        <Field
          name="lastName"
          label="Last name"
          required
          defaultValue={v.lastName}
          autoComplete="family-name"
        />
      </div>
      <Field
        name="company"
        label="Company (optional)"
        defaultValue={v.company ?? undefined}
        autoComplete="organization"
      />
      <Field
        name="street1"
        label="Street address"
        required
        defaultValue={v.street1}
        autoComplete="address-line1"
      />
      <Field
        name="street2"
        label="Apartment, suite, etc. (optional)"
        defaultValue={v.street2 ?? undefined}
        autoComplete="address-line2"
      />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          name="city"
          label="City"
          required
          defaultValue={v.city}
          autoComplete="address-level2"
        />
        <Field
          name="state"
          label="State / Region"
          defaultValue={v.state ?? undefined}
          autoComplete="address-level1"
        />
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          name="postalCode"
          label="Postal code"
          required
          defaultValue={v.postalCode}
          autoComplete="postal-code"
        />
        <Field
          name="countryCode"
          label="Country code (2-letter)"
          required
          defaultValue={v.countryCode}
          autoComplete="country"
          maxLength={2}
        />
      </div>
      <Field
        name="phone"
        label="Phone (optional)"
        type="tel"
        defaultValue={v.phone ?? undefined}
        autoComplete="tel"
      />

      {state && !state.ok && <FormError>{state.error}</FormError>}

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
