"use client";

import { useTranslations } from "next-intl";

import { FieldErrors, inputClass, type FieldMeta } from "@/app/components/account/AccountUI";
import type { CountryOption, RegionOption } from "@skelet/storefront-next/countries/read";
import { subdivisionsFor } from "@skelet/storefront-next/states";

// The address inputs that need more than a text box - shared by the checkout's address step and the account address
// book, so both pick a country from the store's list and a region from the country's own. Labels come from
// checkout.address, where the storefront keeps its address wording

export type ValueField = FieldMeta & { value?: unknown };

// autoComplete takes a section prefix where a form holds two addresses ("shipping country", "billing country")
export function CountrySelect({
  field,
  countries,
  autoComplete = "country",
}: {
  field: ValueField;
  countries: CountryOption[];
  autoComplete?: string;
}) {
  const t = useTranslations("checkout.address");
  const errors = field.errors?.length ? field.errors : null;
  return (
    <div className="flex flex-col gap-1.5 text-sm">
      <label htmlFor={field.id} className="font-medium">
        {t("country")}
      </label>
      <select
        key={field.key}
        id={field.id}
        name={field.name}
        required
        autoComplete={autoComplete}
        defaultValue={typeof field.initialValue === "string" ? field.initialValue : ""}
        aria-invalid={errors ? true : undefined}
        aria-describedby={errors ? field.errorId : undefined}
        className={`${inputClass} w-full ${errors ? "border-red-400" : ""}`}
      >
        <option value="" disabled>
          {t("countryPlaceholder")}
        </option>
        {countries.map((c) => (
          <option key={c.code} value={c.code}>
            {c.emojiFlag ? `${c.emojiFlag} ` : ""}
            {c.name}
          </option>
        ))}
      </select>
      {errors ? <FieldErrors id={field.errorId} errors={errors} /> : null}
    </div>
  );
}

// Countries with seeded subdivisions get a dropdown, everything else free text
export function StateField({
  field,
  country,
  regions,
  autoComplete = "address-level1",
}: {
  field: ValueField;
  country: string;
  regions?: RegionOption[];
  autoComplete?: string;
}) {
  const t = useTranslations("checkout.address");
  const subs = regions && regions.length > 0 ? regions : subdivisionsFor(country);
  const initial = typeof field.initialValue === "string" ? field.initialValue : "";
  const errors = field.errors?.length ? field.errors : null;
  const common = {
    id: field.id,
    name: field.name,
    autoComplete,
    "aria-invalid": errors ? true : undefined,
    "aria-describedby": errors ? field.errorId : undefined,
    className: `${inputClass} w-full disabled:opacity-50 ${errors ? "border-red-400" : ""}`,
  } as const;
  return (
    <div className="flex flex-col gap-1.5 text-sm">
      <label htmlFor={field.id} className="font-medium">
        {t("state")}
      </label>
      {subs ? (
        <select {...common} defaultValue={subs.some((s) => s.code === initial) ? initial : ""}>
          <option value="">{t("statePlaceholder")}</option>
          {subs.map((s) => (
            <option key={s.code} value={s.code}>
              {s.name}
            </option>
          ))}
        </select>
      ) : (
        <input
          {...common}
          type="text"
          defaultValue={initial}
          disabled={!country}
          placeholder={country ? "" : t("countryFirst")}
        />
      )}
      {errors ? <FieldErrors id={field.errorId} errors={errors} /> : null}
    </div>
  );
}
