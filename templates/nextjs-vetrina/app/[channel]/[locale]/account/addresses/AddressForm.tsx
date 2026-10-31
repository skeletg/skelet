"use client";

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { CountrySelect, StateField } from "@/app/components/address/AddressInputs";
import { addAddressAction, updateAddressAction } from "@skelet/storefront-next/auth/actions";
import { addressSchema } from "@skelet/storefront-next/auth/schemas";
import type { AddressFormOptions } from "@skelet/storefront-next/countries/read";
import { phoneFieldFor } from "@skelet/storefront-next/forms/phone";
import { useActionForm } from "@skelet/storefront-next/forms/use-action-form";

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
  // The store's countries, their regions and the country a new address starts on
  options: AddressFormOptions;
};

export function AddressForm({ id, initial, submitLabel, options }: Props) {
  const action = id ? updateAddressAction : addAddressAction;
  const t = useTranslations("account.addressForm");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const v = initial ?? {};
  const { form, fields, formAction, onSubmit, pending } = useActionForm(
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
        countryCode: v.countryCode ?? (options.defaultCountry || undefined),
        phone: v.phone ?? undefined,
      },
    },
  );

  const country = typeof fields.countryCode.value === "string" ? fields.countryCode.value : "";

  return (
    <form
      id={form.id}
      onSubmit={onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-3"
    >
      {id && <input type="hidden" name="id" value={id} />}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          field={fields.firstName}
          label={tf("firstName")}
          required
          autoComplete="given-name"
        />
        <Field
          field={fields.lastName}
          label={tf("lastName")}
          required
          autoComplete="family-name"
        />
      </div>
      <Field
        field={fields.company}
        label={t("company")}
        autoComplete="organization"
      />
      <Field
        field={fields.street1}
        label={t("street1")}
        required
        autoComplete="address-line1"
      />
      <Field
        field={fields.street2}
        label={t("street2")}
        autoComplete="address-line2"
      />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field
          field={fields.city}
          label={t("city")}
          required
          autoComplete="address-level2"
        />
        <Field
          field={fields.postalCode}
          label={t("postalCode")}
          required
          autoComplete="postal-code"
        />
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <CountrySelect field={fields.countryCode} countries={options.countries} />
        {/* Keyed by country so switching it clears a region that belonged to the old one */}
        <StateField key={country} field={fields.state} country={country} regions={options.regionsByCountry[country]} />
      </div>
      <Field
        field={fields.phone}
        label={t("phone")}
        type="tel"
        phone={phoneFieldFor(options.phone, country)}
        autoComplete="tel"
      />

      <FormErrors errors={form.errors} />

      <div className="mt-2 flex items-center gap-4">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? tc("saving") : submitLabel}
        </button>
        <Link
          href="/account/addresses"
          className="text-sm text-muted-foreground hover:underline"
        >
          {tc("cancel")}
        </Link>
      </div>
    </form>
  );
}
