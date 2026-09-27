// Presentational pieces shared between the server-rendered SIMPLE PDP and the client-side ConfigurablePDP. All pure (no
// hooks, no state), so importing from either a Server or Client Component works

import type { ReactNode } from "react";
import { Link } from "@/app/components/LocaleLink";
import { ArrowUp, Check, RotateCcw, ShieldCheck, Truck } from "lucide-react";

import type { ProductAttributeValue } from "@/app/lib/catalog/read";
import { renderJsonText } from "@/app/lib/json-text";

// Interactive gallery lives in a "use client" file so the SSR'd server components can
// still import + render it as a child
export { ProductGallery } from "./Gallery";

// Accented callout that sits right below the Add-to-cart button when the buy is blocked on a shopper choice - no
// variant picked, a required option unset, nothing selected yet
export function SelectionHint({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      role="alert"
      className={`mt-3 flex items-center gap-2.5 rounded-lg border border-destructive/40 bg-destructive/10 px-3.5 py-2.5 text-sm font-medium text-foreground animate-in fade-in slide-in-from-top-1 duration-300 ${className}`}
    >
      <ArrowUp
        aria-hidden
        className="size-4 shrink-0 text-destructive motion-safe:animate-[nudge-up_1.5s_ease-in-out_infinite]"
      />
      <span>{children}</span>
    </div>
  );
}

export function Description({
  label,
  value,
  className,
}: {
  label: string;
  value: unknown;
  className?: string;
}) {
  const rendered = renderJsonText(value);
  if (!rendered) return null;
  return (
    <section className={className}>
      <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </h2>
      <div className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
        {rendered}
      </div>
    </section>
  );
}

export function AttributeList({
  values,
  className,
}: {
  values: ProductAttributeValue[];
  className?: string;
}) {
  if (values.length === 0) return null;
  return (
    <section className={className}>
      <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        Specifications
      </h2>
      <dl className="mt-3 overflow-hidden rounded-xl border border-border/60 text-sm">
        {values.map((av, i) => (
          <div
            key={av.attribute.id}
            className={`grid grid-cols-3 gap-4 px-4 py-2.5 ${
              i % 2 === 0 ? "bg-muted/30" : ""
            }`}
          >
            <dt className="col-span-1 text-muted-foreground">
              {av.attribute.name}
            </dt>
            <dd className="col-span-2 font-medium text-foreground">
              {av.references && av.references.length > 0 ? (
                <span className="flex flex-wrap gap-x-2 gap-y-1">
                  {av.references.map((ref, j) => (
                    <span key={ref.targetId}>
                      <Link
                        href={ref.url}
                        className="underline underline-offset-2 hover:text-muted-foreground"
                      >
                        {ref.label}
                      </Link>
                      {j < av.references.length - 1 ? "," : ""}
                    </span>
                  ))}
                </span>
              ) : (
                <>
                  {formatAttribute(av)}
                  {av.attribute.unit && (
                    <span className="ml-1 font-normal text-muted-foreground">
                      {av.attribute.unit}
                    </span>
                  )}
                </>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export function StockBadge({
  inStock,
  salableQuantity,
}: {
  inStock: boolean;
  salableQuantity: number | null;
}) {
  return (
    <div className="mt-4 flex items-center gap-2.5 text-sm">
      <span
        className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium ${
          inStock
            ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
            : "bg-muted text-muted-foreground"
        }`}
      >
        <span
          className={`h-1.5 w-1.5 rounded-full ${
            inStock ? "bg-emerald-500" : "bg-muted-foreground"
          }`}
          aria-hidden="true"
        />
        {inStock ? "In stock" : "Out of stock"}
      </span>
      {salableQuantity != null && (
        <span className="text-xs text-muted-foreground">
          {salableQuantity} available
        </span>
      )}
    </div>
  );
}

export function ProductReassurance({ className }: { className?: string }) {
  const items = [
    { Icon: Truck, label: "Free shipping", sub: "On qualifying orders" },
    { Icon: RotateCcw, label: "30-day returns", sub: "Shop with confidence" },
    { Icon: ShieldCheck, label: "Secure checkout", sub: "Encrypted payment" },
  ];
  return (
    <ul
      className={`grid gap-x-4 gap-y-3 rounded-xl border border-border/60 p-4 sm:grid-cols-3 ${
        className ?? ""
      }`}
    >
      {items.map(({ Icon, label, sub }) => (
        <li key={label} className="flex items-center gap-2.5">
          <Icon
            className="size-5 shrink-0 text-muted-foreground"
            strokeWidth={1.5}
            aria-hidden="true"
          />
          <div className="min-w-0">
            <p className="text-xs font-medium leading-tight text-foreground">
              {label}
            </p>
            <p className="text-[11px] leading-tight text-muted-foreground">
              {sub}
            </p>
          </div>
        </li>
      ))}
    </ul>
  );
}

// Shared styling for the option pickers (variant / bundle / grouped / custom-options),
// so a selectable choice looks the same everywhere

export function ChoiceIndicator({
  checked,
  multi = false,
}: {
  checked: boolean;
  multi?: boolean;
}) {
  return (
    <span
      aria-hidden="true"
      className={[
        "flex size-5 shrink-0 items-center justify-center border transition",
        multi ? "rounded-md" : "rounded-full",
        checked
          ? "border-foreground bg-foreground text-background"
          : "border-border",
      ].join(" ")}
    >
      {checked ? (
        multi ? (
          <Check className="size-3.5" strokeWidth={3} />
        ) : (
          <span className="size-2 rounded-full bg-background" />
        )
      ) : null}
    </span>
  );
}

// Selectable-card class for a full-width option row (bundle selection, custom-option
// value, list variant)
export function optionCardClass(selected: boolean, invalid = false): string {
  return [
    "flex w-full items-center gap-3 rounded-xl border p-3 text-left transition",
    selected
      ? "border-foreground/60 bg-muted/50 ring-1 ring-foreground/15"
      : invalid
        ? "border-destructive bg-destructive/5 hover:bg-destructive/10"
        : "border-border hover:border-foreground/40 hover:bg-muted/30",
  ].join(" ");
}

// Lives with the scroll helper that has to resolve it - re-exported here so the PDPs keep
// importing their option-rendering helpers from one place
export { optionAnchorId } from "./focus-intent";

export const optionInputClass =
  "w-full rounded-lg border border-border bg-transparent px-3 py-2 text-sm outline-none transition focus:border-foreground/50 focus:ring-2 focus:ring-foreground/10";

// Same input, flagged as a skipped required field - destructive, not the ink primary, because
// this is the one place on the page that means "you have to fix this"
export const optionInputInvalidClass =
  "w-full rounded-lg border border-destructive bg-destructive/5 px-3 py-2 text-sm outline-none transition focus:border-destructive focus:ring-2 focus:ring-destructive/15";

export function formatAttribute(av: ProductAttributeValue): string {
  if (av.selectedValues.length > 0) {
    return av.selectedValues.map((v) => v.name).join(", ");
  }
  if (av.scalarValue == null) return "-";
  if (typeof av.scalarValue === "object") {
    return JSON.stringify(av.scalarValue);
  }
  return String(av.scalarValue);
}
