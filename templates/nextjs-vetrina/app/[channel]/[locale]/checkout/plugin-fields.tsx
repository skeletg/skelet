"use client";

import { useTranslations } from "next-intl";

import { Field, FieldErrors, inputClass } from "@/app/components/account/AccountUI";
import { FIELDS_ASKED, type CheckoutFieldSpec } from "@skelet/storefront-core/checkout/fields";

import type { ValueField } from "./fields";

// What the store's plugins ask besides the step's own fields, each drawn from its declared type - the answers go back
// with the step
export function PluginFields({ id, asked, fields }: { id: string; asked: readonly CheckoutFieldSpec[]; fields: Record<string, ValueField> }) {
  const t = useTranslations("checkout.details");
  if (asked.length === 0) return <input type="hidden" name={FIELDS_ASKED} value="" />;
  return (
    <section className="grid gap-3" aria-labelledby={`${id}-more`}>
      <input type="hidden" name={FIELDS_ASKED} value={asked.map((f) => f.key).join("\n")} />
      <h3 id={`${id}-more`} className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {t("moreDetails")}
      </h3>
      {asked.map((f) => {
        const field = fields[f.key];
        const label = f.required || f.type === "CHECKBOX" ? f.label : t("optional", { label: f.label });
        if (f.type === "TEXT" || f.type === "DATE") {
          return (
            <Field
              key={f.key}
              label={label}
              field={field}
              type={f.type === "DATE" ? "date" : "text"}
              maxLength={f.type === "TEXT" ? f.maxLength : undefined}
              hint={f.hint || undefined}
              required={f.required}
              autoComplete="off"
            />
          );
        }
        return <Control key={f.key} spec={f} label={label} field={field} />;
      })}
    </section>
  );
}

function Control({ spec, label, field }: { spec: CheckoutFieldSpec; label: string; field: ValueField }) {
  const t = useTranslations("checkout.details");
  const errors = field.errors?.length ? field.errors : null;
  const describedBy = [spec.hint ? field.descriptionId : null, errors ? field.errorId : null].filter(Boolean).join(" ") || undefined;
  const initial = typeof field.initialValue === "string" ? field.initialValue : "";
  const shared = {
    id: field.id,
    name: field.name,
    required: spec.required,
    "aria-invalid": errors ? true : undefined,
    "aria-describedby": describedBy,
  };
  const hint = spec.hint ? (
    <span id={field.descriptionId} className="text-xs text-muted-foreground">
      {spec.hint}
    </span>
  ) : null;
  if (spec.type === "CHECKBOX") {
    return (
      <div className="flex flex-col gap-1.5 text-sm">
        <label className="flex items-center gap-2">
          <input key={field.key} type="checkbox" defaultChecked={initial === "on"} className="size-4 rounded accent-primary" {...shared} />
          {label}
        </label>
        {hint}
        {errors ? <FieldErrors id={field.errorId} errors={errors} /> : null}
      </div>
    );
  }
  const className = `${inputClass} w-full ${errors ? "border-red-400" : ""}`;
  return (
    <div className="flex flex-col gap-1.5 text-sm">
      <label htmlFor={field.id} className="font-medium">
        {label}
      </label>
      {spec.type === "SELECT" ? (
        <select key={field.key} defaultValue={initial} className={className} {...shared}>
          <option value="">{t("choose")}</option>
          {spec.options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : (
        <textarea key={field.key} defaultValue={initial} maxLength={spec.maxLength} rows={3} className={className} {...shared} />
      )}
      {hint}
      {errors ? <FieldErrors id={field.errorId} errors={errors} /> : null}
    </div>
  );
}
