"use client";

import type { FormEvent, InputHTMLAttributes } from "react";

import { callingCode, formatAsTyped, formatPhone, formatPhoneNational, type PhoneFieldSetup } from "@skelet/storefront-core/forms/phone";

// A stored E.164 number shows international - or at home, beside a locked store's fixed prefix
function formatStored(value: string, lockedCountry: string): string {
  return lockedCountry ? formatPhoneNational(value, lockedCountry) : formatPhone(value);
}

// Its own client island, like PasswordInput. Uncontrolled so Conform keeps owning the value - it only rewrites the
// text in place, and only while typing at the end: reformatting mid-edit would throw the caret to the end, and
// reformatting a deletion would put back the bracket or space the shopper just removed
//
// The country's calling code sits fixed beside the box, so the shopper types the number the way they'd dial it at home
// ("064 1234567", "(201) 555-0123") and it formats as they go - a number typed with its own + still works, for a phone
// from another country
export function PhoneInput({
  setup,
  defaultValue,
  placeholder,
  className = "",
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "onInput"> & { setup: PhoneFieldSetup }) {
  const { country, locked, example, allowed } = setup;
  const onInput = (e: FormEvent<HTMLInputElement>) => {
    const el = e.currentTarget;
    const inputType = (e.nativeEvent as InputEvent).inputType ?? "";
    if (inputType.startsWith("delete") || el.selectionStart !== el.value.length) return;
    const next = formatAsTyped(el.value, country);
    if (next !== el.value) el.value = next;
  };
  const prefix = locked ? callingCode(country) : null;
  // A locked store shows the prefix beside the box, so the example is the number as typed at home
  const hint = placeholder ?? example ?? callingCode(country) ?? undefined;
  const input = (
    <input
      {...props}
      type="tel"
      inputMode="tel"
      defaultValue={typeof defaultValue === "string" ? formatStored(defaultValue, locked ? country : "") : defaultValue}
      placeholder={hint}
      onInput={onInput}
      className={prefix ? `${className} rounded-l-none` : className}
    />
  );
  return (
    <>
      {prefix ? (
        <div className="flex">
          <span
            aria-hidden
            className="inline-flex items-center rounded-l-md border border-r-0 border-border bg-muted px-3 text-sm text-muted-foreground"
          >
            {prefix}
          </span>
          {input}
        </div>
      ) : (
        input
      )}
      {/* What the schema and the backend read the number with - see refinePhone */}
      <input type="hidden" name={`${props.name}Country`} value={country} />
      {allowed.length > 0 ? <input type="hidden" name={`${props.name}Allowed`} value={allowed.join(",")} /> : null}
    </>
  );
}
