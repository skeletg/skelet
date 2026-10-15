import type { ReactNode } from "react";

import { Link } from "@/app/components/LocaleLink";
import type { StepStatus } from "@/app/lib/checkout/step";

// Presentational pieces with no state - the page renders the step cards, and the step forms reuse the address block

type AddressLike = {
  firstName: string;
  lastName: string;
  company: string | null;
  street1: string;
  street2: string | null;
  city: string;
  state: string | null;
  postalCode: string;
  countryCode: string;
};

export function AddressLines({ address: a }: { address: AddressLike }) {
  return (
    <span className="block leading-relaxed">
      <span className="block font-medium">
        {a.firstName} {a.lastName}
      </span>
      {a.company ? <span className="block text-muted-foreground">{a.company}</span> : null}
      <span className="block">{a.street1}</span>
      {a.street2 ? <span className="block">{a.street2}</span> : null}
      <span className="block">
        {a.city}
        {a.state ? `, ${a.state}` : ""} {a.postalCode}
      </span>
      <span className="block">{a.countryCode}</span>
    </span>
  );
}

export function oneLine(a: AddressLike): string {
  return `${a.firstName} ${a.lastName} - ${a.street1}, ${a.city} ${a.postalCode}, ${a.countryCode}`;
}

// A pending step is a header only; a completed one shows what the cart holds and an Edit link that reopens it with
// ?step=, which the page honours for any step already reached
export function StepCard({
  number,
  title,
  status,
  summary,
  editHref,
  editLabel,
  children,
}: {
  number: number;
  title: string;
  status: StepStatus;
  summary?: ReactNode;
  editHref?: string;
  editLabel: string;
  children?: ReactNode;
}) {
  return (
    <section className="glass rounded-xl" aria-current={status === "active" ? "step" : undefined}>
      <header className="flex items-center justify-between gap-3 px-5 py-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <span
            className={`grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold ${
              status === "completed"
                ? "bg-foreground text-background"
                : status === "active"
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted text-muted-foreground"
            }`}
            aria-hidden
          >
            {status === "completed" ? "✓" : number}
          </span>
          <h2 className="truncate text-base font-semibold">{title}</h2>
        </div>
        {status === "completed" && editHref ? (
          <Link
            href={editHref}
            scroll={false}
            className="text-xs text-muted-foreground underline-offset-2 hover:underline"
          >
            {editLabel}
          </Link>
        ) : null}
      </header>
      {status === "active" ? (
        <div className="border-t border-border px-5 py-5 sm:px-6">{children}</div>
      ) : null}
      {status === "completed" && summary ? (
        <div className="border-t border-border px-5 py-3 text-sm text-muted-foreground sm:px-6">{summary}</div>
      ) : null}
    </section>
  );
}
