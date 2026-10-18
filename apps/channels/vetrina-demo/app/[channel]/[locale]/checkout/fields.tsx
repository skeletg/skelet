"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useEffect } from "react";

import { Field } from "@/app/components/account/AccountUI";
import { CountrySelect, StateField, type ValueField } from "@/app/components/address/AddressInputs";
import type { CountryOption, RegionOption } from "@/app/lib/countries/read";
import type { FormResult } from "@/app/lib/forms/schema";

// Pieces every checkout step form shares

// A saved step lands back on the step the cart says is next - an edit opened with ?step= would otherwise stay open
export function useReturnToNextStep(lastResult: FormResult | undefined) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  useEffect(() => {
    if (lastResult?.status === "success" && params.has("step")) {
      const next = new URLSearchParams(params.toString());
      next.delete("step");
      router.replace(`${pathname}${next.size ? `?${next}` : ""}`, { scroll: false });
    }
  }, [lastResult, params, pathname, router]);
}

export function ContinueButton({ pending, label }: { pending: boolean; label?: string }) {
  const t = useTranslations("checkout");
  return (
    <button
      type="submit"
      disabled={pending}
      className="mt-2 inline-flex h-11 w-full items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50 sm:w-auto sm:self-end sm:px-8"
    >
      {pending ? t("saving") : (label ?? t("continue"))}
    </button>
  );
}

export type { ValueField };

export { CountrySelect };

type AddressKey =
  | "firstName"
  | "lastName"
  | "company"
  | "street1"
  | "street2"
  | "city"
  | "state"
  | "postalCode"
  | "countryCode";

export type AddressFieldMetas = Record<AddressKey, ValueField> & { phone?: ValueField };

// section prefixes every autocomplete token, so a browser fills the delivery and billing blocks from the right entries.
// The details step asks for the phone once, as contact, so it turns the address's own off
export function AddressFields({
  fields,
  countries,
  regionsByCountry,
  section,
  withPhone = true,
}: {
  fields: AddressFieldMetas;
  countries: CountryOption[];
  regionsByCountry: Record<string, RegionOption[]>;
  section?: "shipping" | "billing";
  withPhone?: boolean;
}) {
  const t = useTranslations("checkout.address");
  const tf = useTranslations("forms.fields");
  const ac = (token: string) => (section ? `${section} ${token}` : token);
  const country = typeof fields.countryCode.value === "string" ? fields.countryCode.value : "";
  return (
    <div className="mt-3 grid gap-3 rounded-lg border border-border p-4">
      <div className="grid grid-cols-2 gap-3">
        <Field label={tf("firstName")} field={fields.firstName} autoComplete={ac("given-name")} required />
        <Field label={tf("lastName")} field={fields.lastName} autoComplete={ac("family-name")} required />
      </div>
      <Field label={t("company")} field={fields.company} autoComplete={ac("organization")} />
      <Field label={t("street1")} field={fields.street1} autoComplete={ac("address-line1")} required />
      <Field label={t("street2")} field={fields.street2} autoComplete={ac("address-line2")} />
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("postalCode")} field={fields.postalCode} autoComplete={ac("postal-code")} required />
        <Field label={t("city")} field={fields.city} autoComplete={ac("address-level2")} required />
      </div>
      <CountrySelect field={fields.countryCode} countries={countries} autoComplete={ac("country")} />
      {/* Keyed by country so switching it clears a region that belonged to the old one */}
      <StateField
        key={country}
        field={fields.state}
        country={country}
        regions={regionsByCountry[country]}
        autoComplete={ac("address-level1")}
      />
      {withPhone && fields.phone ? (
        <Field label={tf("phone")} field={fields.phone} type="tel" phoneCountry={country} autoComplete={ac("tel")} />
      ) : null}
    </div>
  );
}

// A radio card - the selected one is ringed, and the input itself stays a real radio so the form posts it
export function ChoiceCard({
  name,
  value,
  defaultChecked,
  children,
  aside,
}: {
  name: string;
  value: string;
  defaultChecked: boolean;
  children: React.ReactNode;
  aside?: React.ReactNode;
}) {
  return (
    <label className="flex cursor-pointer items-start justify-between gap-3 rounded-lg border border-border p-3 text-sm transition-colors hover:bg-muted has-[:checked]:border-primary has-[:checked]:ring-1 has-[:checked]:ring-primary">
      <span className="flex items-start gap-2">
        <input
          type="radio"
          name={name}
          value={value}
          defaultChecked={defaultChecked}
          className="mt-0.5 size-4 accent-primary"
        />
        <span>{children}</span>
      </span>
      {aside ? <span className="shrink-0 font-medium tabular-nums">{aside}</span> : null}
    </label>
  );
}
