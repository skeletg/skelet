// What the store's plugins ask at checkout - the backend declares them and holds the answers to the same rules

import type { CheckoutState } from "./read";

export type CheckoutFieldSpec = CheckoutState["fields"][number];

// The answer as the backend takes it - a ticked box is "true", an unticked one empty
export function fieldAnswer(f: CheckoutFieldSpec, raw: string | null | undefined): string {
  const v = (raw ?? "").trim();
  if (f.type === "CHECKBOX") return v === "on" || v === "true" ? "true" : "";
  return v;
}

function isDate(v: string): boolean {
  const d = new Date(`${v}T00:00:00Z`);
  return /^\d{4}-\d{2}-\d{2}$/.test(v) && !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === v;
}

export function fieldIssue(f: CheckoutFieldSpec, raw: string | null | undefined): string | null {
  const v = fieldAnswer(f, raw);
  if (!v) return f.required ? (f.type === "CHECKBOX" ? "fieldTick" : "required") : null;
  switch (f.type) {
    case "SELECT":
      return f.options.includes(v) ? null : "fieldInvalid";
    case "DATE":
      return isDate(v) ? null : "fieldInvalid";
    case "CHECKBOX":
      return null;
    default:
      return [...v].length > f.maxLength ? "checkoutFieldTooLong" : null;
  }
}

// A required field without an answer keeps the details step open - a plugin may have started asking since it was saved
export function fieldsComplete(fields: readonly CheckoutFieldSpec[]): boolean {
  return !fields.some((f) => f.required && !f.value);
}
